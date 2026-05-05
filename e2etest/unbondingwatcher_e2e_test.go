//go:build e2e

package e2etest

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/babylonlabs-io/vigilante/testutil"
	"github.com/btcsuite/btcd/btcec/v2"

	bbnclient "github.com/babylonlabs-io/babylon/v4/client/client"
	bbn "github.com/babylonlabs-io/babylon/v4/types"
	btcstakingtypes "github.com/babylonlabs-io/babylon/v4/x/btcstaking/types"
	"github.com/babylonlabs-io/vigilante/types"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/babylon/v4/btcstaking"
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
		testutil.MakeTestBackend(t),
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

	tm := StartManager(t, WithNumMatureOutputs(numMatureOutputs), WithEpochInterval(defaultEpochInterval))
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
		testutil.MakeTestBackend(t),
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
	t.Parallel()
	// segwit is activated at height 300. It's necessary for staking/slashing tx
	numMatureOutputs := uint32(300)

	tm := StartManager(t, WithNumMatureOutputs(numMatureOutputs), WithEpochInterval(defaultEpochInterval))
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
		testutil.MakeTestBackend(t),
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
	tm := StartManager(t, WithNumMatureOutputs(numMatureOutputs), WithEpochInterval(defaultEpochInterval))
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
		testutil.MakeTestBackend(t),
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

	t1 := time.Now()
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
			staker.CreateStakingTx(t, tm, []*btcec.PublicKey{fpSK.PubKey()}, topUTXO, addr, bsParams)
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
	timeCreateStakingTx := time.Since(t1)

	tm.BitcoindHandler.GenerateBlocks(100)

	t2 := time.Now()
	for _, staker := range stakers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			staker.CreateUnbondingData(t, tm, []*btcec.PublicKey{fpSK.PubKey()}, bsParams)
			staker.AddCov(t, tm, signerAddr, []*btcec.PublicKey{fpSK.PubKey()})
			staker.PrepareUnbondingTx(t, tm)
			staker.SendTxAndWait(t, tm, staker.unbondingSlashingInfo.UnbondingTx)
		}()
	}

	wg.Wait()
	timeCreateUnbondingTx := time.Since(t2)

	tm.BitcoindHandler.GenerateBlocks(100)
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
		t3 := time.Now()
		for _, staker := range stakers {
			go func() {
				staker.SendDelegation(t, tm, signerAddr, []*btcec.PublicKey{fpSK.PubKey()}, bsParams)
				staker.SendCovSig(t, tm)
				staker.stakeReportedAt = time.Now()
			}()
			time.Sleep(200 * time.Millisecond)
		}
		timeSendDelegation := time.Since(t3)
		t.Logf("time to send delegations: %v", timeSendDelegation)
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
	t.Logf("time to create staking txs: %v", timeCreateStakingTx)
	t.Logf("time to create unbonding txs: %v", timeCreateUnbondingTx)
}

