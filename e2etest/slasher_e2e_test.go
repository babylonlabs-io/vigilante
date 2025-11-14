//go:build e2e

package e2etest

import (
	"testing"
	"time"

	"github.com/babylonlabs-io/vigilante/testutil"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/vigilante/btcclient"
	bst "github.com/babylonlabs-io/vigilante/btcstaking-tracker"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/stretchr/testify/require"
)

func TestSlasher_GracefulShutdown(t *testing.T) {
	t.Parallel()
	numMatureOutputs := uint32(300)

	tm := StartManager(t, WithNumMatureOutputs(numMatureOutputs), WithEpochInterval(defaultEpochInterval))
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
		testutil.MakeTestBackend(t),
	)

	go bsTracker.Start()

	// wait for bootstrapping
	time.Sleep(10 * time.Second)

	tm.BTCClient.Stop()
	// gracefully shut down
	defer bsTracker.Stop()
}

func TestSlasher_Slasher(t *testing.T) {
	t.Parallel()
	// segwit is activated at height 300. It's needed by staking/slashing tx
	numMatureOutputs := uint32(300)

	tm := StartManager(t, WithNumMatureOutputs(numMatureOutputs), WithEpochInterval(5))
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
	bstCfg.IndexerAddr = tm.Config.BTCStakingTracker.IndexerAddr

	bsTracker := bst.NewBTCStakingTracker(
		tm.BTCClient,
		backend,
		tm.BabylonClient,
		&bstCfg,
		&commonCfg,
		zap.NewNop(),
		stakingTrackerMetrics,
		testutil.MakeTestBackend(t),
	)
	go bsTracker.Start()
	defer bsTracker.Stop()

	// wait for bootstrapping
	time.Sleep(5 * time.Second)

	// set up a finality provider
	_, fpSK := tm.CreateFinalityProvider(t)
	// set up a BTC delegation
	stakingSlashingInfo, _, _ := tm.CreateBTCDelegation(t, fpSK)

	// commit public randomness, vote and equivocate
	tm.VoteAndEquivocate(t, fpSK)

	// slashing tx will eventually enter mempool
	slashingMsgTx, err := stakingSlashingInfo.SlashingTx.ToMsgTx()
	require.NoError(t, err)
	slashingMsgTxHash1 := slashingMsgTx.TxHash()
	slashingMsgTxHash := &slashingMsgTxHash1

	btccParamsResp, err := tm.BabylonClient.BTCCheckpointParams()
	require.NoError(t, err)

	for i := 0; i <= int(btccParamsResp.Params.BtcConfirmationDepth); i++ {
		tm.mineBlock(t)
	}

	// mine a block that includes slashing tx
	require.Eventually(t, func() bool {
		txns, err := tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{slashingMsgTxHash})
		if err != nil {
			t.Logf("error: %v", err)
			return false
		}
		return len(txns) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)
}

func TestSlasher_Slasher_MultisigBTCDel(t *testing.T) {
	t.Parallel()
	// segwit is activated at height 300. It's needed by staking/slashing tx
	numMatureOutputs := uint32(300)

	tm := StartManager(t, WithNumMatureOutputs(numMatureOutputs), WithEpochInterval(5))
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
	bstCfg.IndexerAddr = tm.Config.BTCStakingTracker.IndexerAddr

	bsTracker := bst.NewBTCStakingTracker(
		tm.BTCClient,
		backend,
		tm.BabylonClient,
		&bstCfg,
		&commonCfg,
		zap.NewNop(),
		stakingTrackerMetrics,
		testutil.MakeTestBackend(t),
	)
	go bsTracker.Start()
	defer bsTracker.Stop()

	// wait for bootstrapping
	time.Sleep(5 * time.Second)

	// set up a finality provider
	_, fpSK := tm.CreateFinalityProvider(t)
	// set up a BTC delegation
	stakingSlashingInfo, _, _ := tm.CreateMultisigBTCDelegation(t, fpSK)

	// commit public randomness, vote and equivocate
	tm.VoteAndEquivocate(t, fpSK)

	// slashing tx will eventually enter mempool
	slashingMsgTx, err := stakingSlashingInfo.SlashingTx.ToMsgTx()
	require.NoError(t, err)
	slashingMsgTxHash1 := slashingMsgTx.TxHash()
	slashingMsgTxHash := &slashingMsgTxHash1

	btccParamsResp, err := tm.BabylonClient.BTCCheckpointParams()
	require.NoError(t, err)

	for i := 0; i <= int(btccParamsResp.Params.BtcConfirmationDepth); i++ {
		tm.mineBlock(t)
	}

	// mine a block that includes slashing tx
	require.Eventually(t, func() bool {
		txns, err := tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{slashingMsgTxHash})
		if err != nil {
			t.Logf("error: %v", err)
			return false
		}
		return len(txns) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)
}

