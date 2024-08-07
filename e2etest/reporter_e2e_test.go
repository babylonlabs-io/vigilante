package e2etest

import (
	"math/rand"
	"testing"
	"time"

	"github.com/babylonlabs-io/babylon/testutil/datagen"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/reporter"
	"github.com/babylonlabs-io/vigilante/types"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/integration/rpctest"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
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
	//tipHash, tipHeight, err := tm.BTCClient.GetBestBlock()
	bbnBtcLcTip, err := tm.BabylonClient.BTCHeaderChainTip()
	require.NoError(t, err)

	return uint64(tipHeight) == bbnBtcLcTip.Header.Height && tipHash.String() == bbnBtcLcTip.Header.HashHex
}

func (tm *TestManager) GenerateAndSubmitsNBlocksFromTip(t *testing.T, N int) {
	var ut time.Time

	for i := 0; i < N; i++ {
		//tm.MinerNode.GenerateAndSubmitBlock(nil, -1, ut)
		tm.generateAndSubmitBlock(t, ut)
	}
}

func (tm *TestManager) generateAndSubmitBlock(t *testing.T, bt time.Time) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	height, err := tm.TestRpcClient.GetBlockCount()
	hash, err := tm.TestRpcClient.GetBlockHash(height)
	require.NoError(t, err)
	block, err := tm.TestRpcClient.GetBlock(hash)
	require.NoError(t, err)
	require.NoError(t, err)
	mBlock, err := tm.TestRpcClient.GetBlock(&block.Header.PrevBlock)
	require.NoError(t, err)
	prevBlock := btcutil.NewBlock(mBlock)
	prevBlock.SetHeight(int32(height))

	arr := datagen.GenRandomByteArray(r, 20)
	add, err := btcutil.NewAddressScriptHashFromHash(arr, regtestParams)
	require.NoError(t, err)
	// Create a new block including the specified transactions
	newBlock, err := rpctest.CreateBlock(prevBlock, nil, rpctest.BlockVersion,
		bt, add, nil, regtestParams)
	require.NoError(t, err)

	err = tm.TestRpcClient.SubmitBlock(newBlock, nil)

	require.NoError(t, err)
}

func (tm *TestManager) GenerateAndSubmitBlockNBlockStartingFromDepth(t *testing.T, N int, depth uint32) {
	r := rand.New(rand.NewSource(time.Now().Unix()))

	if depth == 0 {
		// depth 0 means we are starting from tip
		tm.GenerateAndSubmitsNBlocksFromTip(t, N)
		return
	}

	height, err := tm.TestRpcClient.GetBlockCount()
	require.NoError(t, err)
	//_, bestHeight, err := tm.TestRpcClient.GetBestBlock()
	//require.NoError(t, err)

	startingBlockHeight := height - int64(depth)

	blockHash, err := tm.TestRpcClient.GetBlockHash(int64(startingBlockHeight))
	require.NoError(t, err)

	startingBlockMsg, err := tm.TestRpcClient.GetBlock(blockHash)
	require.NoError(t, err)

	startingBlock := btcutil.NewBlock(startingBlockMsg)
	startingBlock.SetHeight(int32(startingBlockHeight))

	arr := datagen.GenRandomByteArray(r, 20)
	add, err := btcutil.NewAddressScriptHashFromHash(arr, regtestParams)
	require.NoError(t, err)

	var lastSubmittedBlock *btcutil.Block
	var ut time.Time

	for i := 0; i < N; i++ {
		var blockToSubmit *btcutil.Block

		if lastSubmittedBlock == nil {
			// first block to submit start from starting block
			newBlock, err := rpctest.CreateBlock(startingBlock, nil, rpctest.BlockVersion,
				ut, add, nil, regtestParams)
			require.NoError(t, err)
			blockToSubmit = newBlock
		} else {
			newBlock, err := rpctest.CreateBlock(lastSubmittedBlock, nil, rpctest.BlockVersion,
				ut, add, nil, regtestParams)
			require.NoError(t, err)
			blockToSubmit = newBlock
		}
		err = tm.TestRpcClient.SubmitBlock(blockToSubmit, nil)
		require.NoError(t, err)
		lastSubmittedBlock = blockToSubmit
	}
}

