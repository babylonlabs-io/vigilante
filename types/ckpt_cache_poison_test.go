package types_test

import (
	"crypto/sha256"
	"math/rand"
	"testing"

	"github.com/babylonlabs-io/babylon/v4/btctxformatter"
	"github.com/babylonlabs-io/babylon/v4/testutil/datagen"
	"github.com/babylonlabs-io/vigilante/types"
	"github.com/stretchr/testify/require"
)

// VIG-02 regression: an attacker who observes a legitimate part0 on Bitcoin
// can forge a part1 with the correct 10-byte checksum but a corrupted BLS
// signature. Before the fix, CheckpointCache.Match() paired real_part0 with
// the fake part1 and immediately deleted both segments; the rejected proof
// then orphaned the real part1 when it arrived later. After the fix, the
// pair is queued without deletion, the rejected pair is recorded so it is
// not retried, and a later real part1 can still match real_part0.

func vig02RawCheckpoint(seed int64, epoch uint64) *btctxformatter.RawBtcCheckpoint {
	r := rand.New(rand.NewSource(seed))

	return &btctxformatter.RawBtcCheckpoint{
		Epoch:            epoch,
		BlockHash:        datagen.GenRandomByteArray(r, btctxformatter.BlockHashLength),
		BitMap:           datagen.GenRandomByteArray(r, btctxformatter.BitMapLength),
		SubmitterAddress: datagen.GenRandomByteArray(r, btctxformatter.AddressLength),
		BlsSig:           datagen.GenRandomByteArray(r, btctxformatter.BlsSigLength),
	}
}

func vig02SegmentsFromEncoded(t *testing.T, tag btctxformatter.BabylonTag, firstHalf, secondHalf []byte) (*types.CkptSegment, *types.CkptSegment) {
	t.Helper()

	firstData, err := btctxformatter.IsBabylonCheckpointData(tag, btctxformatter.CurrentVersion, firstHalf)
	require.NoError(t, err)
	secondData, err := btctxformatter.IsBabylonCheckpointData(tag, btctxformatter.CurrentVersion, secondHalf)
	require.NoError(t, err)

	return &types.CkptSegment{BabylonData: firstData}, &types.CkptSegment{BabylonData: secondData}
}