func TestSlasher_SlashingUnbonding(t *testing.T) {
	t.Parallel()
	// segwit is activated at height 300. It's needed by staking/slashing tx
	numMatureOutputs := uint32(300)

	tm := StartManager(t, WithNumMatureOutputs(numMatureOutputs), WithEpochInterval(5))
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
	bstCfg.IndexerAddr = tm.Config.BTCStakingTracker.IndexerAddr

	bsTracker := bst.NewBTCStakingTracker(
		tm.BTCClient,
		backend,
		tm.BabylonClient,
		&bstCfg,
		&commonCfg,
		zap.NewNop(),
		stakingTrackerMetrics,
		testutil.MakeTestBackend(t),
	)
	go bsTracker.Start()
	defer bsTracker.Stop()

	// wait for bootstrapping
	time.Sleep(5 * time.Second)

	// set up a finality provider
	_, fpSK := tm.CreateFinalityProvider(t)
	// set up a BTC delegation
	_, _, _ = tm.CreateBTCDelegation(t, fpSK)
	// set up a BTC delegation
	stakingSlashingInfo1, unbondingSlashingInfo1, stakerPrivKey1 := tm.CreateBTCDelegation(t, fpSK)

	// undelegate
	unbondingSlashingInfo, _ := tm.Undelegate(t, stakingSlashingInfo1, unbondingSlashingInfo1, stakerPrivKey1, func() { tm.CatchUpBTCLightClient(t) })

	// commit public randomness, vote and equivocate
	tm.VoteAndEquivocate(t, fpSK)

	// slashing tx will eventually enter mempool
	unbondingSlashingMsgTx, err := unbondingSlashingInfo.SlashingTx.ToMsgTx()
	require.NoError(t, err)
	unbondingSlashingMsgTxHash1 := unbondingSlashingMsgTx.TxHash()
	unbondingSlashingMsgTxHash := &unbondingSlashingMsgTxHash1

	btccParamsResp, err := tm.BabylonClient.BTCCheckpointParams()
	require.NoError(t, err)

	for i := 0; i <= int(btccParamsResp.Params.BtcConfirmationDepth); i++ {
		tm.mineBlock(t)
	}

	// slash unbonding tx will eventually enter mempool
	require.Eventually(t, func() bool {
		_, err := tm.BTCClient.GetRawTransaction(unbondingSlashingMsgTxHash)
		return err == nil
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	// mine a block that includes slashing tx
	require.Eventually(t, func() bool {
		txns, err := tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{unbondingSlashingMsgTxHash})
		if err != nil {
			t.Logf("error: %v", err)
			return false
		}
		return len(txns) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	tm.mineBlock(t)

	// ensure tx is eventually on Bitcoin
	require.Eventually(t, func() bool {
		res, err := tm.BTCClient.GetRawTransactionVerbose(unbondingSlashingMsgTxHash)
		if err != nil {
			return false
		}
		return len(res.BlockHash) > 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)
}

func TestSlasher_SlashingUnbonding_MultisigBTCDel(t *testing.T) {
	t.Parallel()
	// segwit is activated at height 300. It's needed by staking/slashing tx
	numMatureOutputs := uint32(300)

	tm := StartManager(t, WithNumMatureOutputs(numMatureOutputs), WithEpochInterval(5))
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
	bstCfg.IndexerAddr = tm.Config.BTCStakingTracker.IndexerAddr

	bsTracker := bst.NewBTCStakingTracker(
		tm.BTCClient,
		backend,
		tm.BabylonClient,
		&bstCfg,
		&commonCfg,
		zap.NewNop(),
		stakingTrackerMetrics,
		testutil.MakeTestBackend(t),
	)
	go bsTracker.Start()
	defer bsTracker.Stop()

	// wait for bootstrapping
	time.Sleep(5 * time.Second)

	// set up a finality provider
	_, fpSK := tm.CreateFinalityProvider(t)
	// set up a BTC delegation
	_, _, _ = tm.CreateMultisigBTCDelegation(t, fpSK)
	// set up a BTC delegation
	stakingSlashingInfo1, unbondingSlashingInfo1, stakerPrivKeys1 := tm.CreateMultisigBTCDelegation(t, fpSK)

	// undelegate
	unbondingSlashingInfo, _ := tm.UndelegateMultisigBTCDel(t, stakingSlashingInfo1, unbondingSlashingInfo1, stakerPrivKeys1, func() { tm.CatchUpBTCLightClient(t) })

	// commit public randomness, vote and equivocate
	tm.VoteAndEquivocate(t, fpSK)

	// slashing tx will eventually enter mempool
	unbondingSlashingMsgTx, err := unbondingSlashingInfo.SlashingTx.ToMsgTx()
	require.NoError(t, err)
	unbondingSlashingMsgTxHash1 := unbondingSlashingMsgTx.TxHash()
	unbondingSlashingMsgTxHash := &unbondingSlashingMsgTxHash1

	btccParamsResp, err := tm.BabylonClient.BTCCheckpointParams()
	require.NoError(t, err)

	for i := 0; i <= int(btccParamsResp.Params.BtcConfirmationDepth); i++ {
		tm.mineBlock(t)
	}

	// slash unbonding tx will eventually enter mempool
	require.Eventually(t, func() bool {
		_, err := tm.BTCClient.GetRawTransaction(unbondingSlashingMsgTxHash)
		return err == nil
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	// mine a block that includes slashing tx
	require.Eventually(t, func() bool {
		txns, err := tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{unbondingSlashingMsgTxHash})
		if err != nil {
			t.Logf("error: %v", err)
			return false
		}
		return len(txns) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	tm.mineBlock(t)

	// ensure tx is eventually on Bitcoin
	require.Eventually(t, func() bool {
		res, err := tm.BTCClient.GetRawTransactionVerbose(unbondingSlashingMsgTxHash)
		if err != nil {
			return false
		}
		return len(res.BlockHash) > 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)
}

func TestSlasher_Bootstrapping(t *testing.T) {
	t.Parallel()
	// segwit is activated at height 300. It's needed by staking/slashing tx
	numMatureOutputs := uint32(300)

	tm := StartManager(t, WithNumMatureOutputs(numMatureOutputs), WithEpochInterval(5))
	defer tm.Stop(t)
	// start WebSocket connection with Babylon for subscriber services
	err := tm.BabylonClient.Start()
	require.NoError(t, err)
	// Insert all existing BTC headers to babylon node
	tm.CatchUpBTCLightClient(t)

	// set up a finality provider
	_, fpSK := tm.CreateFinalityProvider(t)
	// set up a BTC delegation
	stakingSlashingInfo, _, _ := tm.CreateBTCDelegation(t, fpSK)

	// commit public randomness, vote and equivocate
	tm.VoteAndEquivocate(t, fpSK)

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
	bstCfg.IndexerAddr = tm.Config.BTCStakingTracker.IndexerAddr

	bsTracker := bst.NewBTCStakingTracker(
		tm.BTCClient,
		backend,
		tm.BabylonClient,
		&bstCfg,
		&commonCfg,
		zap.NewNop(),
		stakingTrackerMetrics,
		testutil.MakeTestBackend(t),
	)

	// bootstrap BTC staking tracker
	err = bsTracker.Bootstrap(0)
	require.NoError(t, err)

	// slashing tx will eventually enter mempool
	slashingMsgTx, err := stakingSlashingInfo.SlashingTx.ToMsgTx()
	require.NoError(t, err)
	slashingMsgTxHash1 := slashingMsgTx.TxHash()
	slashingMsgTxHash := &slashingMsgTxHash1

	// mine a block that includes slashing tx
	require.Eventually(t, func() bool {
		txns, err := tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{slashingMsgTxHash})
		if err != nil {
			t.Logf("error: %v", err)
			return false
		}
		return len(txns) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	minedBlock := tm.mineBlock(t)
	// ensure 2 txs will eventually be received (staking tx and slashing tx)
	require.Equal(t, 2, len(minedBlock.Transactions))
}

func TestOpReturnBurn(t *testing.T) {
	t.Parallel()
	numMatureOutputs := uint32(300)

	tm := StartManager(t, WithNumMatureOutputs(numMatureOutputs), WithEpochInterval(5))
	defer tm.Stop(t)

	tx := wire.NewMsgTx(wire.TxVersion)
	script, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_RETURN).
		AddData([]byte("test")).
		Script()
	require.NoError(t, err)

	tx.AddTxOut(wire.NewTxOut(1000, script))

	res, err := tm.BTCClient.FundRawTransaction(tx, btcjson.FundRawTransactionOpts{}, nil)
	require.NoError(t, err)

	signedTx, allSigned, err := tm.BTCClient.SignRawTransactionWithWallet(res.Transaction)
	require.NoError(t, err)
	require.True(t, allSigned)

	hash, err := tm.BTCClient.SendRawTransaction(signedTx, true)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unspendable output exceeds maximum configured by user (maxburnamount)")

	burnLimitBTC := btcutil.Amount(900).ToBTC()
	hash, err = tm.BTCClient.SendRawTransactionWithBurnLimit(signedTx, true, burnLimitBTC)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Unspendable output exceeds maximum configured by user (maxburnamount)")

	burnLimitBTC = btcutil.Amount(1001).ToBTC()
	hash, err = tm.BTCClient.SendRawTransactionWithBurnLimit(signedTx, true, burnLimitBTC)
	require.NoError(t, err)

	tm.mineBlock(t)

	require.Eventually(t, func() bool {
		res, err := tm.BTCClient.GetRawTransactionVerbose(hash)
		if err != nil {
			return false
		}
		return len(res.BlockHash) > 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)
}