func TestReporter_BoostrapUnderFrequentBTCHeaders(t *testing.T) {
	// no need to much mature outputs, we are not going to submit transactions in this test
	numMatureOutputs := uint32(300)

	tm := StartManager(t, numMatureOutputs, 1, nil, nil)
	defer tm.Stop(t)

	reporterMetrics := metrics.NewReporterMetrics()
	vigilantReporter, err := reporter.New(
		&tm.Config.Reporter,
		logger,
		tm.BTCClient,
		tm.BabylonClient,
		tm.Config.Common.RetrySleepTime,
		tm.Config.Common.MaxRetrySleepTime,
		reporterMetrics,
	)
	require.NoError(t, err)

	// start a routine that mines BTC blocks very fast
	//go func() {
	//	ticker := time.NewTicker(10 * time.Second)
	//	for range ticker.C {
	//		tm.GenerateAndSubmitsNBlocksFromTip(t, 1)
	//	}
	//}()

	// mine some BTC headers
	tm.GenerateAndSubmitsNBlocksFromTip(t, 1)

	// start reporter
	vigilantReporter.Start()
	defer vigilantReporter.Stop()

	// tips should eventually match
	require.Eventually(t, func() bool {
		return tm.BabylonBTCChainMatchesBtc(t)
	}, longEventuallyWaitTimeOut, eventuallyPollTime)
}

func TestRelayHeadersAndHandleRollbacks(t *testing.T) {
	// no need to much mature outputs, we are not going to submit transactions in this test
	numMatureOutputs := uint32(2)

	blockEventChan := make(chan *types.BlockEvent, 1000)
	handlers := &rpcclient.NotificationHandlers{
		OnFilteredBlockConnected: func(height int32, header *wire.BlockHeader, txs []*btcutil.Tx) {
			blockEventChan <- types.NewBlockEvent(types.BlockConnected, height, header)
		},
		OnFilteredBlockDisconnected: func(height int32, header *wire.BlockHeader) {
			blockEventChan <- types.NewBlockEvent(types.BlockDisconnected, height, header)
		},
	}

	tm := StartManager(t, numMatureOutputs, 2, handlers, blockEventChan)
	// this is necessary to receive notifications about new transactions entering mempool
	defer tm.Stop(t)

	reporterMetrics := metrics.NewReporterMetrics()

	vigilantReporter, err := reporter.New(
		&tm.Config.Reporter,
		logger,
		tm.BTCClient,
		tm.BabylonClient,
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
	tm.GenerateAndSubmitsNBlocksFromTip(t, 3)

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
	// no need to much mature outputs, we are not going to submit transactions in this test
	numMatureOutputs := uint32(2)

	blockEventChan := make(chan *types.BlockEvent, 1000)
	handlers := &rpcclient.NotificationHandlers{
		OnFilteredBlockConnected: func(height int32, header *wire.BlockHeader, txs []*btcutil.Tx) {
			blockEventChan <- types.NewBlockEvent(types.BlockConnected, height, header)
		},
		OnFilteredBlockDisconnected: func(height int32, header *wire.BlockHeader) {
			blockEventChan <- types.NewBlockEvent(types.BlockDisconnected, height, header)
		},
	}

	tm := StartManager(t, numMatureOutputs, 2, handlers, blockEventChan)
	// this is necessary to receive notifications about new transactions entering mempool
	defer tm.Stop(t)

	reporterMetrics := metrics.NewReporterMetrics()

	vigilantReporter, err := reporter.New(
		&tm.Config.Reporter,
		logger,
		tm.BTCClient,
		tm.BabylonClient,
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

	// Start new reporter
	vigilantReporterNew, err := reporter.New(
		&tm.Config.Reporter,
		logger,
		tm.BTCClient,
		tm.BabylonClient,
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
