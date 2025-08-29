//go:build e2e

package e2etest

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/babylonlabs-io/babylon/v3/testutil/datagen"
	"github.com/babylonlabs-io/vigilante/testutil"
	"github.com/babylonlabs-io/vigilante/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/btcsuite/btcd/chaincfg"

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

func TestSlasher_MultiStaking(t *testing.T) {
	t.Parallel()
	tm := StartManager(t,
		WithNumMatureOutputs(300),
		WithEpochInterval(5),
		WithNumCovenants(2))

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
		zaptest.NewLogger(t),
		stakingTrackerMetrics,
		testutil.MakeTestBackend(t),
	)
	go bsTracker.Start()
	defer bsTracker.Stop()

	// wait for bootstrapping
	time.Sleep(5 * time.Second)

	signerAddr := tm.BabylonClient.MustGetAddr()
	addr := sdk.MustAccAddressFromBech32(signerAddr)

	bsParams, err := tm.BabylonClient.BTCStakingParams()
	require.NoError(t, err)

	contractAddr := tm.DeployCwContract(t)
	consumer := datagen.GenRandomRollupRegister(r, contractAddr)
	tm.RegisterBSN(t, consumer, contractAddr)

	// set up a finality provider
	_, fp1SK := tm.CreateFinalityProvider(t)
	_, fp2SK := tm.CreateFinalityProviderBSN(t, consumer.ConsumerId)
	staker := Staker{}
	fpSKs := []*btcec.PublicKey{fp1SK.PubKey(), fp2SK.PubKey()}

	topUnspentResult, _, err := tm.BTCClient.GetHighUTXOAndSum()
	require.NoError(t, err)
	topUTXO, err := types.NewUTXO(topUnspentResult, regtestParams)
	require.NoError(t, err)

	staker.CreateStakingTx(t, tm, fpSKs, topUTXO, addr, bsParams)
	staker.SendTxAndWait(t, tm, staker.stakingSlashingInfo.StakingTx)

	var res *btcjson.TxRawResult
	require.Eventually(t, func() bool {
		tm.mineBlock(t)

		res, err = tm.BTCClient.GetRawTransactionVerbose(staker.stakingMsgTxHash)
		return err == nil
	}, eventuallyWaitTimeOut, eventuallyPollTime)
	tm.mineBlock(t)

	blockHash, err := chainhash.NewHashFromStr(res.BlockHash)
	require.NoError(t, err)
	block, err := tm.TestRpcClient.GetBlock(blockHash)
	require.NoError(t, err)
	staker.stakingTxInfo = getTxInfoByHash(t, staker.stakingMsgTxHash, block)

	tm.mineBlock(t)

	staker.CreateUnbondingData(t, tm, fpSKs, bsParams)
	staker.AddCov(t, tm, signerAddr, fpSKs)
	staker.PrepareUnbondingTx(t, tm)

	tm.mineBlock(t)
	tm.CatchUpBTCLightClient(t)

	staker.SendDelegation(t, tm, signerAddr, fpSKs, bsParams)
	staker.SendCovSig(t, tm)

	// commit public randomness, vote and equivocate
	tm.VoteAndEquivocate(t, fp1SK)

	// slashing tx will eventually enter mempool
	slashingMsgTx, err := staker.stakingSlashingInfo.SlashingTx.ToMsgTx()
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

