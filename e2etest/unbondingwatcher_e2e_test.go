//go:build e2e
// +build e2e

package e2etest

import (
	"context"
	"encoding/hex"
	"fmt"
	bbnclient "github.com/babylonlabs-io/babylon/client/client"
	btcstakingtypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	"github.com/babylonlabs-io/vigilante/types"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
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
	bstCfg.IndexerAddr = tm.Config.BTCStakingTracker.IndexerAddr
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
		txns, err := tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{&unbondingTxHash})
		require.NoError(t, err)
		return len(txns) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	minedBlock := tm.mineBlock(t)
	require.Equal(t, 2, len(minedBlock.Transactions))

	tm.CatchUpBTCLightClient(t)

	require.Eventually(t, func() bool {
		resp, err := tm.BabylonClient.BTCDelegation(stakingSlashingInfo.StakingTx.TxHash().String())
		require.NoError(t, err)

		tm.mineBlock(t)

		// TODO: Add field for staker signature in BTCDelegation query to check it directly,
		// for now it is enough to check that delegation is not active, as if unbonding was reported
		// delegation will be deactivated
		return !resp.BtcDelegation.Active
	}, 2*time.Minute, 3*time.Second)
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
	bstCfg.IndexerAddr = tm.Config.BTCStakingTracker.IndexerAddr
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
		txns, err := tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{&stakingMsgTxHash})
		require.NoError(t, err)
		return len(txns) == 1
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
	bstCfg.IndexerAddr = tm.Config.BTCStakingTracker.IndexerAddr
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
		txns, err := tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{&stakingMsgTxHash})
		require.NoError(t, err)
		return len(txns) == 1
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
		txns, err := tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{&unbondingTxHash})
		require.NoError(t, err)

		return len(txns) == 1
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

		tm.mineBlock(t)

		return resp.BtcDelegation.StatusDesc == btcstakingtypes.BTCDelegationStatus_UNBONDED.String()
	}, eventuallyWaitTimeOut, 3*eventuallyPollTime)
}

