//go:build e2e
// +build e2e

package e2etest

import (
	"sync"
	"testing"
	"time"

	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/netparams"

	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/reporter"
	"github.com/stretchr/testify/require"
)

var (
	longEventuallyWaitTimeOut = 2 * eventuallyWaitTimeOut
)

func (tm *TestManager) BabylonBTCChainMatchesBtc(t *testing.T) bool {
	tipHeight, err := tm.TestRpcClient.GetBlockCount()
	require.NoError(t, err)
	tipHash, err := tm.TestRpcClient.GetBlockHash(tipHeight)
	require.NoError(t, err)
	bbnBtcLcTip, err := tm.BabylonClient.BTCHeaderChainTip()
	require.NoError(t, err)

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

	for i := 0; i < N; i++ {
		tm.BitcoindHandler.GenerateBlocks(N)
	}
}

func TestReporter_BoostrapUnderFrequentBTCHeaders(t *testing.T) {
	//t.Parallel() // todo(lazar): this test when run in parallel is very flaky, investigate why
	// no need to much mature outputs, we are not going to submit transactions in this test
	numMatureOutputs := uint32(150)

	tm := StartManager(t, numMatureOutputs, defaultEpochInterval)
	defer tm.Stop(t)

	reporterMetrics := metrics.NewReporterMetrics()

	// create the chain notifier
	btcParams, err := netparams.GetBTCParams(tm.Config.BTC.NetParams)
	require.NoError(t, err)
	btcCfg := btcclient.ToBitcoindConfig(tm.Config.BTC)
	btcNotifier, err := btcclient.NewNodeBackend(btcCfg, btcParams, &btcclient.EmptyHintCache{})
	require.NoError(t, err)

	vigilantReporter, err := reporter.New(
		&tm.Config.Reporter,
		logger,
		tm.BTCClient,
		tm.BabylonClient,
		btcNotifier,
		tm.Config.Common.RetrySleepTime,
		tm.Config.Common.MaxRetrySleepTime,
		reporterMetrics,
	)
	require.NoError(t, err)

	// start a routine that mines BTC blocks very fast
	var wg sync.WaitGroup
	stopChan := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(10 * time.Second)
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

	tm := StartManager(t, numMatureOutputs, defaultEpochInterval)
	// this is necessary to receive notifications about new transactions entering mempool
	defer tm.Stop(t)

	reporterMetrics := metrics.NewReporterMetrics()

	btcParams, err := netparams.GetBTCParams(tm.Config.BTC.NetParams)
	require.NoError(t, err)
	btcCfg := btcclient.ToBitcoindConfig(tm.Config.BTC)
	btcNotifier, err := btcclient.NewNodeBackend(btcCfg, btcParams, &btcclient.EmptyHintCache{})
	require.NoError(t, err)

	vigilantReporter, err := reporter.New(
		&tm.Config.Reporter,
		logger,
		tm.BTCClient,
		tm.BabylonClient,
		btcNotifier,
		tm.Config.Common.RetrySleepTime,
		tm.Config.Common.MaxRetrySleepTime,
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
	}, longEventuallyWaitTimeOut, eventuallyPollTime)
}

func TestHandleReorgAfterRestart(t *testing.T) {
	t.Parallel()
	// no need to much mature outputs, we are not going to submit transactions in this test
	numMatureOutputs := uint32(150)

	tm := StartManager(t, numMatureOutputs, defaultEpochInterval)
	// this is necessary to receive notifications about new transactions entering mempool
	defer tm.Stop(t)

	reporterMetrics := metrics.NewReporterMetrics()

	btcParams, err := netparams.GetBTCParams(tm.Config.BTC.NetParams)
	require.NoError(t, err)
	btcCfg := btcclient.ToBitcoindConfig(tm.Config.BTC)
	btcNotifier, err := btcclient.NewNodeBackend(btcCfg, btcParams, &btcclient.EmptyHintCache{})
	require.NoError(t, err)

	vigilantReporter, err := reporter.New(
		&tm.Config.Reporter,
		logger,
		tm.BTCClient,
		tm.BabylonClient,
		btcNotifier,
		tm.Config.Common.RetrySleepTime,
		tm.Config.Common.MaxRetrySleepTime,
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

	// // we will start from block before tip and submit 2 new block this should trigger rollback
	tm.GenerateAndSubmitBlockNBlockStartingFromDepth(t, 2, 1)

	btcClient := initBTCClientWithSubscriber(t, tm.Config) //current tm.btcClient already has an active zmq subscription, would panic
	defer btcClient.Stop()

	// Start new reporter
	vigilantReporterNew, err := reporter.New(
		&tm.Config.Reporter,
		logger,
		btcClient,
		tm.BabylonClient,
		btcNotifier,
		tm.Config.Common.RetrySleepTime,
		tm.Config.Common.MaxRetrySleepTime,
		reporterMetrics,
	)
	require.NoError(t, err)

	vigilantReporterNew.Start()

	// Headers should match even though reorg happened
	require.Eventually(t, func() bool {
		return tm.BabylonBTCChainMatchesBtc(t)
	}, longEventuallyWaitTimeOut, eventuallyPollTime)
}