// TestActivatingDelegationWithTwoTrackers - activates a delegation with two staking trackers
// making sure that after delegation has been activated by one tracker, the other tracker
// cleans up the delegation from its internal state
func TestActivatingDelegationWithTwoTrackers(t *testing.T) {
	t.Parallel()
	// segwit is activated at height 300. It's necessary for staking/slashing tx
	numMatureOutputs := uint32(300)

	tm := StartManager(t, WithNumMatureOutputs(numMatureOutputs), WithEpochInterval(defaultEpochInterval))
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
		testutil.MakeTestBackend(t),
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
		testutil.MakeTestBackend(t),
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

// TestStakeExpansionFlow tests the complete stake expansion flow.
func TestStakeExpansionFlow(t *testing.T) {
	t.Parallel()
	// segwit is activated at height 300. It's necessary for staking/slashing tx
	numMatureOutputs := uint32(300)

	tm := StartManager(t, WithNumMatureOutputs(numMatureOutputs), WithEpochInterval(defaultEpochInterval))
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
		testutil.MakeTestBackend(t),
	)
	bsTracker.Start()
	defer bsTracker.Stop()

	// set up a finality provider
	_, fpSK := tm.CreateFinalityProvider(t)

	// Step 1: Create initial BTC delegation (the one that will be expanded)
	originalStakingSlashingInfo, _, delSK := tm.CreateBTCDelegation(t, fpSK)
	originalStakingTxHash := originalStakingSlashingInfo.StakingTx.TxHash()

	// Verify original delegation is active
	var originalDel *btcstakingtypes.BTCDelegationResponse
	require.Eventually(t, func() bool {
		resp, err := tm.BabylonClient.BTCDelegation(originalStakingTxHash.String())
		require.NoError(t, err)
		originalDel = resp.BtcDelegation
		return resp.BtcDelegation.Active
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	// Step 2: Create stake expansion delegation (simulating MsgBtcStakeExpand)
	// This creates a new delegation that references the original staking transaction
	expansionStakingMsgTx, fundingMsgTx, expansionStakingSlashingInfo, expansionUnbondingSlashingInfo, _ := tm.CreateBTCStakeExpansion(t, fpSK, originalDel.TotalSat, originalStakingTxHash.String(), originalDel.StakingOutputIdx)
	expansionStakingTxHash := expansionStakingMsgTx.TxHash()

	// Step 3: Add covenant signatures for the expansion delegation
	signerAddr := tm.BabylonClient.MustGetAddr()
	expansionStakingMsgTxHash := expansionStakingMsgTx.TxHash()

	slashingSpendPath, err := expansionStakingSlashingInfo.StakingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)

	unbondingSlashingPathSpendInfo, err := expansionUnbondingSlashingInfo.UnbondingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)

	tm.addCovenantSigStkExp(
		t,
		signerAddr,
		expansionStakingMsgTx,
		&expansionStakingMsgTxHash,
		fpSK,
		slashingSpendPath,
		expansionStakingSlashingInfo,
		expansionUnbondingSlashingInfo,
		unbondingSlashingPathSpendInfo,
		0,
		originalStakingSlashingInfo,
		fundingMsgTx.TxOut[0],
	)

	// Step 4: Add witness to the stake expansion transaction before sending to Bitcoin
	// Get the original and funding outputs for witness generation
	originalStakingUnbondingPathSpendInfo, err := originalStakingSlashingInfo.StakingInfo.UnbondingPathSpendInfo()

	// sign with delegator key
	stakerSig, err := btcstaking.SignTxForFirstScriptSpendWithTwoInputsFromScript(
		expansionStakingMsgTx,
		originalStakingSlashingInfo.StakingInfo.StakingOutput,
		fundingMsgTx.TxOut[0],
		delSK,
		originalStakingUnbondingPathSpendInfo.GetPkScriptPath(),
	)
	require.NoError(t, err)

	// Get covenant sigs from stake expansion delegation
	resp, err := tm.BabylonClient.BTCDelegation(expansionStakingTxHash.String())
	require.NoError(t, err)

	covenantSigs := resp.BtcDelegation.StkExp.PreviousStkCovenantSigs
	witness, err := originalStakingUnbondingPathSpendInfo.CreateUnbondingPathWitness(
		[]*schnorr.Signature{covenantSigs[0].Sig.MustToBTCSig()},
		stakerSig,
	)
	require.NoError(t, err)
	expansionStakingMsgTx.TxIn[0].Witness = witness

	// Sign the funding input with wallet
	signedTx, complete, err := tm.BTCClient.SignRawTransactionWithWallet(expansionStakingMsgTx)
	require.NoError(t, err)
	require.True(t, complete, "Transaction signing incomplete")

	// Step 5: send expansion staking tx to Bitcoin node's mempool
	_, err = tm.BTCClient.SendRawTransaction(signedTx, true)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		txns, err := tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{&expansionStakingTxHash})
		require.NoError(t, err)
		return len(txns) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	// Step 6: Mine the expansion transaction
	mBlock := tm.mineBlock(t)
	require.Equal(t, 2, len(mBlock.Transactions))

	// wait until expansion staking tx is on Bitcoin
	require.Eventually(t, func() bool {
		_, err := tm.BTCClient.GetRawTransaction(&expansionStakingTxHash)
		return err == nil
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	// Step 7: Wait for k-deep confirmation and let vigilante detect the stake expansion
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Insert k empty blocks to Bitcoin to achieve required depth
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

	// Step 8: Verify that the vigilante detected the stake expansion
	require.Eventually(t, func() bool {
		return promtestutil.ToFloat64(stakingTrackerMetrics.DetectedUnbondedStakeExpansionCounter) >= 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	// Step 9: Verify the expansion delegation becomes active
	require.Eventually(t, func() bool {
		resp, err := tm.BabylonClient.BTCDelegation(expansionStakingTxHash.String())
		require.NoError(t, err)
		return resp.BtcDelegation.Active
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	// Verify the original delegation is unbonded
	require.Eventually(t, func() bool {
		resp, err := tm.BabylonClient.BTCDelegation(originalStakingTxHash.String())
		require.NoError(t, err)

		tm.mineBlock(t)

		return resp.BtcDelegation.StatusDesc == btcstakingtypes.BTCDelegationStatus_UNBONDED.String()
	}, eventuallyWaitTimeOut, 3*eventuallyPollTime)

	t.Logf("Stake expansion flow completed successfully: original tx %s expanded to %s",
		originalStakingTxHash.String(), expansionStakingTxHash.String())
}

// TestStakeExpansionParentUnbondAtOneDeepWhenChildUnbondedEarly drives the
// vigilante-side stake-expansion-with-unbonded-child fix end-to-end on a
// real bitcoind + babylond pair.
//
// Scenario (mirrors the post-upgrade live-attack regression in babylon's
// upgrades_v4_3_phantom_test.go):
//
//  1. Parent ACTIVE + child VERIFIED stake-expansion. Covenants signed for both.
//  2. Broadcast the expansion staking tx to BTC and mine 1 block. Parent's
//     UTXO is now spent on BTC; the child's staking output exists on BTC. The
//     parent stays ACTIVE on babylon — no parent-unbond has been reported yet.
//  3. Broadcast the child's unbonding tx to BTC and mine 1 block, then submit
//     MsgBTCUndelegate for the CHILD using the unbonding tx as
//     StakeSpendingTx. Babylon writes DelegatorUnbondingInfo on the child and
//     marks it UNBONDED — this is the "poison" pattern that creates the
//     phantom-voting-power state.
//  4. With babylon now reporting child.IsUnbonded=true, the watcher sees the
//     parent's UTXO spend and MUST skip the wait-for-k-deep expansion-
//     activation branch (the gating fix). It builds a 1-deep merkle proof for
//     the parent's spend and submits MsgBTCUndelegate for the parent.
//  5. Parent transitions to UNBONDED on babylon — at depth=2 of the expansion
//     tx, well under k. No additional empty blocks are mined for k-deep.
//
// Without the vigilante fix, step 4 would loop indefinitely in
// waitForRequiredDepth (the watcher would treat the parent's spend as an
// expansion-activation), and the parent would never be reported as unbonded
// at sub-k depth. The test is the regression-pin for that hang.
//
// Note: this test depends on the running babylond accepting the parent's
// MsgBTCUndelegate at <k-deep when the child is unbonded (the v4.3 babylon
// fix). If the babylond image does not yet carry that fix, the parent
// transition will not happen until k blocks are mined; in that case the
// final require.Eventually block will time out, signaling that the babylon
// image needs to be bumped.
func TestStakeExpansionParentUnbondAtOneDeepWhenChildUnbondedEarly(t *testing.T) {
	t.Parallel()
	// segwit is activated at height 300. It's necessary for staking/slashing tx
	numMatureOutputs := uint32(300)

	// The end-to-end assertion requires babylond to accept the parent's
	// MsgBTCUndelegate at sub-k depth when the child stake-expansion is
	// already unbonded — that server-side change ships in babylon v4.3+.
	// go.mod still pins v4.2.x for the Go SDK; we only override the docker
	// image tag so this test exercises the v4.3 server behavior. Drop the
	// override once the babylon Go dep is bumped.
	tm := StartManager(t,
		WithNumMatureOutputs(numMatureOutputs),
		WithEpochInterval(defaultEpochInterval),
		WithBabylonVersion("v4.3.0"),
	)
	defer tm.Stop(t)
	tm.CatchUpBTCLightClient(t)

	btcNotifier, err := btcclient.NewNodeBackend(
		btcclient.ToBitcoindConfig(tm.Config.BTC),
		&chaincfg.RegressionNetParams,
		&btcclient.EmptyHintCache{},
	)
	require.NoError(t, err)
	require.NoError(t, btcNotifier.Start())

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
		testutil.MakeTestBackend(t),
	)
	bsTracker.Start()
	defer bsTracker.Stop()

	_, fpSK := tm.CreateFinalityProvider(t)

	// ── Step 1: parent staking ──────────────────────────────────────────────
	parentStakingSlashingInfo, _, delSK := tm.CreateBTCDelegation(t, fpSK)
	parentStakingTxHash := parentStakingSlashingInfo.StakingTx.TxHash()

	var parentDel *btcstakingtypes.BTCDelegationResponse
	require.Eventually(t, func() bool {
		resp, err := tm.BabylonClient.BTCDelegation(parentStakingTxHash.String())
		require.NoError(t, err)
		parentDel = resp.BtcDelegation
		return parentDel.Active
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	// ── Step 2: child stake-expansion (VERIFIED) ────────────────────────────
	childStakingMsgTx, fundingMsgTx, childStakingSlashingInfo, childUnbondingSlashingInfo, _ :=
		tm.CreateBTCStakeExpansion(t, fpSK, parentDel.TotalSat, parentStakingTxHash.String(), parentDel.StakingOutputIdx)
	childStakingTxHash := childStakingMsgTx.TxHash()

	signerAddr := tm.BabylonClient.MustGetAddr()
	childStakingMsgTxHash := childStakingMsgTx.TxHash()
	childSlashingSpendPath, err := childStakingSlashingInfo.StakingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)
	childUnbondingSlashingPathSpendInfo, err := childUnbondingSlashingInfo.UnbondingInfo.SlashingPathSpendInfo()
	require.NoError(t, err)

	tm.addCovenantSigStkExp(
		t,
		signerAddr,
		childStakingMsgTx,
		&childStakingMsgTxHash,
		fpSK,
		childSlashingSpendPath,
		childStakingSlashingInfo,
		childUnbondingSlashingInfo,
		childUnbondingSlashingPathSpendInfo,
		0,
		parentStakingSlashingInfo,
		fundingMsgTx.TxOut[0],
	)

	require.Eventually(t, func() bool {
		resp, err := tm.BabylonClient.BTCDelegation(childStakingTxHash.String())
		require.NoError(t, err)
		return resp.BtcDelegation.StatusDesc == btcstakingtypes.BTCDelegationStatus_VERIFIED.String()
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	// ── Step 3: build expansion staking tx witness and broadcast to BTC ─────
	parentUnbondingPathSpendInfo, err := parentStakingSlashingInfo.StakingInfo.UnbondingPathSpendInfo()
	require.NoError(t, err)

	stakerSig, err := btcstaking.SignTxForFirstScriptSpendWithTwoInputsFromScript(
		childStakingMsgTx,
		parentStakingSlashingInfo.StakingInfo.StakingOutput,
		fundingMsgTx.TxOut[0],
		delSK,
		parentUnbondingPathSpendInfo.GetPkScriptPath(),
	)
	require.NoError(t, err)

	childResp, err := tm.BabylonClient.BTCDelegation(childStakingTxHash.String())
	require.NoError(t, err)
	covenantSigs := childResp.BtcDelegation.StkExp.PreviousStkCovenantSigs
	witness, err := parentUnbondingPathSpendInfo.CreateUnbondingPathWitness(
		[]*schnorr.Signature{covenantSigs[0].Sig.MustToBTCSig()},
		stakerSig,
	)
	require.NoError(t, err)
	childStakingMsgTx.TxIn[0].Witness = witness

	signedExpTx, complete, err := tm.BTCClient.SignRawTransactionWithWallet(childStakingMsgTx)
	require.NoError(t, err)
	require.True(t, complete, "expansion tx signing incomplete")

	_, err = tm.BTCClient.SendRawTransaction(signedExpTx, true)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		txns, err := tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{&childStakingTxHash})
		require.NoError(t, err)
		return len(txns) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	expansionBlock := tm.mineBlock(t)
	require.Equal(t, 2, len(expansionBlock.Transactions),
		"expansion staking tx must be the only non-coinbase tx in this block")
	tm.CatchUpBTCLightClient(t)

	// ── Step 4: poison the child — unbond it via MsgBTCUndelegate at depth=1 ─
	childUnbondingTxSchnorrSig, err := btcstaking.SignTxWithOneScriptSpendInputStrict(
		childUnbondingSlashingInfo.UnbondingTx,
		childStakingSlashingInfo.StakingTx,
		0,
		func() []byte {
			sp, err := childStakingSlashingInfo.StakingInfo.UnbondingPathSpendInfo()
			require.NoError(t, err)
			return sp.GetPkScriptPath()
		}(),
		delSK,
	)
	require.NoError(t, err)

	childUnbondingPathSpendInfo, err := childStakingSlashingInfo.StakingInfo.UnbondingPathSpendInfo()
	require.NoError(t, err)
	childCovenantUnbondingSigs := childResp.BtcDelegation.UndelegationResponse.CovenantUnbondingSigList
	childUnbondingWitness, err := childUnbondingPathSpendInfo.CreateUnbondingPathWitness(
		[]*schnorr.Signature{childCovenantUnbondingSigs[0].Sig.MustToBTCSig()},
		childUnbondingTxSchnorrSig,
	)
	require.NoError(t, err)
	childUnbondingSlashingInfo.UnbondingTx.TxIn[0].Witness = childUnbondingWitness

	childUnbondingTxHash, err := tm.BTCClient.SendRawTransaction(childUnbondingSlashingInfo.UnbondingTx, true)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		txns, err := tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{childUnbondingTxHash})
		require.NoError(t, err)
		return len(txns) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	childUnbondingBlock := tm.mineBlock(t)
	require.Equal(t, 2, len(childUnbondingBlock.Transactions))
	tm.CatchUpBTCLightClient(t)

	serializedChildUnbondingTx, err := bbn.SerializeBTCTx(childUnbondingSlashingInfo.UnbondingTx)
	require.NoError(t, err)

	childUnbondingFundingTxs := tm.getFundingTxs(t, childUnbondingSlashingInfo.UnbondingTx)
	childUnbondingTxInfo := getTxInfo(t, childUnbondingBlock)

	poisonChildMsg := &btcstakingtypes.MsgBTCUndelegate{
		Signer:          signerAddr,
		StakingTxHash:   childStakingTxHash.String(),
		StakeSpendingTx: serializedChildUnbondingTx,
		StakeSpendingTxInclusionProof: &btcstakingtypes.InclusionProof{
			Key:   childUnbondingTxInfo.Key,
			Proof: childUnbondingTxInfo.Proof,
		},
		FundingTransactions: childUnbondingFundingTxs,
	}
	_, err = tm.BabylonClient.ReliablySendMsg(context.Background(), poisonChildMsg, nil, nil)
	require.NoError(t, err, "poisoning the child stake-expansion via MsgBTCUndelegate must succeed on the running babylon")

	require.Eventually(t, func() bool {
		resp, err := tm.BabylonClient.BTCDelegation(childStakingTxHash.String())
		require.NoError(t, err)
		return resp.BtcDelegation.StatusDesc == btcstakingtypes.BTCDelegationStatus_UNBONDED.String() &&
			resp.BtcDelegation.UndelegationResponse != nil &&
			resp.BtcDelegation.UndelegationResponse.DelegatorUnbondingInfoResponse != nil
	}, eventuallyWaitTimeOut, eventuallyPollTime, "child must be UNBONDED with DelegatorUnbondingInfo set after poison")

	// Sanity: parent stays ACTIVE while child is poisoned. This is the exact
	// phantom-voting-power state that the vigilante fix must remediate.
	parentStillActive, err := tm.BabylonClient.BTCDelegation(parentStakingTxHash.String())
	require.NoError(t, err)
	require.Equal(t, btcstakingtypes.BTCDelegationStatus_ACTIVE.String(), parentStillActive.BtcDelegation.StatusDesc,
		"parent must still be ACTIVE — vigilante has not reported its unbond yet")

	// ── Step 5: vigilante drives the sub-k parent unbond ────────────────────
	// We do NOT mine any further BTC blocks. The expansion tx sits at depth=2
	// (expansion block + child-unbonding block), strictly below k=3 (see
	// --btc-confirmation-depth=3 in e2etest/container/container.go). Without
	// the vigilante fix the watcher would loop in waitForRequiredDepth here
	// forever, since nothing else advances the chain. With the fix it
	// recognizes child.IsUnbonded=true, skips the expansion branch, builds a
	// sub-k proof, and submits MsgBTCUndelegate for the parent. Babylon's
	// block production is independent of BTC mining, so the message lands and
	// the parent transitions to UNBONDED while the expansion is still sub-k.
	require.Eventually(t, func() bool {
		resp, err := tm.BabylonClient.BTCDelegation(parentStakingTxHash.String())
		if err != nil {
			t.Logf("BTCDelegation query error: %v", err)
			return false
		}
		return resp.BtcDelegation.StatusDesc == btcstakingtypes.BTCDelegationStatus_UNBONDED.String()
	}, 2*eventuallyWaitTimeOut, eventuallyPollTime,
		"parent must be reported as UNBONDED by vigilante at sub-k depth")

	t.Logf("phantom-pattern remediation succeeded: parent %s reported as UNBONDED at sub-k depth via vigilante; child %s stayed UNBONDED",
		parentStakingTxHash.String(), childStakingTxHash.String())
}

func TestUnbondingWatcherCensorship(t *testing.T) {
	t.Parallel()
	numMatureOutputs := uint32(300)

	tm := StartManager(t, WithNumMatureOutputs(numMatureOutputs), WithEpochInterval(defaultEpochInterval))
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
		testutil.MakeTestBackend(t),
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
