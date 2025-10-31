package e2etest

import (
	"context"
	"encoding/hex"
	"sync"
	"testing"
	"time"

	bbnclient "github.com/babylonlabs-io/babylon/v4/client/client"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"go.uber.org/zap/zaptest"

	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/netparams"

	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/reporter"
	"github.com/stretchr/testify/require"
)

func (tm *TestManager) BabylonBTCChainMatchesBtc(t *testing.T) bool {
	tipHeight, err := tm.TestRpcClient.GetBlockCount()
	require.NoError(t, err)
	tipHash, err := tm.TestRpcClient.GetBlockHash(tipHeight)
	require.NoError(t, err)
	bbnBtcLcTip, err := tm.BabylonClient.BTCHeaderChainTip()
	require.NoError(t, err)

	t.Logf("bbn tip height: %d, bbn tip hash: %s, btc tip height: %d, btc tip hash: %s",
		bbnBtcLcTip.Header.Height, bbnBtcLcTip.Header.HashHex, tipHeight, tipHash.String())

	return uint32(tipHeight) == bbnBtcLcTip.Header.Height && tipHash.String() == bbnBtcLcTip.Header.HashHex
}

func (tm *TestManager) GenerateAndSubmitBlockNBlockStartingFromDepth(t *testing.T, N int, depth uint32) {
	if depth == 0 {
		// depth 0 means we are starting from tip
		tm.BitcoindHandler.GenerateBlocks(N)
		return
	}

	height, err := tm.TestRpcClient.GetBlockCount()
	require.NoError(t, err)

	startingBlockHeight := height - int64(depth)

	blockHash, err := tm.TestRpcClient.GetBlockHash(startingBlockHeight)
	require.NoError(t, err)

	// invalidate blocks from this height
	tm.BitcoindHandler.InvalidateBlock(blockHash.String())

	// Generate N new blocks on the alternative chain
	tm.BitcoindHandler.GenerateBlocks(N)
}

func TestReporter_BoostrapUnderFrequentBTCHeaders(t *testing.T) {
	t.Parallel()
	// no need to much mature outputs, we are not going to submit transactions in this test
	numMatureOutputs := uint32(150)

	tm := StartManager(t, WithNumMatureOutputs(numMatureOutputs), WithEpochInterval(defaultEpochInterval))
	defer tm.Stop(t)

	reporterMetrics := metrics.NewReporterMetrics()

	// create the chain notifier
	btcParams, err := netparams.GetBTCParams(tm.Config.BTC.NetParams)
	require.NoError(t, err)
	btcCfg := btcclient.ToBitcoindConfig(tm.Config.BTC)
	btcNotifier, err := btcclient.NewNodeBackend(btcCfg, btcParams, &btcclient.EmptyHintCache{})
	require.NoError(t, err)

	// create reporter backend
	reporterBackend := reporter.NewBabylonBackend(tm.BabylonClient)

	vigilantReporter, err := reporter.New(
		&tm.Config.Reporter,
		logger,
		tm.BTCClient,
		reporterBackend,
		tm.BabylonClient,
		btcNotifier,
		tm.Config.Common.RetrySleepTime,
		tm.Config.Common.MaxRetrySleepTime,
		tm.Config.Common.MaxRetryTimes,
		reporterMetrics,
	)
	require.NoError(t, err)

	// start a routine that mines BTC blocks very fast
	var wg sync.WaitGroup
	stopChan := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				tm.BitcoindHandler.GenerateBlocks(1)
			case <-stopChan:
				return
			}
		}
	}()

	// mine some BTC headers
	tm.BitcoindHandler.GenerateBlocks(1)

	// start reporter
	vigilantReporter.Start()
	defer vigilantReporter.Stop()

	// tips should eventually match
	require.Eventually(t, func() bool {
		return tm.BabylonBTCChainMatchesBtc(t)
	}, longEventuallyWaitTimeOut, eventuallyPollTime)

	close(stopChan)
	wg.Wait()
}

