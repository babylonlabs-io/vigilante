//go:build e2e
// +build e2e

package e2etest

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	txformat "github.com/babylonlabs-io/babylon/v4/btctxformatter"
	checkpointingtypes "github.com/babylonlabs-io/babylon/v4/x/checkpointing/types"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	sdk "github.com/cosmos/cosmos-sdk/types"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/netparams"
	"github.com/babylonlabs-io/vigilante/reporter"
)

// VIG-02 e2e regression: covers the steady-state ordering case end-to-end
// against a real bitcoind regtest, electrs, and babylond stack. The
// attacker forges a part1 with the correct 10-byte checksum but a
// corrupted BLS signature. The poisoned pair lands on Bitcoin first, the
// reporter submits it, Babylon rejects it. With the fix in place the
// reporter does NOT lose the legitimate part0: when the real part1 is
// later mined, the legit pair matches and the checkpoint progresses to
// Submitted. Without the fix the checkpoint would stay Sealed.

func vig02e2eFakeSecondPart(t *testing.T, tag txformat.BabylonTag, firstPart, secondPart []byte) []byte {
	t.Helper()

	firstData, err := txformat.IsBabylonCheckpointData(tag, txformat.CurrentVersion, firstPart)
	require.NoError(t, err)
	secondData, err := txformat.IsBabylonCheckpointData(tag, txformat.CurrentVersion, secondPart)
	require.NoError(t, err)

	fakeData := append([]byte(nil), secondData.Data...)
	fakeData[0] ^= 0x01

	expectedChecksum := sha256.Sum256(firstData.Data)
	checksumStart := len(fakeData) - 10
	require.Equal(t, expectedChecksum[:10], fakeData[checksumStart:])

	fakeSecondPart := make([]byte, 0, len(secondPart))
	fakeSecondPart = append(fakeSecondPart, tag...)
	fakeSecondPart = append(fakeSecondPart, byte(1<<4|txformat.CurrentVersion))
	fakeSecondPart = append(fakeSecondPart, fakeData...)

	_, err = txformat.IsBabylonCheckpointData(tag, txformat.CurrentVersion, fakeSecondPart)
	require.NoError(t, err)

	return fakeSecondPart
}

