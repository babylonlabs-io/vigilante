//go:build e2e
// +build e2e

package e2etest

import (
	"fmt"
	btcstakingtypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"go.uber.org/zap"
	"sync"
	"testing"
	"time"

	"github.com/babylonlabs-io/babylon/btcstaking"
	"github.com/babylonlabs-io/vigilante/btcclient"
	bst "github.com/babylonlabs-io/vigilante/btcstaking-tracker"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/stretchr/testify/require"
)

func TestUnbondingWatcher(t *testing.T) {
	t.Parallel()
	// segwit is activated at height 300. It's needed by staking/slashing tx
	numMatureOutputs := uint32(300)

	tm := StartManager(t, numMatureOutputs, defaultEpochInterval)
	defer tm.Stop(t)
	// Insert all existing BTC headers to babylon node
	tm.CatchUpBTCLightClient(t)

	emptyHintCache := btcclient.EmptyHintCache{}

	backend, err := btcclient.NewNodeBackend(
		btcclient.ToBitcoindConfig(tm.Config.BTC),
		&chaincfg.RegressionNetParams,
		&emptyHintCache,
	)
	require.NoError(t, err)

	err = backend.Start()
	require.NoError(t, err)

	commonCfg := config.DefaultCommonConfig()
	bstCfg := config.DefaultBTCStakingTrackerConfig()
	bstCfg.CheckDelegationsInterval = 1 * time.Second
	stakingTrackerMetrics := metrics.NewBTCStakingTrackerMetrics()

	bsTracker := bst.NewBTCStakingTracker(
		tm.BTCClient,
		backend,
		tm.BabylonClient,
		&bstCfg,
		&commonCfg,
		zap.NewNop(),
		stakingTrackerMetrics,
	)
	bsTracker.Start()
	defer bsTracker.Stop()

	// set up a finality provider
	_, fpSK := tm.CreateFinalityProvider(t)
	// set up a BTC delegation
	stakingSlashingInfo, unbondingSlashingInfo, delSK := tm.CreateBTCDelegation(t, fpSK)

	// Staker unbonds by directly sending tx to btc network. Watcher should detect it and report to babylon.
	unbondingPathSpendInfo, err := stakingSlashingInfo.StakingInfo.UnbondingPathSpendInfo()
	require.NoError(t, err)
	stakingOutIdx, err := outIdx(unbondingSlashingInfo.UnbondingTx, unbondingSlashingInfo.UnbondingInfo.UnbondingOutput)
	require.NoError(t, err)

	unbondingTxSchnorrSig, err := btcstaking.SignTxWithOneScriptSpendInputStrict(
		unbondingSlashingInfo.UnbondingTx,
		stakingSlashingInfo.StakingTx,
		stakingOutIdx,
		unbondingPathSpendInfo.GetPkScriptPath(),
		delSK,
	)
	require.NoError(t, err)

	resp, err := tm.BabylonClient.BTCDelegation(stakingSlashingInfo.StakingTx.TxHash().String())
	require.NoError(t, err)

	covenantSigs := resp.BtcDelegation.UndelegationResponse.CovenantUnbondingSigList
	witness, err := unbondingPathSpendInfo.CreateUnbondingPathWitness(
		[]*schnorr.Signature{covenantSigs[0].Sig.MustToBTCSig()},
		unbondingTxSchnorrSig,
	)
	unbondingSlashingInfo.UnbondingTx.TxIn[0].Witness = witness

	// Send unbonding tx to Bitcoin
	_, err = tm.BTCClient.SendRawTransaction(unbondingSlashingInfo.UnbondingTx, true)
	require.NoError(t, err)

	// mine a block with this tx, and insert it to Bitcoin
	unbondingTxHash := unbondingSlashingInfo.UnbondingTx.TxHash()
	t.Logf("submitted unbonding tx with hash %s", unbondingTxHash.String())
	require.Eventually(t, func() bool {
		return len(tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{&unbondingTxHash})) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	minedBlock := tm.mineBlock(t)
	require.Equal(t, 2, len(minedBlock.Transactions))

	tm.CatchUpBTCLightClient(t)

	require.Eventually(t, func() bool {
		resp, err := tm.BabylonClient.BTCDelegation(stakingSlashingInfo.StakingTx.TxHash().String())
		require.NoError(t, err)

		// TODO: Add field for staker signature in BTCDelegation query to check it directly,
		// for now it is enough to check that delegation is not active, as if unbonding was reported
		// delegation will be deactivated
		return !resp.BtcDelegation.Active

	}, eventuallyWaitTimeOut, eventuallyPollTime)
}

// TestActivatingDelegation verifies that a delegation created without an inclusion proof will
// eventually become "active".
// Specifically, that stakingEventWatcher will send a MsgAddBTCDelegationInclusionProof to do so.
func TestActivatingDelegation(t *testing.T) {
	t.Parallel()
	// segwit is activated at height 300. It's necessary for staking/slashing tx
	numMatureOutputs := uint32(300)

	tm := StartManager(t, numMatureOutputs, defaultEpochInterval)
	defer tm.Stop(t)
	// Insert all existing BTC headers to babylon node
	tm.CatchUpBTCLightClient(t)

	btcNotifier, err := btcclient.NewNodeBackend(
		btcclient.ToBitcoindConfig(tm.Config.BTC),
		&chaincfg.RegressionNetParams,
		&btcclient.EmptyHintCache{},
	)
	require.NoError(t, err)

	err = btcNotifier.Start()
	require.NoError(t, err)

	commonCfg := config.DefaultCommonConfig()
	bstCfg := config.DefaultBTCStakingTrackerConfig()
	bstCfg.CheckDelegationsInterval = 1 * time.Second
	stakingTrackerMetrics := metrics.NewBTCStakingTrackerMetrics()

	bsTracker := bst.NewBTCStakingTracker(
		tm.BTCClient,
		btcNotifier,
		tm.BabylonClient,
		&bstCfg,
		&commonCfg,
		zap.NewNop(),
		stakingTrackerMetrics,
	)
	bsTracker.Start()
	defer bsTracker.Stop()

	// set up a finality provider
	_, fpSK := tm.CreateFinalityProvider(t)
	// set up a BTC delegation
	stakingMsgTx, stakingSlashingInfo, _, _ := tm.CreateBTCDelegationWithoutIncl(t, fpSK)
	stakingMsgTxHash := stakingMsgTx.TxHash()

	// send staking tx to Bitcoin node's mempool
	_, err = tm.BTCClient.SendRawTransaction(stakingMsgTx, true)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return len(tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{&stakingMsgTxHash})) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	mBlock := tm.mineBlock(t)
	require.Equal(t, 2, len(mBlock.Transactions))

	// wait until staking tx is on Bitcoin
	require.Eventually(t, func() bool {
		_, err := tm.BTCClient.GetRawTransaction(&stakingMsgTxHash)
		return err == nil
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// We want to introduce a latency to make sure that we are not trying to submit inclusion proof while the
		// staking tx is not yet K-deep
		time.Sleep(10 * time.Second)
		// Insert k empty blocks to Bitcoin
		btccParamsResp, err := tm.BabylonClient.BTCCheckpointParams()
		if err != nil {
			fmt.Println("Error fetching BTCCheckpointParams:", err)
			return
		}
		for i := 0; i < int(btccParamsResp.Params.BtcConfirmationDepth); i++ {
			tm.mineBlock(t)
		}
		tm.CatchUpBTCLightClient(t)
	}()

	wg.Wait()

	// make sure we didn't submit any "invalid" incl proof
	require.Eventually(t, func() bool {
		return promtestutil.ToFloat64(stakingTrackerMetrics.FailedReportedActivateDelegations) == 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	require.Eventually(t, func() bool {
		return promtestutil.ToFloat64(stakingTrackerMetrics.NumberOfVerifiedNotInChainDelegations) == 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	// created delegation lacks inclusion proof, once created it will be in
	// pending status, once convenant signatures are added it will be in verified status,
	// and once the stakingEventWatcher submits MsgAddBTCDelegationInclusionProof it will
	// be in active status
	require.Eventually(t, func() bool {
		resp, err := tm.BabylonClient.BTCDelegation(stakingSlashingInfo.StakingTx.TxHash().String())
		require.NoError(t, err)

		return resp.BtcDelegation.Active
	}, eventuallyWaitTimeOut, eventuallyPollTime)
}

// TestActivatingAndUnbondingDelegation tests that delegation will eventually become UNBONDED given that
// both staking and unbonding tx are in the same block.
// In this test, we include both staking tx and unbonding tx in the same block.
// The delegation goes through "VERIFIED" → "ACTIVE" → "UNBONDED" status throughout this test.
func TestActivatingAndUnbondingDelegation(t *testing.T) {
	//t.Parallel()
	// segwit is activated at height 300. It's necessary for staking/slashing tx
	numMatureOutputs := uint32(300)

	tm := StartManager(t, numMatureOutputs, defaultEpochInterval)
	defer tm.Stop(t)
	// Insert all existing BTC headers to babylon node
	tm.CatchUpBTCLightClient(t)

	btcNotifier, err := btcclient.NewNodeBackend(
		btcclient.ToBitcoindConfig(tm.Config.BTC),
		&chaincfg.RegressionNetParams,
		&btcclient.EmptyHintCache{},
	)
	require.NoError(t, err)

	err = btcNotifier.Start()
	require.NoError(t, err)

	commonCfg := config.DefaultCommonConfig()
	bstCfg := config.DefaultBTCStakingTrackerConfig()
	bstCfg.CheckDelegationsInterval = 1 * time.Second
	stakingTrackerMetrics := metrics.NewBTCStakingTrackerMetrics()

	bsTracker := bst.NewBTCStakingTracker(
		tm.BTCClient,
		btcNotifier,
		tm.BabylonClient,
		&bstCfg,
		&commonCfg,
		zap.NewNop(),
		stakingTrackerMetrics,
	)
	bsTracker.Start()
	defer bsTracker.Stop()

	// set up a finality provider
	_, fpSK := tm.CreateFinalityProvider(t)
	// set up a BTC delegation
	stakingMsgTx, stakingSlashingInfo, unbondingSlashingInfo, delSK := tm.CreateBTCDelegationWithoutIncl(t, fpSK)
	stakingMsgTxHash := stakingMsgTx.TxHash()

	// send staking tx to Bitcoin node's mempool
	_, err = tm.BTCClient.SendRawTransaction(stakingMsgTx, true)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		return len(tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{&stakingMsgTxHash})) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	// Staker unbonds by directly sending tx to btc network. Watcher should detect it and report to babylon.
	unbondingPathSpendInfo, err := stakingSlashingInfo.StakingInfo.UnbondingPathSpendInfo()
	require.NoError(t, err)
	stakingOutIdx, err := outIdx(unbondingSlashingInfo.UnbondingTx, unbondingSlashingInfo.UnbondingInfo.UnbondingOutput)
	require.NoError(t, err)

	unbondingTxSchnorrSig, err := btcstaking.SignTxWithOneScriptSpendInputStrict(
		unbondingSlashingInfo.UnbondingTx,
		stakingSlashingInfo.StakingTx,
		stakingOutIdx,
		unbondingPathSpendInfo.GetPkScriptPath(),
		delSK,
	)
	require.NoError(t, err)

	resp, err := tm.BabylonClient.BTCDelegation(stakingSlashingInfo.StakingTx.TxHash().String())
	require.NoError(t, err)

	covenantSigs := resp.BtcDelegation.UndelegationResponse.CovenantUnbondingSigList
	witness, err := unbondingPathSpendInfo.CreateUnbondingPathWitness(
		[]*schnorr.Signature{covenantSigs[0].Sig.MustToBTCSig()},
		unbondingTxSchnorrSig,
	)
	require.NoError(t, err)
	unbondingSlashingInfo.UnbondingTx.TxIn[0].Witness = witness

	// Send unbonding tx to Bitcoin
	_, err = tm.BTCClient.SendRawTransaction(unbondingSlashingInfo.UnbondingTx, true)
	require.NoError(t, err)

	unbondingTxHash := unbondingSlashingInfo.UnbondingTx.TxHash()
	t.Logf("submitted unbonding tx with hash %s", unbondingTxHash.String())
	require.Eventually(t, func() bool {
		return len(tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{&unbondingTxHash})) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	mBlock := tm.mineBlock(t)
	// both staking and unbonding txs are in this block
	require.Equal(t, 3, len(mBlock.Transactions))

	// insert k empty blocks to Bitcoin
	btccParamsResp, err := tm.BabylonClient.BTCCheckpointParams()
	require.NoError(t, err)
	btccParams := btccParamsResp.Params
	for i := 0; i < int(btccParams.BtcConfirmationDepth); i++ {
		tm.mineBlock(t)
	}

	tm.CatchUpBTCLightClient(t)

	// wait until delegation has become unbonded
	require.Eventually(t, func() bool {
		resp, err := tm.BabylonClient.BTCDelegation(stakingSlashingInfo.StakingTx.TxHash().String())
		require.NoError(t, err)

		return resp.BtcDelegation.StatusDesc == btcstakingtypes.BTCDelegationStatus_UNBONDED.String()
	}, eventuallyWaitTimeOut, eventuallyPollTime)
}