func TestRelayHeadersAndHandleRollbacks(t *testing.T) {
	t.Parallel()
	// no need to much mature outputs, we are not going to submit transactions in this test
	numMatureOutputs := uint32(150)

	tm := StartManager(t, WithNumMatureOutputs(numMatureOutputs), WithEpochInterval(defaultEpochInterval))
	// this is necessary to receive notifications about new transactions entering mempool
	defer tm.Stop(t)

	reporterMetrics := metrics.NewReporterMetrics()

	btcParams, err := netparams.GetBTCParams(tm.Config.BTC.NetParams)
	require.NoError(t, err)
	btcCfg := btcclient.ToBitcoindConfig(tm.Config.BTC)
	btcNotifier, err := btcclient.NewNodeBackend(btcCfg, btcParams, &btcclient.EmptyHintCache{})
	require.NoError(t, err)

	// create reporter backend
	reporterBackend := reporter.NewBabylonBackend(tm.BabylonClient)

	vigilantReporter, err := reporter.New(
		&tm.Config.Reporter,
		logger,
		tm.BTCClient,
		reporterBackend,
		tm.BabylonClient,
		btcNotifier,
		tm.Config.Common.RetrySleepTime,
		tm.Config.Common.MaxRetrySleepTime,
		tm.Config.Common.MaxRetryTimes,
		reporterMetrics,
	)
	require.NoError(t, err)

	vigilantReporter.Start()
	defer vigilantReporter.Stop()

	require.Eventually(t, func() bool {
		return tm.BabylonBTCChainMatchesBtc(t)
	}, longEventuallyWaitTimeOut, eventuallyPollTime)

	// generate 3, we are submitting headers 1 by 1 so we use small amount as this is slow process
	tm.BitcoindHandler.GenerateBlocks(3)

	require.Eventually(t, func() bool {
		return tm.BabylonBTCChainMatchesBtc(t)
	}, longEventuallyWaitTimeOut, eventuallyPollTime)

	// we will start from block before tip and submit 2 new block this should trigger rollback
	tm.GenerateAndSubmitBlockNBlockStartingFromDepth(t, 2, 1)

	// tips should eventually match
	require.Eventually(t, func() bool {
		return tm.BabylonBTCChainMatchesBtc(t)
	}, 2*longEventuallyWaitTimeOut, eventuallyPollTime)
}

func TestHandleReorgAfterRestart(t *testing.T) {
	t.Parallel()
	// no need to much mature outputs, we are not going to submit transactions in this test
	numMatureOutputs := uint32(150)

	tm := StartManager(t, WithNumMatureOutputs(numMatureOutputs), WithEpochInterval(defaultEpochInterval))
	// this is necessary to receive notifications about new transactions entering mempool
	defer tm.Stop(t)

	reporterMetrics := metrics.NewReporterMetrics()

	btcParams, err := netparams.GetBTCParams(tm.Config.BTC.NetParams)
	require.NoError(t, err)
	btcCfg := btcclient.ToBitcoindConfig(tm.Config.BTC)
	btcNotifier, err := btcclient.NewNodeBackend(btcCfg, btcParams, &btcclient.EmptyHintCache{})
	require.NoError(t, err)

	// create reporter backend
	reporterBackend := reporter.NewBabylonBackend(tm.BabylonClient)

	vigilantReporter, err := reporter.New(
		&tm.Config.Reporter,
		logger,
		tm.BTCClient,
		reporterBackend,
		tm.BabylonClient,
		btcNotifier,
		tm.Config.Common.RetrySleepTime,
		tm.Config.Common.MaxRetrySleepTime,
		tm.Config.Common.MaxRetryTimes,
		reporterMetrics,
	)
	require.NoError(t, err)

	vigilantReporter.Start()

	require.Eventually(t, func() bool {
		return tm.BabylonBTCChainMatchesBtc(t)
	}, longEventuallyWaitTimeOut, eventuallyPollTime)

	// At this point babylon is inline with btc. Now:
	// Kill reporter
	// and generate reorg on btc
	// start reporter again
	// Even though reorg happened, reporter should be able to provide better chain
	// in bootstrap phase

	vigilantReporter.Stop()
	vigilantReporter.WaitForShutdown()

	// Trigger reorg: invalidate 1 block and mine 3 new ones
	// Mining 3 instead of 2 ensures the new chain has strictly better work than the old chain
	// This is required for Babylon's fork choice rule which rejects chains with equal work
	tm.GenerateAndSubmitBlockNBlockStartingFromDepth(t, 3, 1)

	btcClient := initBTCClientWithSubscriber(t, tm.Config) //current tm.btcClient already has an active zmq subscription, would panic
	defer btcClient.Stop()

	// Start new reporter
	reporterBackendNew := reporter.NewBabylonBackend(tm.BabylonClient)
	vigilantReporterNew, err := reporter.New(
		&tm.Config.Reporter,
		logger,
		btcClient,
		reporterBackendNew,
		tm.BabylonClient,
		btcNotifier,
		tm.Config.Common.RetrySleepTime,
		tm.Config.Common.MaxRetrySleepTime,
		tm.Config.Common.MaxRetryTimes,
		reporterMetrics,
	)
	require.NoError(t, err)

	vigilantReporterNew.Start()

	// Headers should match even though reorg happened
	require.Eventually(t, func() bool {
		return tm.BabylonBTCChainMatchesBtc(t)
	}, longEventuallyWaitTimeOut, eventuallyPollTime)
}