func TestUnbondingLoaded(t *testing.T) {
	t.Parallel()
	numMatureOutputs := uint32(400)
	tm := StartManager(t, numMatureOutputs, defaultEpochInterval)
	defer tm.Stop(t)
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
	bstCfg.CheckDelegationsInterval = 3 * time.Second
	bstCfg.IndexerAddr = tm.Config.BTCStakingTracker.IndexerAddr

	trackerMetrics := metrics.NewBTCStakingTrackerMetrics()
	bsTracker := bst.NewBTCStakingTracker(
		tm.BTCClient,
		backend,
		tm.BabylonClient,
		&bstCfg,
		&commonCfg,
		logger,
		trackerMetrics,
	)
	bsTracker.Start()
	defer bsTracker.Stop()

	// set up a finality provider
	_, fpSK := tm.CreateFinalityProvider(t)
	signerAddr := tm.BabylonClient.MustGetAddr()
	addr := sdk.MustAccAddressFromBech32(signerAddr)

	bsParams, err := tm.BabylonClient.BTCStakingParams()
	require.NoError(t, err)

	numStakers := 150
	stakers := make([]*Staker, 0, numStakers)
	// loop for creating staking txs
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
			staker.CreateStakingTx(t, tm, fpSK, topUTXO, addr, bsParams)
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
	tm.BitcoindHandler.GenerateBlocks(1000)

	for _, staker := range stakers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			staker.CreateUnbondingData(t, tm, fpSK, bsParams)
			staker.AddCov(t, tm, signerAddr, fpSK)
			staker.PrepareUnbondingTx(t, tm)
			staker.SendTxAndWait(t, tm, staker.unbondingSlashingInfo.UnbondingTx)
		}()
	}

	wg.Wait()

	tm.BitcoindHandler.GenerateBlocks(1000)
	tm.CatchUpBTCLightClient(t)

	t.Logf("waiting for indexer to catch up")
	require.Eventually(t, func() bool {
		indexerTip, err := tm.Electrs.GetTipHeight(bstCfg.IndexerAddr)
		require.NoError(t, err)

		btcTip, err := tm.BTCClient.GetBestBlock()
		require.NoError(t, err)

		return indexerTip == int(btcTip)
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	// async send delegations
	go func() {
		for _, staker := range stakers {
			go func() {
				staker.SendDelegation(t, tm, signerAddr, fpSK.PubKey(), bsParams)
				staker.SendCovSig(t, tm)
				staker.stakeReportedAt = time.Now()
			}()
			time.Sleep(200 * time.Millisecond)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	go func(ctx context.Context) {
		ticker := time.NewTicker(5 * time.Second)
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

	activeMap := make(map[string]struct{})
	require.Eventually(t, func() bool {
		for _, staker := range stakers {
			hash := staker.stakingMsgTxHash.String()
			if _, ok := activeMap[hash]; ok {
				continue
			}
			resp, err := tm.BabylonClient.BTCDelegation(staker.stakingMsgTxHash.String())
			if err != nil {
				continue
			}

			if resp.BtcDelegation.StatusDesc == btcstakingtypes.BTCDelegationStatus_UNBONDED.String() {
				staker.unbondingDetectedAt = time.Now()
				activeMap[hash] = struct{}{}
			}
		}

		detected := promtestutil.ToFloat64(trackerMetrics.DetectedUnbondingTransactionsCounter)
		reported := promtestutil.ToFloat64(trackerMetrics.ReportedUnbondingTransactionsCounter)
		failed := promtestutil.ToFloat64(trackerMetrics.FailedReportedUnbondingTransactions)

		t.Logf("detected: %v, reported: %v, failed: %v", detected, reported, failed)
		t.Logf("num in activeMap: %v", len(activeMap))

		return len(activeMap) == len(stakers)
	}, 135*time.Minute, 700*time.Millisecond)

	cancel()

	t.Logf("avg time to detect unbonding: %v", avgTimeToDetectUnbonding(stakers))
}

// TestActivatingDelegationWithTwoTrackers - activates a delegation with two staking trackers
// making sure that after delegation has been activated by one tracker, the other tracker
// cleans up the delegation from its internal state
func TestActivatingDelegationWithTwoTrackers(t *testing.T) {
	t.Parallel()
	// segwit is activated at height 300. It's necessary for staking/slashing tx
	numMatureOutputs := uint32(300)

	tm := StartManager(t, numMatureOutputs, defaultEpochInterval)
	defer tm.Stop(t)
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
	bstCfg.IndexerAddr = tm.Config.BTCStakingTracker.IndexerAddr
	stakingTrackerMetrics := metrics.NewBTCStakingTrackerMetrics()

	commonCfg2 := config.DefaultCommonConfig()
	bstCfg2 := config.DefaultBTCStakingTrackerConfig()
	bstCfg2.CheckDelegationsInterval = 1 * time.Second
	bstCfg2.IndexerAddr = tm.Config.BTCStakingTracker.IndexerAddr
	stakingTrackerMetrics2 := metrics.NewBTCStakingTrackerMetrics()

	prvKey, _, address := testdata.KeyTestPubAddr()

	err = tm.BabylonClient.Provider().Keybase.ImportPrivKeyHex(
		"test-2",
		hex.EncodeToString(prvKey.Bytes()),
		"secp256k1",
	)
	require.NoError(t, err)

	cfg := defaultVigilanteConfig()
	cfg.BTC.Endpoint = tm.Config.BTC.Endpoint
	cfg.BTCStakingTracker.IndexerAddr = tm.Config.BTCStakingTracker.IndexerAddr
	cfg.Babylon.KeyDirectory = tm.Config.Babylon.KeyDirectory
	cfg.Babylon.GasAdjustment = tm.Config.Babylon.GasAdjustment
	cfg.Babylon.Key = "test-2"
	cfg.Babylon.RPCAddr = tm.Config.Babylon.RPCAddr
	cfg.Babylon.GRPCAddr = tm.Config.Babylon.GRPCAddr

	babylonClient, err := bbnclient.New(&cfg.Babylon, nil)
	require.NoError(t, err)

	msg := banktypes.NewMsgSend(
		sdk.MustAccAddressFromBech32(tm.BabylonClient.MustGetAddr()),
		address,
		sdk.NewCoins(sdk.NewInt64Coin("ubbn", 100_000_000)),
	)

	_, err = tm.BabylonClient.ReliablySendMsg(context.Background(), msg, nil, nil)
	require.NoError(t, err)

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

	btcClient := initBTCClientWithSubscriber(t, tm.Config)

	bsTracker2 := bst.NewBTCStakingTracker(
		btcClient,
		btcNotifier,
		babylonClient,
		&bstCfg2,
		&commonCfg2,
		zap.NewNop(),
		stakingTrackerMetrics2,
	)
	bsTracker2.Start()
	defer bsTracker2.Stop()

	// set up a finality provider
	_, fpSK := tm.CreateFinalityProvider(t)
	// set up a BTC delegation
	stakingMsgTx, stakingSlashingInfo, _, _ := tm.CreateBTCDelegationWithoutIncl(t, fpSK)
	stakingMsgTxHash := stakingMsgTx.TxHash()

	// send staking tx to Bitcoin node's mempool
	_, err = tm.BTCClient.SendRawTransaction(stakingMsgTx, true)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		txns, err := tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{&stakingMsgTxHash})
		require.NoError(t, err)
		return len(txns) == 1
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

	require.Eventually(t, func() bool {
		resp, err := tm.BabylonClient.BTCDelegation(stakingSlashingInfo.StakingTx.TxHash().String())
		require.NoError(t, err)

		return resp.BtcDelegation.Active
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	// tracker that activated delegation should decrease the number of delegations in "VERIFIED" state
	// after successful activation. The other tracker should do that as well.
	require.Eventually(t, func() bool {
		return promtestutil.ToFloat64(stakingTrackerMetrics2.NumberOfVerifiedDelegations) == 0 &&
			promtestutil.ToFloat64(stakingTrackerMetrics.NumberOfVerifiedDelegations) == 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)
}

func TestUnbondingWatcherCensorship(t *testing.T) {
	t.Parallel()
	numMatureOutputs := uint32(300)

	tm := StartManager(t, numMatureOutputs, defaultEpochInterval)
	defer tm.Stop(t)
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
	bstCfg.IndexerAddr = tm.Config.BTCStakingTracker.IndexerAddr
	stakingTrackerMetrics := metrics.NewBTCStakingTrackerMetrics()

	cfg := defaultVigilanteConfig()
	cfg.BTC.Endpoint = tm.Config.BTC.Endpoint
	cfg.BTCStakingTracker.IndexerAddr = tm.Config.BTCStakingTracker.IndexerAddr
	cfg.Babylon.KeyDirectory = tm.Config.Babylon.KeyDirectory
	cfg.Babylon.GasAdjustment = tm.Config.Babylon.GasAdjustment
	cfg.Babylon.Key = "test-spending-key"
	cfg.Babylon.RPCAddr = tm.Config.Babylon.RPCAddr
	cfg.Babylon.GRPCAddr = tm.Config.Babylon.GRPCAddr
	cfg.Babylon.BlockTimeout = 10 * time.Millisecond // very short timeout to test censorship

	babylonClient, err := bbnclient.New(&cfg.Babylon, nil) // new client only for the tracker
	require.NoError(t, err)

	bsTracker := bst.NewBTCStakingTracker(
		tm.BTCClient,
		backend,
		babylonClient,
		&bstCfg,
		&commonCfg,
		zap.NewNop(),
		stakingTrackerMetrics,
	)
	bsTracker.Start()
	defer bsTracker.Stop()

	_, fpSK := tm.CreateFinalityProvider(t)
	stakingSlashingInfo, unbondingSlashingInfo, delSK := tm.CreateBTCDelegation(t, fpSK)

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
		txns, err := tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{&unbondingTxHash})
		require.NoError(t, err)
		return len(txns) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	minedBlock := tm.mineBlock(t)
	require.Equal(t, 2, len(minedBlock.Transactions))

	tm.CatchUpBTCLightClient(t)

	require.Eventually(t, func() bool {
		tm.mineBlock(t)

		return promtestutil.ToFloat64(stakingTrackerMetrics.UnbondingCensorshipGaugeVec.
			WithLabelValues(stakingSlashingInfo.StakingTx.TxHash().String())) == 1
	}, time.Minute, 3*time.Second)
}