func TestSlasher_Loaded_MultiStaking(t *testing.T) {
	t.Parallel()
	tm := StartManager(t,
		WithNumMatureOutputs(300),
		WithEpochInterval(5),
		WithNumCovenants(2))

	defer tm.Stop(t)
	err := tm.BabylonClient.Start()
	require.NoError(t, err)
	tm.CatchUpBTCLightClient(t)

	backend, err := btcclient.NewNodeBackend(
		btcclient.ToBitcoindConfig(tm.Config.BTC),
		&chaincfg.RegressionNetParams,
		&btcclient.EmptyHintCache{},
	)
	require.NoError(t, err)

	err = backend.Start()
	require.NoError(t, err)

	numStakers := 50
	commonCfg := config.DefaultCommonConfig()
	bstCfg := config.DefaultBTCStakingTrackerConfig()
	bstCfg.CheckDelegationsInterval = 1 * time.Second
	stakingTrackerMetrics := metrics.NewBTCStakingTrackerMetrics()
	bstCfg.IndexerAddr = tm.Config.BTCStakingTracker.IndexerAddr
	bstCfg.MaxSlashingConcurrency = 15 // we throttle slashing on purpose to we can test a restart

	tempDirName := t.TempDir()
	cfg := config.DefaultDBConfig()
	cfg.DBPath = tempDirName
	dbBackend, err := cfg.GetDBBackend()
	require.NoError(t, err)

	t.Cleanup(func() {
		err := dbBackend.Close()
		require.NoError(t, err)
	})

	bsTracker := bst.NewBTCStakingTracker(
		tm.BTCClient,
		backend,
		tm.BabylonClient,
		&bstCfg,
		&commonCfg,
		zaptest.NewLogger(t),
		stakingTrackerMetrics,
		dbBackend,
	)
	go bsTracker.Start()
	defer bsTracker.Stop()

	// wait for bootstrapping
	time.Sleep(5 * time.Second)

	signerAddr := tm.BabylonClient.MustGetAddr()
	addr := sdk.MustAccAddressFromBech32(signerAddr)

	bsParams, err := tm.BabylonClient.BTCStakingParams()
	require.NoError(t, err)

	contractAddr := tm.DeployCwContract(t)
	consumer := datagen.GenRandomRollupRegister(r, contractAddr)
	tm.RegisterBSN(t, consumer, contractAddr)

	// set up a finality provider
	_, fp1SK := tm.CreateFinalityProvider(t)
	_, fp2SK := tm.CreateFinalityProviderBSN(t, consumer.ConsumerId)
	fpSKs := []*btcec.PublicKey{fp1SK.PubKey(), fp2SK.PubKey()}

	stakers := make([]*Staker, 0, numStakers)
	for i := 0; i < numStakers; i++ {
		stakers = append(stakers, &Staker{})
	}

	var wg sync.WaitGroup
	for i, staker := range stakers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			topUnspentResult, err := tm.BTCClient.ListUnspent()
			require.NoError(t, err)

			unspentIdx := i % len(topUnspentResult)
			topUTXO, err := types.NewUTXO(&topUnspentResult[unspentIdx], regtestParams)
			require.NoError(t, err)
			staker.CreateStakingTx(t, tm, fpSKs, topUTXO, addr, bsParams)
			staker.SendTxAndWait(t, tm, staker.stakingSlashingInfo.StakingTx)

			var res *btcjson.TxRawResult
			require.Eventually(t, func() bool {
				res, err = tm.BTCClient.GetRawTransactionVerbose(staker.stakingMsgTxHash)
				return err == nil
			}, eventuallyWaitTimeOut, eventuallyPollTime)
			blockHash, err := chainhash.NewHashFromStr(res.BlockHash)
			require.NoError(t, err)
			block, err := tm.TestRpcClient.GetBlock(blockHash)
			require.NoError(t, err)
			staker.stakingTxInfo = getTxInfoByHash(t, staker.stakingMsgTxHash, block)
		}()
		tm.mineBlock(t)
	}

	wg.Wait()
	tm.mineBlock(t)

	for _, staker := range stakers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			staker.CreateUnbondingData(t, tm, fpSKs, bsParams)
			staker.AddCov(t, tm, signerAddr, fpSKs)
			staker.PrepareUnbondingTx(t, tm)
		}()
	}
	wg.Wait()

	tm.BitcoindHandler.GenerateBlocks(10)
	tm.CatchUpBTCLightClient(t)

	for _, staker := range stakers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tm.mineBlock(t)
			staker.SendDelegation(t, tm, signerAddr, fpSKs, bsParams)
			staker.SendCovSig(t, tm)
		}()
		time.Sleep(100 * time.Millisecond)
	}
	wg.Wait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func(ctx context.Context) {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				tm.mineBlock(t)
			case <-ctx.Done():
				return
			}
		}
	}(ctx)

	tm.CatchUpBTCLightClient(t)

	// commit public randomness, vote and equivocate
	tm.VoteAndEquivocate(t, fp1SK)

	allSlashingMsgTxHashes := map[chainhash.Hash]bool{}
	for _, staker := range stakers {
		slashingMsgTx, err := staker.stakingSlashingInfo.SlashingTx.ToMsgTx()
		require.NoError(t, err)
		allSlashingMsgTxHashes[slashingMsgTx.TxHash()] = false
	}

	numSlashed := 0
	t1 := time.Now()
	require.Eventually(t, func() bool {
		for slashingMsgTxHash := range allSlashingMsgTxHashes {
			if allSlashingMsgTxHashes[slashingMsgTxHash] {
				continue
			}

			txns, err := tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{&slashingMsgTxHash})
			if err != nil {
				t.Logf("error: %v", err)
				return false
			}

			// let's stop the slasher and see if we correctly bootstrap from where we left off,
			// even with abrupt shutdown we should finish all the slashing
			if len(allSlashingMsgTxHashes)/2 == numSlashed {
				bsTracker.Stop()
				dbBackend.Close()
				dbBackend, err = cfg.GetDBBackend()
				require.NoError(t, err)
				bsTracker = bst.NewBTCStakingTracker(
					tm.BTCClient,
					backend,
					tm.BabylonClient,
					&bstCfg,
					&commonCfg,
					zaptest.NewLogger(t),
					stakingTrackerMetrics,
					dbBackend,
				)
				go func() {
					err := bsTracker.Bootstrap(0)
					require.NoError(t, err)
				}()
			}

			if len(txns) == 1 {
				allSlashingMsgTxHashes[slashingMsgTxHash] = true
				numSlashed++
				t.Logf("num slashed: %d, %s", numSlashed, slashingMsgTxHash.String())
			}
		}
		return numSlashed == len(stakers)
	}, 5*eventuallyWaitTimeOut, eventuallyPollTime)

	t.Logf("time elapsed to finish slashing: %v", time.Since(t1))
}
