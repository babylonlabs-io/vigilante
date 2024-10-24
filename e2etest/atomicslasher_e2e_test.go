//go:build e2e
// +build e2e

package e2etest

import (
	"go.uber.org/zap"
	"testing"
	"time"

	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/babylonlabs-io/vigilante/btcclient"
	bst "github.com/babylonlabs-io/vigilante/btcstaking-tracker"
	"github.com/babylonlabs-io/vigilante/btcstaking-tracker/btcslasher"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/stretchr/testify/require"
)

// TestAtomicSlasher verifies the behavior of the atomic slasher by setting up delegations,
// sending slashing transactions, and ensuring that slashing is detected and executed correctly.
func TestAtomicSlasher(t *testing.T) {
	t.Parallel()
	// segwit is activated at height 300. It's needed by staking/slashing tx
	numMatureOutputs := uint32(300)

	tm := StartManager(t, numMatureOutputs, defaultEpochInterval)
	defer tm.Stop(t)

	// start WebSocket connection with Babylon for subscriber services
	err := tm.BabylonClient.Start()
	require.NoError(t, err)
	// Insert all existing BTC headers to babylon node
	tm.CatchUpBTCLightClient(t)

	backend, err := btcclient.NewNodeBackend(
		btcclient.ToBitcoindConfig(tm.Config.BTC),
		&chaincfg.RegressionNetParams,
		&btcclient.EmptyHintCache{},
	)
	require.NoError(t, err)

	err = backend.Start()
	require.NoError(t, err)

	commonCfg := config.DefaultCommonConfig()
	bstCfg := config.DefaultBTCStakingTrackerConfig()
	bstCfg.CheckDelegationsInterval = 1 * time.Second

	bsTracker := bst.NewBTCStakingTracker(
		tm.BTCClient,
		backend,
		tm.BabylonClient,
		&bstCfg,
		&commonCfg,
		zap.NewNop(),
		metrics.NewBTCStakingTrackerMetrics(),
	)
	go bsTracker.Start()
	defer bsTracker.Stop()

	bsParamsResp, err := tm.BabylonClient.BTCStakingParams()
	require.NoError(t, err)
	bsParams := bsParamsResp.Params

	// set up a finality provider
	btcFP, fpSK := tm.CreateFinalityProvider(t)
	// set up 2 BTC delegations
	tm.CreateBTCDelegation(t, fpSK)
	tm.CreateBTCDelegation(t, fpSK)

	// retrieve 2 BTC delegations
	btcDelsResp, err := tm.BabylonClient.BTCDelegations(bstypes.BTCDelegationStatus_ACTIVE, nil)
	require.NoError(t, err)
	require.Len(t, btcDelsResp.BtcDelegations, 2)
	btcDels := btcDelsResp.BtcDelegations

	/*
		finality provider builds slashing tx witness and sends slashing tx to Bitcoin
	*/
	victimBTCDel := btcDels[0]
	victimSlashingTx, err := btcslasher.BuildSlashingTxWithWitness(victimBTCDel, &bsParams, regtestParams, fpSK)
	// send slashing tx to Bitcoin
	require.NoError(t, err)
	slashingTxHash, err := tm.BTCClient.SendRawTransaction(victimSlashingTx, true)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		_, err := tm.BTCClient.GetRawTransaction(slashingTxHash)
		return err == nil
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	// mine a block that includes slashing tx, which will trigger atomic slasher
	require.Eventually(t, func() bool {
		return len(tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{slashingTxHash})) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	minedBlock := tm.mineBlock(t)
	require.Equal(t, 2, len(minedBlock.Transactions))

	/*
		atomic slasher will detect the selective slashing on victim BTC delegation
		the finality provider will get slashed on Babylon
	*/
	require.Eventually(t, func() bool {
		resp, err := tm.BabylonClient.FinalityProvider(btcFP.BtcPk.MarshalHex())
		if err != nil {
			return false
		}
		return resp.FinalityProvider.SlashedBabylonHeight > 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	/*
		atomic slasher will slash the other BTC delegation on Bitcoin
	*/
	btcDel2 := btcDels[1]
	slashTx2, err := bstypes.NewBTCSlashingTxFromHex(btcDel2.SlashingTxHex)
	require.NoError(t, err)
	slashingTxHash2 := slashTx2.MustGetTxHash()

	require.Eventually(t, func() bool {
		_, err := tm.BTCClient.GetRawTransaction(slashingTxHash2)
		if err != nil {
			t.Logf("err of getting slashingTxHash of the BTC delegation affected by atomic slashing: %v", err)
		}
		return err == nil
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	// mine a block that includes slashing tx, which will trigger atomic slasher
	require.Eventually(t, func() bool {
		return len(tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{slashingTxHash2})) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	minedBlock = tm.mineBlock(t)
	require.Equal(t, 2, len(minedBlock.Transactions))
}

// TestAtomicSlasher_Unbonding tests the atomic slasher's handling of unbonding BTC delegations,
// including the creation and detection of unbonding slashing transactions.
func TestAtomicSlasher_Unbonding(t *testing.T) {
	t.Parallel()
	// segwit is activated at height 300. It's needed by staking/slashing tx
	numMatureOutputs := uint32(300)

	tm := StartManager(t, numMatureOutputs, defaultEpochInterval)
	defer tm.Stop(t)

	// start WebSocket connection with Babylon for subscriber services
	err := tm.BabylonClient.Start()
	require.NoError(t, err)
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
	go bsTracker.Start()
	defer bsTracker.Stop()

	bsParamsResp, err := tm.BabylonClient.BTCStakingParams()
	require.NoError(t, err)
	bsParams := bsParamsResp.Params

	// set up a finality provider
	btcFP, fpSK := tm.CreateFinalityProvider(t)

	// set up 1st BTC delegation, which will be later used as the victim
	stakingSlashingInfo, unbondingSlashingInfo, btcDelSK := tm.CreateBTCDelegation(t, fpSK)
	btcDelsResp, err := tm.BabylonClient.BTCDelegations(bstypes.BTCDelegationStatus_ACTIVE, nil)
	require.NoError(t, err)
	require.Len(t, btcDelsResp.BtcDelegations, 1)
	victimBTCDel := btcDelsResp.BtcDelegations[0]

	// set up 2nd BTC delegation, which will be subjected to atomic slashing
	tm.CreateBTCDelegation(t, fpSK)
	btcDelsResp2, err := tm.BabylonClient.BTCDelegations(bstypes.BTCDelegationStatus_ACTIVE, nil)
	require.NoError(t, err)
	require.Len(t, btcDelsResp2.BtcDelegations, 2)

	// NOTE: `BTCDelegations` API does not return BTC delegations in created time order
	// thus we need to find out the 2nd BTC delegation one-by-one
	var btcDel2 *bstypes.BTCDelegationResponse
	for _, delResp2 := range btcDelsResp2.BtcDelegations {
		if delResp2.StakingTxHex != victimBTCDel.StakingTxHex {
			btcDel2 = delResp2
			break
		}
	}

	require.NotNil(t, btcDel2, "err second delegation not found")

	/*
		the victim BTC delegation unbonds
	*/
	tm.Undelegate(t, stakingSlashingInfo, unbondingSlashingInfo, btcDelSK, func() { tm.CatchUpBTCLightClient(t) })

	/*
		finality provider builds unbonding slashing tx witness and sends it to Bitcoin
	*/
	victimUnbondingSlashingTx, err := btcslasher.BuildUnbondingSlashingTxWithWitness(victimBTCDel, &bsParams, regtestParams, fpSK)
	require.NoError(t, err)

	// send slashing tx to Bitcoin
	// NOTE: sometimes unbonding slashing tx is not immediately spendable for some reason
	var unbondingSlashingTxHash *chainhash.Hash
	require.Eventually(t, func() bool {
		unbondingSlashingTxHash, err = tm.BTCClient.SendRawTransaction(victimUnbondingSlashingTx, true)
		if err != nil {
			t.Logf("err of SendRawTransaction: %v", err)
			return false
		}
		return true
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	// unbonding slashing tx is eventually queryable
	require.Eventually(t, func() bool {
		_, err := tm.BTCClient.GetRawTransaction(unbondingSlashingTxHash)
		if err != nil {
			t.Logf("err of GetRawTransaction: %v", err)
			return false
		}
		return true
	}, eventuallyWaitTimeOut, eventuallyPollTime)
	// mine a block that includes unbonding slashing tx, which will trigger atomic slasher
	require.Eventually(t, func() bool {
		return len(tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{unbondingSlashingTxHash})) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	minedBlock := tm.mineBlock(t)
	require.Equal(t, 2, len(minedBlock.Transactions))

	/*
		atomic slasher will detect the selective slashing on victim BTC delegation
		the finality provider will get slashed on Babylon
	*/
	require.Eventually(t, func() bool {
		resp, err := tm.BabylonClient.FinalityProvider(btcFP.BtcPk.MarshalHex())
		if err != nil {
			return false
		}
		return resp.FinalityProvider.SlashedBabylonHeight > 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	/*
		atomic slasher will slash the other BTC delegation on Bitcoin
	*/
	slashingTx2, err := bstypes.NewBTCSlashingTxFromHex(btcDel2.SlashingTxHex)
	require.NoError(t, err)

	slashingTxHash2 := slashingTx2.MustGetTxHash()
	require.Eventually(t, func() bool {
		_, err := tm.BTCClient.GetRawTransaction(slashingTxHash2)
		t.Logf("err of getting slashingTxHash of the BTC delegation affected by atomic slashing: %v", err)
		return err == nil
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	// mine a block that includes slashing tx, which will trigger atomic slasher
	require.Eventually(t, func() bool {
		return len(tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{slashingTxHash2})) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	minedBlock = tm.mineBlock(t)
	require.Equal(t, 2, len(minedBlock.Transactions))
}