func TestReporter_Censorship(t *testing.T) {
	t.Parallel()
	numMatureOutputs := uint32(150)

	tm := StartManager(t, WithNumMatureOutputs(numMatureOutputs), WithEpochInterval(defaultEpochInterval))
	defer tm.Stop(t)

	reporterMetrics := metrics.NewReporterMetrics()

	// create the chain notifier
	btcParams, err := netparams.GetBTCParams(tm.Config.BTC.NetParams)
	require.NoError(t, err)
	btcCfg := btcclient.ToBitcoindConfig(tm.Config.BTC)
	btcNotifier, err := btcclient.NewNodeBackend(btcCfg, btcParams, &btcclient.EmptyHintCache{})
	require.NoError(t, err)

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

	tm.Config.Common.MaxRetryTimes = 1

	reporterBackendCensorship := reporter.NewBabylonBackend(babylonClient)
	vigilantReporter, err := reporter.New(
		&tm.Config.Reporter,
		logger,
		tm.BTCClient,
		reporterBackendCensorship,
		babylonClient,
		btcNotifier,
		tm.Config.Common.RetrySleepTime,
		tm.Config.Common.MaxRetrySleepTime,
		tm.Config.Common.MaxRetryTimes,
		reporterMetrics,
	)
	require.NoError(t, err)

	// mine some BTC headers
	tm.BitcoindHandler.GenerateBlocks(1)

	// start reporter
	go vigilantReporter.Start()
	defer vigilantReporter.Stop()

	// tips should eventually match
	require.Eventually(t, func() bool {
		return promtestutil.ToFloat64(reporterMetrics.HeadersCensorshipGauge) == 1
	}, longEventuallyWaitTimeOut, eventuallyPollTime)
}

func TestReporter_DuplicateSubmissions(t *testing.T) {
	t.Parallel()
	// no need to much mature outputs, we are not going to submit transactions in this test
	numMatureOutputs := uint32(150)

	tm := StartManager(t, WithNumMatureOutputs(numMatureOutputs), WithEpochInterval(defaultEpochInterval))
	defer tm.Stop(t)

	reporterMetrics := metrics.NewReporterMetrics()

	// create the chain notifier
	btcParams, err := netparams.GetBTCParams(tm.Config.BTC.NetParams)
	require.NoError(t, err)
	btcCfg := btcclient.ToBitcoindConfig(tm.Config.BTC)
	btcNotifier, err := btcclient.NewNodeBackend(btcCfg, btcParams, &btcclient.EmptyHintCache{})
	require.NoError(t, err)

	tm.Config.Common.MaxRetryTimes = 1
	tm.Config.Reporter.BTCCacheSize = 1000

	reporterBackend1 := reporter.NewBabylonBackend(tm.BabylonClient)
	reporter1, err := reporter.New(
		&tm.Config.Reporter,
		zaptest.NewLogger(t).Named("reporter1"),
		tm.BTCClient,
		reporterBackend1,
		tm.BabylonClient,
		btcNotifier,
		tm.Config.Common.RetrySleepTime,
		tm.Config.Common.MaxRetrySleepTime,
		tm.Config.Common.MaxRetryTimes,
		reporterMetrics,
	)
	require.NoError(t, err)

	bstCfg2 := config.DefaultBTCStakingTrackerConfig()
	bstCfg2.CheckDelegationsInterval = 1 * time.Second
	bstCfg2.IndexerAddr = tm.Config.BTCStakingTracker.IndexerAddr

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
	cfg.Reporter.BTCCacheSize = 1000

	babylonClient, err := bbnclient.New(&cfg.Babylon, nil)
	require.NoError(t, err)

	msg := banktypes.NewMsgSend(
		sdk.MustAccAddressFromBech32(tm.BabylonClient.MustGetAddr()),
		address,
		sdk.NewCoins(sdk.NewInt64Coin("ubbn", 100_000_000)),
	)

	_, err = tm.BabylonClient.ReliablySendMsg(context.Background(), msg, nil, nil)
	require.NoError(t, err)

	cfg.Common.MaxRetryTimes = 1
	reporter2Metrics := metrics.NewReporterMetrics()
	reporterBackend2 := reporter.NewBabylonBackend(babylonClient)
	reporter2, err := reporter.New(
		&cfg.Reporter,
		zaptest.NewLogger(t).Named("reporter2"),
		tm.BTCClient,
		reporterBackend2,
		babylonClient,
		btcNotifier,
		tm.Config.Common.RetrySleepTime,
		tm.Config.Common.MaxRetrySleepTime,
		tm.Config.Common.MaxRetryTimes,
		reporter2Metrics,
	)
	require.NoError(t, err)

	tm.BitcoindHandler.GenerateBlocks(500)

	// start reporters
	go reporter1.Start()
	go reporter2.Start()

	defer reporter1.Stop()
	defer reporter2.Stop()

	// tips should eventually match
	require.Eventually(t, func() bool {
		return tm.BabylonBTCChainMatchesBtc(t)
	}, longEventuallyWaitTimeOut, eventuallyPollTime)

	// we expect that we have failed headers
	require.Eventually(t, func() bool {
		return promtestutil.ToFloat64(reporterMetrics.FailedHeadersCounter) == 0 || promtestutil.ToFloat64(reporter2Metrics.FailedHeadersCounter) == 0
	}, longEventuallyWaitTimeOut, eventuallyPollTime)
}