// vig02FakeSecondHalf builds a part1 OP_RETURN payload that keeps the
// 10-byte checksum over the legitimate part0 but corrupts the BLS signature
// bytes. ConnectParts and DecodeRawCheckpoint accept it locally; Babylon's
// VerifyCheckpoint rejects it as "invalid BLS multi-sig".
func vig02FakeSecondHalf(t *testing.T, tag btctxformatter.BabylonTag, firstHalf, realSecondHalf []byte) []byte {
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

// TestCheckpointCachePoisonedPart1DoesNotConsumeRealPart0 covers the
// steady-state ordering case: the poison block (real_part0 + fake_part1)
// is processed first, its proof is rejected by Babylon, and a later block
// brings the real part1. The fix must keep real_part0 around so the
// legitimate match still happens.
func TestCheckpointCachePoisonedPart1DoesNotConsumeRealPart0(t *testing.T) {
	t.Parallel()

	tag := btctxformatter.BabylonTag([]byte("bbn0"))
	rawCheckpoint := vig02RawCheckpoint(20260428, 88)
	firstHalf, realSecondHalf, err := btctxformatter.EncodeCheckpointData(tag, btctxformatter.CurrentVersion, rawCheckpoint)
	require.NoError(t, err)
	fakeSecondHalf := vig02FakeSecondHalf(t, tag, firstHalf, realSecondHalf)

	firstSeg, fakeSecondSeg := vig02SegmentsFromEncoded(t, tag, firstHalf, fakeSecondHalf)
	_, realSecondSeg := vig02SegmentsFromEncoded(t, tag, firstHalf, realSecondHalf)

	cache := types.NewCheckpointCache(tag, btctxformatter.CurrentVersion)
	require.NoError(t, cache.AddSegment(firstSeg))
	require.NoError(t, cache.AddSegment(fakeSecondSeg))

	// First match: legit part0 + fake part1 connect locally.
	cache.Match()
	require.Equal(t, 1, cache.NumCheckpoints(), "the poisoned pair is queued for submission")
	require.Equal(t, 2, cache.NumSegments(), "VIG-02 fix: segments are NOT deleted at match time")

	// Reporter pops the candidate and submits to Babylon. Babylon rejects
	// it with a non-transient error (invalid BLS). The reporter calls
	// MarkAttempted to prevent resubmitting the same proof.
	popped := cache.PopEarliestCheckpoint()
	require.NotNil(t, popped)
	require.Equal(t, rawCheckpoint.Epoch, popped.Epoch)
	cache.MarkAttempted(popped)
	require.Equal(t, 1, cache.NumAttemptedPairs())
	require.Equal(t, 2, cache.NumSegments(), "segments survive the rejection")

	// Real part1 arrives in a later block.
	require.NoError(t, cache.AddSegment(realSecondSeg))

	cache.Match()
	require.Equal(t, 1, cache.NumCheckpoints(),
		"VIG-02 fix: real part1 matches real part0 after the poisoned pair was rejected")
	require.Equal(t, 3, cache.NumSegments(),
		"all segments still present until the successful submission removes them")

	popped = cache.PopEarliestCheckpoint()
	require.NotNil(t, popped)
	require.Equal(t, rawCheckpoint.Epoch, popped.Epoch)

	// Babylon accepts the legitimate proof. The reporter calls
	// RemoveSegments to drop both halves of the matched pair.
	cache.RemoveSegments(popped)
	require.Equal(t, 1, cache.NumSegments(),
		"only the unrelated fake_part1 remains after the legit pair is removed")
	require.Equal(t, 0, cache.NumCheckpoints())

	// A subsequent Match() must not requeue the poisoned pair: real_part0
	// has been removed, and the (real_part0, fake_part1) key is also gone
	// because RemoveSegments cleared it.
	cache.Match()
	require.Equal(t, 0, cache.NumCheckpoints(),
		"poisoned pair no longer matchable: real_part0 has been removed")
}

// TestCheckpointCacheMatchSkipsPreviouslyAttemptedPairs ensures that calling
// Match() repeatedly does not re-queue a pair that was already submitted and
// rejected. Without the dedup set, the poisoned pair would resurface on
// every Match() cycle and spam Babylon with the same rejected proof.
func TestCheckpointCacheMatchSkipsPreviouslyAttemptedPairs(t *testing.T) {
	t.Parallel()

	tag := btctxformatter.BabylonTag([]byte("bbn0"))
	rawCheckpoint := vig02RawCheckpoint(20260428, 88)
	firstHalf, realSecondHalf, err := btctxformatter.EncodeCheckpointData(tag, btctxformatter.CurrentVersion, rawCheckpoint)
	require.NoError(t, err)
	fakeSecondHalf := vig02FakeSecondHalf(t, tag, firstHalf, realSecondHalf)

	firstSeg, fakeSecondSeg := vig02SegmentsFromEncoded(t, tag, firstHalf, fakeSecondHalf)

	cache := types.NewCheckpointCache(tag, btctxformatter.CurrentVersion)
	require.NoError(t, cache.AddSegment(firstSeg))
	require.NoError(t, cache.AddSegment(fakeSecondSeg))

	cache.Match()
	require.Equal(t, 1, cache.NumCheckpoints())
	popped := cache.PopEarliestCheckpoint()
	cache.MarkAttempted(popped)

	// Subsequent Match() runs must not re-queue the same poisoned pair.
	for i := 0; i < 5; i++ {
		cache.Match()
		require.Equal(t, 0, cache.NumCheckpoints(),
			"attempted pair stays blacklisted across repeated Match() calls")
	}
}

// TestCheckpointCacheMatchesBothPairsWhenRealPart1IsAlreadyCached covers
// the bootstrap recovery path: when the reporter restarts, both fake_part1
// and real_part1 are extracted from the BTC cache before Match() runs.
// Match() must produce both candidate checkpoints (poisoned + real) so the
// reporter's submission loop can fail on the poisoned one and succeed on
// the real one.
func TestCheckpointCacheMatchesBothPairsWhenRealPart1IsAlreadyCached(t *testing.T) {
	t.Parallel()

	tag := btctxformatter.BabylonTag([]byte("bbn0"))
	rawCheckpoint := vig02RawCheckpoint(20260428, 88)
	firstHalf, realSecondHalf, err := btctxformatter.EncodeCheckpointData(tag, btctxformatter.CurrentVersion, rawCheckpoint)
	require.NoError(t, err)
	fakeSecondHalf := vig02FakeSecondHalf(t, tag, firstHalf, realSecondHalf)

	firstSeg, fakeSecondSeg := vig02SegmentsFromEncoded(t, tag, firstHalf, fakeSecondHalf)
	_, realSecondSeg := vig02SegmentsFromEncoded(t, tag, firstHalf, realSecondHalf)

	cache := types.NewCheckpointCache(tag, btctxformatter.CurrentVersion)
	require.NoError(t, cache.AddSegment(firstSeg))
	require.NoError(t, cache.AddSegment(fakeSecondSeg))
	require.NoError(t, cache.AddSegment(realSecondSeg))

	cache.Match()
	require.Equal(t, 2, cache.NumCheckpoints(),
		"both (real_part0, fake_part1) and (real_part0, real_part1) match")

	// Simulate the reporter's submission loop: pop both candidates,
	// reject one (poisoned), accept the other (real).
	var poisoned, legit *types.Ckpt
	for i := 0; i < 2; i++ {
		ckpt := cache.PopEarliestCheckpoint()
		require.NotNil(t, ckpt)
		// distinguish by which part1 segment is referenced
		fakeHash := sha256.Sum256(fakeSecondSeg.Data)
		realHash := sha256.Sum256(realSecondSeg.Data)
		ckptPart1Hash := sha256.Sum256(ckpt.Segments[1].Data)
		switch ckptPart1Hash {
		case fakeHash:
			poisoned = ckpt
		case realHash:
			legit = ckpt
		}
	}
	require.NotNil(t, poisoned, "poisoned candidate should be queued")
	require.NotNil(t, legit, "real candidate should be queued")

	cache.MarkAttempted(poisoned)
	cache.RemoveSegments(legit)

	// fake_part1 stays cached (nothing removed it), but the poisoned pair
	// is blacklisted. real_part0 and real_part1 are gone after success.
	require.Equal(t, 1, cache.NumSegments())
	require.Equal(t, 1, cache.NumAttemptedPairs())

	cache.Match()
	require.Equal(t, 0, cache.NumCheckpoints(),
		"no pairs left to match: real halves removed, poisoned pair blacklisted")
}

// TestCheckpointCacheRemoveSegmentsClearsAttemptedPairsForSamePair documents
// that successfully resolving a pair (e.g. via duplicate-submission accept)
// also clears the attempted-pair entry, freeing memory.
func TestCheckpointCacheRemoveSegmentsClearsAttemptedPairsForSamePair(t *testing.T) {
	t.Parallel()

	tag := btctxformatter.BabylonTag([]byte("bbn0"))
	rawCheckpoint := vig02RawCheckpoint(1, 1)
	firstHalf, secondHalf, err := btctxformatter.EncodeCheckpointData(tag, btctxformatter.CurrentVersion, rawCheckpoint)
	require.NoError(t, err)

	firstSeg, secondSeg := vig02SegmentsFromEncoded(t, tag, firstHalf, secondHalf)

	cache := types.NewCheckpointCache(tag, btctxformatter.CurrentVersion)
	require.NoError(t, cache.AddSegment(firstSeg))
	require.NoError(t, cache.AddSegment(secondSeg))

	cache.Match()
	require.Equal(t, 1, cache.NumCheckpoints())
	ckpt := cache.PopEarliestCheckpoint()

	cache.MarkAttempted(ckpt)
	require.Equal(t, 1, cache.NumAttemptedPairs())

	cache.RemoveSegments(ckpt)
	require.Equal(t, 0, cache.NumAttemptedPairs(),
		"successful resolution clears the attempted-pair entry")
	require.Equal(t, 0, cache.NumSegments())
}
