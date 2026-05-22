package reporter_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/rand"
	"testing"

	"github.com/babylonlabs-io/babylon/v4/btctxformatter"
	"github.com/babylonlabs-io/babylon/v4/client/babylonclient"
	"github.com/babylonlabs-io/babylon/v4/testutil/datagen"
	btcctypes "github.com/babylonlabs-io/babylon/v4/x/btccheckpoint/types"
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	vdatagen "github.com/babylonlabs-io/vigilante/testutil/datagen"
	vtypes "github.com/babylonlabs-io/vigilante/types"
)

// VIG-02 reporter-level regression: a poisoned (real_part0, fake_part1)
// pair gets submitted, Babylon rejects it, and a later block bringing the
// real part1 must still produce a successful submission. The fix keeps the
// segments in the cache after the rejection and blacklists only the
// specific (part0, part1) pair, so real_part0 can re-pair with real_part1.

func vig02PoisonRawCheckpoint(seed int64, epoch uint64) *btctxformatter.RawBtcCheckpoint {
	r := rand.New(rand.NewSource(seed))

	return &btctxformatter.RawBtcCheckpoint{
		Epoch:            epoch,
		BlockHash:        datagen.GenRandomByteArray(r, btctxformatter.BlockHashLength),
		BitMap:           datagen.GenRandomByteArray(r, btctxformatter.BitMapLength),
		SubmitterAddress: datagen.GenRandomByteArray(r, btctxformatter.AddressLength),
		BlsSig:           datagen.GenRandomByteArray(r, btctxformatter.BlsSigLength),
	}
}

func vig02PoisonFakeSecondHalf(t *testing.T, tag btctxformatter.BabylonTag, firstHalf, realSecondHalf []byte) []byte {
	t.Helper()

	firstData, err := btctxformatter.IsBabylonCheckpointData(tag, btctxformatter.CurrentVersion, firstHalf)
	require.NoError(t, err)
	secondData, err := btctxformatter.IsBabylonCheckpointData(tag, btctxformatter.CurrentVersion, realSecondHalf)
	require.NoError(t, err)

	fakeData := append([]byte(nil), secondData.Data...)
	fakeData[0] ^= 0x01

	expectedChecksum := sha256.Sum256(firstData.Data)
	checksumStart := len(fakeData) - 10
	require.Equal(t, expectedChecksum[:10], fakeData[checksumStart:])

	fakeSecondHalf := make([]byte, 0, len(realSecondHalf))
	fakeSecondHalf = append(fakeSecondHalf, tag...)
	fakeSecondHalf = append(fakeSecondHalf, byte(1<<4|btctxformatter.CurrentVersion))
	fakeSecondHalf = append(fakeSecondHalf, fakeData...)

	_, err = btctxformatter.IsBabylonCheckpointData(tag, btctxformatter.CurrentVersion, fakeSecondHalf)
	require.NoError(t, err)

	return fakeSecondHalf
}

func vig02OpReturnTx(t *testing.T, r *rand.Rand, checkpointPart []byte) *wire.MsgTx {
	t.Helper()

	tx := vdatagen.GenRandomTx(r)
	script, err := txscript.NewScriptBuilder().AddOp(txscript.OP_RETURN).AddData(checkpointPart).Script()
	require.NoError(t, err)
	tx.TxOut[0] = wire.NewTxOut(0, script)

	return tx
}

func vig02CalcMerkleRoot(txs []*wire.MsgTx) chainhash.Hash {
	utilTxs := make([]*btcutil.Tx, 0, len(txs))
	for _, tx := range txs {
		utilTxs = append(utilTxs, btcutil.NewTx(tx))
	}
	tree := blockchain.BuildMerkleTreeStore(utilTxs, false)

	return *tree[len(tree)-1]
}

func vig02IndexedBlockWithParts(t *testing.T, r *rand.Rand, height uint32, prevHash *chainhash.Hash, parts ...[]byte) (*vtypes.IndexedBlock, chainhash.Hash) {
	t.Helper()

	block, _ := vdatagen.GenRandomBlock(r, 0, prevHash)
	txs := []*wire.MsgTx{block.Transactions[0]}
	for _, p := range parts {
		txs = append(txs, vig02OpReturnTx(t, r, p))
	}
	block.Transactions = txs
	block.Header.MerkleRoot = vig02CalcMerkleRoot(txs)

	indexed := vtypes.NewIndexedBlockFromMsgBlock(height, block)
	hash := indexed.BlockHash()

	return indexed, hash
}

func vig02DefaultTagAndParts(t *testing.T, epoch uint64) ([]byte, []byte, []byte) {
	t.Helper()

	tagBytes, err := hex.DecodeString(btcctypes.DefaultParams().CheckpointTag)
	require.NoError(t, err)
	tag := btctxformatter.BabylonTag(tagBytes)
	rawCheckpoint := vig02PoisonRawCheckpoint(20260428, epoch)
	firstHalf, realSecondHalf, err := btctxformatter.EncodeCheckpointData(tag, btctxformatter.CurrentVersion, rawCheckpoint)
	require.NoError(t, err)
	fakeSecondHalf := vig02PoisonFakeSecondHalf(t, tag, firstHalf, realSecondHalf)

	return firstHalf, realSecondHalf, fakeSecondHalf
}