func vig02e2eWaitForSealedEpoch(t *testing.T, tm *TestManager) uint64 {
	t.Helper()

	var epoch uint64
	require.Eventually(t, func() bool {
		resp, err := tm.BabylonClient.LatestEpochFromStatus(checkpointingtypes.Sealed)
		if err != nil {
			return false
		}
		epoch = resp.RawCheckpoint.EpochNum

		return true
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	return epoch
}

func vig02e2eCheckpointStatus(t *testing.T, tm *TestManager, epoch uint64) checkpointingtypes.CheckpointStatus {
	t.Helper()

	resp, err := tm.BabylonClient.RawCheckpoint(epoch)
	require.NoError(t, err)

	return resp.RawCheckpoint.Status
}

func vig02e2eEncodedParts(t *testing.T, tm *TestManager, epoch uint64) ([]byte, []byte, []byte) {
	t.Helper()

	ckptResp, err := tm.BabylonClient.RawCheckpoint(epoch)
	require.NoError(t, err)
	require.Equal(t, checkpointingtypes.Sealed, ckptResp.RawCheckpoint.Status)

	rawCheckpoint, err := ckptResp.RawCheckpoint.Ckpt.ToRawCheckpoint()
	require.NoError(t, err)

	submitterAddr, err := sdk.AccAddressFromBech32(tm.BabylonClient.MustGetAddr())
	require.NoError(t, err)

	btcCheckpoint, err := checkpointingtypes.FromRawCkptToBTCCkpt(rawCheckpoint, submitterAddr)
	require.NoError(t, err)

	btccParams, err := tm.BabylonClient.QueryClient.BTCCheckpointParams()
	require.NoError(t, err)

	tagBytes, err := hex.DecodeString(btccParams.Params.CheckpointTag)
	require.NoError(t, err)
	tag := txformat.BabylonTag(tagBytes)

	firstPart, secondPart, err := txformat.EncodeCheckpointData(tag, txformat.CurrentVersion, btcCheckpoint)
	require.NoError(t, err)

	fakeSecondPart := vig02e2eFakeSecondPart(t, tag, firstPart, secondPart)

	return firstPart, secondPart, fakeSecondPart
}

func vig02e2eSendOpReturnTx(t *testing.T, tm *TestManager, data []byte) *chainhash.Hash {
	t.Helper()

	tx := wire.NewMsgTx(wire.TxVersion)
	dataScript, err := txscript.NewScriptBuilder().AddOp(txscript.OP_RETURN).AddData(data).Script()
	require.NoError(t, err)
	tx.AddTxOut(wire.NewTxOut(0, dataScript))

	changePosition := 1
	fundedTx, err := tm.BTCClient.FundRawTransaction(tx, btcjson.FundRawTransactionOpts{
		ChangePosition: &changePosition,
	}, nil)
	require.NoError(t, err)

	require.NoError(t, tm.BTCClient.WalletPassphrase(tm.Config.BTC.WalletPassword, tm.Config.BTC.WalletLockTime))
	signedTx, complete, err := tm.BTCClient.SignRawTransactionWithWallet(fundedTx.Transaction)
	require.NoError(t, err)
	require.True(t, complete)

	hash, err := tm.BTCClient.SendRawTransaction(signedTx, true)
	require.NoError(t, err)

	return hash
}

func vig02e2eStartReporter(t *testing.T, tm *TestManager, m *metrics.ReporterMetrics) *reporter.Reporter {
	t.Helper()

	btcParams, err := netparams.GetBTCParams(tm.Config.BTC.NetParams)
	require.NoError(t, err)
	btcCfg := btcclient.ToBitcoindConfig(tm.Config.BTC)
	btcNotifier, err := btcclient.NewNodeBackend(btcCfg, btcParams, &btcclient.EmptyHintCache{})
	require.NoError(t, err)

	r, err := reporter.New(
		&tm.Config.Reporter,
		logger,
		tm.BTCClient,
		tm.BabylonClient,
		btcNotifier,
		tm.Config.Common.RetrySleepTime,
		tm.Config.Common.MaxRetrySleepTime,
		tm.Config.Common.MaxRetryTimes,
		m,
	)
	require.NoError(t, err)
	r.Start()

	return r
}

// TestReporter_CheckpointCachePoisoning_RecoversAfterRealPart1 reproduces
// the VIG-02 attack ordering against the full local stack and asserts the
// fixed behavior: the checkpoint reaches Submitted once the real part1
// arrives, even after the poisoned proof was rejected by Babylon.
func TestReporter_CheckpointCachePoisoning_RecoversAfterRealPart1(t *testing.T) {
	t.Parallel()

	// numMatureOutputs needs to be high enough that we can fund several
	// OP_RETURN transactions on top of whatever the test stack consumes.
	tm := StartManager(t, WithNumMatureOutputs(300), WithEpochInterval(5))
	defer tm.Stop(t)

	targetEpoch := vig02e2eWaitForSealedEpoch(t, tm)
	firstPart, realSecondPart, fakeSecondPart := vig02e2eEncodedParts(t, tm, targetEpoch)

	reporterMetrics := metrics.NewReporterMetrics()
	vigilantReporter := vig02e2eStartReporter(t, tm, reporterMetrics)
	defer func() {
		vigilantReporter.Stop()
		vigilantReporter.WaitForShutdown()
	}()

	require.Eventually(t, func() bool {
		return tm.BabylonBTCChainMatchesBtc(t)
	}, longEventuallyWaitTimeOut, eventuallyPollTime)

	// Mine the poison block: legitimate part0 plus checksum-valid fake part1.
	part0Hash := vig02e2eSendOpReturnTx(t, tm, firstPart)
	fakePart1Hash := vig02e2eSendOpReturnTx(t, tm, fakeSecondPart)
	poisonBlock := tm.mineBlock(t)
	t.Logf("poison block mined: part0=%s fake_part1=%s txs=%d",
		part0Hash, fakePart1Hash, len(poisonBlock.Transactions))

	// The reporter should match the poison pair, submit it, and have the
	// submission rejected by Babylon. FailedCheckpointsCounter increments.
	require.Eventually(t, func() bool {
		return promtestutil.ToFloat64(reporterMetrics.FailedCheckpointsCounter) >= 1
	}, longEventuallyWaitTimeOut, eventuallyPollTime,
		"reporter should record one failed submission for the poisoned pair")
	require.Equal(t, checkpointingtypes.Sealed, vig02e2eCheckpointStatus(t, tm, targetEpoch))

	// Mine the real part1 in a later block. With the VIG-02 fix in place,
	// the cache still holds real_part0, the (real_part0, fake_part1) pair
	// is blacklisted, and the new (real_part0, real_part1) pair matches.
	realPart1Hash := vig02e2eSendOpReturnTx(t, tm, realSecondPart)
	realBlock := tm.mineBlock(t)
	t.Logf("real part1 block mined: real_part1=%s txs=%d",
		realPart1Hash, len(realBlock.Transactions))

	// The legitimate proof should be submitted, and Babylon should advance
	// the checkpoint to Submitted.
	require.Eventually(t, func() bool {
		status := vig02e2eCheckpointStatus(t, tm, targetEpoch)

		return status == checkpointingtypes.Submitted ||
			status == checkpointingtypes.Confirmed ||
			status == checkpointingtypes.Finalized
	}, longEventuallyWaitTimeOut, eventuallyPollTime,
		"checkpoint must progress past Sealed once the real part1 lands; the VIG-02 fix preserves real_part0 across the poisoned rejection")

	// Sanity: at least one successful submission was recorded for the
	// legitimate pair (the poisoned pair contributed only to the failed
	// counter).
	require.GreaterOrEqual(t, promtestutil.ToFloat64(reporterMetrics.SuccessfulCheckpointsCounter), float64(1))
	require.GreaterOrEqual(t, promtestutil.ToFloat64(reporterMetrics.FailedCheckpointsCounter), float64(1))

	// Give the reporter a brief moment to drain any further matches; the
	// poisoned pair must NOT be resubmitted on subsequent Match() cycles.
	time.Sleep(2 * time.Second)
	require.LessOrEqual(t, promtestutil.ToFloat64(reporterMetrics.FailedCheckpointsCounter), float64(2),
		"poisoned pair should be blacklisted, not resubmitted repeatedly")
}