// TestReporter_ProcessCheckpoints_PoisonedPart1RecoversAfterRealPart1Arrives
// covers the steady-state ordering case end-to-end through the reporter:
//   - Block A contains real_part0 and fake_part1 (the attacker's poison block)
//   - The reporter submits the matched pair, Babylon rejects it
//   - Block B contains real_part1
//   - The reporter must produce a second, successful submission of the
//     legitimate checkpoint (this is what the fix guarantees)
func TestReporter_ProcessCheckpoints_PoisonedPart1RecoversAfterRealPart1Arrives(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBabylonClient, r := newMockReporter(t, ctrl)
	firstHalf, realSecondHalf, fakeSecondHalf := vig02DefaultTagAndParts(t, 99)

	rng := rand.New(rand.NewSource(99))
	blockA, blockAHash := vig02IndexedBlockWithParts(t, rng, 100, nil, firstHalf, fakeSecondHalf)
	blockB, _ := vig02IndexedBlockWithParts(t, rng, 101, &blockAHash, realSecondHalf)

	// First call (Block A): Babylon rejects with invalid-BLS-style error.
	// Second call (Block B): Babylon accepts the legitimate proof.
	gomock.InOrder(
		mockBabylonClient.EXPECT().ReliablySendMsg(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("invalid BLS multi-sig: raw checkpoint is invalid")).
			Times(1),
		mockBabylonClient.EXPECT().ReliablySendMsg(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&babylonclient.RelayerTxResponse{Code: 0}, nil).
			Times(1),
	)

	// Block A: poison submission gets rejected; segments stay in cache.
	numSegs, numMatched := r.ProcessCheckpoints("bbn1poison", []*vtypes.IndexedBlock{blockA})
	require.Equal(t, 2, numSegs)
	require.Equal(t, 1, numMatched, "poison pair is matched and submitted")

	// Block B: real_part1 arrives. With the fix, real_part0 is still in
	// the cache (Match() did not delete it), so the legit pair matches
	// and the reporter submits a second time, successfully.
	numSegs, numMatched = r.ProcessCheckpoints("bbn1poison", []*vtypes.IndexedBlock{blockB})
	require.Equal(t, 1, numSegs)
	require.Equal(t, 1, numMatched, "real pair is matched and submitted after rejection of the poisoned pair")
}

// TestReporter_ProcessCheckpoints_PoisonedPair_NotResubmittedOnSubsequentMatches
// pins the dedup behavior: once a pair has been rejected by Babylon, the
// reporter must not resubmit the same proof on the next ProcessCheckpoints
// invocation.
func TestReporter_ProcessCheckpoints_PoisonedPair_NotResubmittedOnSubsequentMatches(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBabylonClient, r := newMockReporter(t, ctrl)
	firstHalf, _, fakeSecondHalf := vig02DefaultTagAndParts(t, 100)

	rng := rand.New(rand.NewSource(100))
	blockA, _ := vig02IndexedBlockWithParts(t, rng, 200, nil, firstHalf, fakeSecondHalf)
	emptyBlock, _ := vig02IndexedBlockWithParts(t, rng, 201, nil)

	// Babylon must be called exactly once across both ProcessCheckpoints
	// invocations: the second invocation finds the same pair but skips it
	// because it was already attempted.
	mockBabylonClient.EXPECT().
		ReliablySendMsg(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("invalid BLS multi-sig: raw checkpoint is invalid")).
		Times(1)

	_, numMatched := r.ProcessCheckpoints("bbn1poison", []*vtypes.IndexedBlock{blockA})
	require.Equal(t, 1, numMatched)

	// Subsequent processing must not resubmit the rejected pair. We pass
	// an empty block to drive the matchAndSubmitCheckpoints path without
	// adding new segments.
	_, numMatched = r.ProcessCheckpoints("bbn1poison", []*vtypes.IndexedBlock{emptyBlock})
	require.Equal(t, 0, numMatched, "blacklisted pair must not be re-submitted")
}

// TestReporter_ProcessCheckpoints_TransientErrorAllowsRetry pins the
// transient-error branch: if ReliablySendMsg returns the broadcast-timeout
// error, the segments stay in the cache without being marked attempted, so
// the next ProcessCheckpoints call retries the same pair.
func TestReporter_ProcessCheckpoints_TransientErrorAllowsRetry(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBabylonClient, r := newMockReporter(t, ctrl)
	firstHalf, realSecondHalf, _ := vig02DefaultTagAndParts(t, 101)

	rng := rand.New(rand.NewSource(101))
	blockA, blockAHash := vig02IndexedBlockWithParts(t, rng, 300, nil, firstHalf, realSecondHalf)
	emptyBlock, _ := vig02IndexedBlockWithParts(t, rng, 301, &blockAHash)

	gomock.InOrder(
		mockBabylonClient.EXPECT().
			ReliablySendMsg(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _, _, _ interface{}) (*babylonclient.RelayerTxResponse, error) {
				return nil, babylonclient.ErrTimeoutAfterWaitingForTxBroadcast
			}).
			Times(1),
		mockBabylonClient.EXPECT().
			ReliablySendMsg(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&babylonclient.RelayerTxResponse{Code: 0}, nil).
			Times(1),
	)

	_, numMatched := r.ProcessCheckpoints("bbn1retry", []*vtypes.IndexedBlock{blockA})
	require.Equal(t, 1, numMatched)

	// On the next match cycle the same pair must be retried because the
	// previous failure was transient. We avoid adding new segments.
	_, numMatched = r.ProcessCheckpoints("bbn1retry", []*vtypes.IndexedBlock{emptyBlock})
	require.Equal(t, 1, numMatched, "transient errors should not blacklist the pair")
}
