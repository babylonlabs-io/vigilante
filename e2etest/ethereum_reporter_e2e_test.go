//go:build e2e

package e2etest

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/contracts/btcprism"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/netparams"
	"github.com/babylonlabs-io/vigilante/reporter"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/stretchr/testify/require"
)

// TestEthereumReporter_SubmitHeaders tests that the Ethereum reporter can submit BTC headers
func TestEthereumReporter_SubmitHeaders(t *testing.T) {
	t.Parallel()

	// Start Bitcoin and Babylon with default genesis (height 0)
	numMatureOutputs := uint32(150)
	tm := StartManager(t, WithNumMatureOutputs(numMatureOutputs), WithEpochInterval(defaultEpochInterval))
	defer tm.Stop(t)

	// Generate blocks to reach minimum height required by contract (>2016)
	// The contract requires _blockHeight > 2016 for difficulty adjustment math
	tm.BitcoindHandler.GenerateBlocks(2020)

	// Start Anvil
	anvilHandler := NewAnvilHandler(t, tm.manger)
	_, err := anvilHandler.Start(t)
	require.NoError(t, err)

	// Get BTC tip height for contract deployment
	btcTipHeight, err := tm.TestRpcClient.GetBlockCount()
	require.NoError(t, err)

	// Deploy contract near the tip to avoid exceeding reporter's cache limit during bootstrap
	// Reporter cache is limited to k+w (default 10+100=110 blocks)
	genesisHeight := uint64(btcTipHeight - 50) // Start 50 blocks before tip
	genesisHash, err := tm.TestRpcClient.GetBlockHash(int64(genesisHeight))
	require.NoError(t, err)
	genesisBlock, err := tm.TestRpcClient.GetBlock(genesisHash)
	require.NoError(t, err)

	// Convert block hash to [32]byte (little-endian -> big-endian for contract)
	var blockHash [32]byte
	hashBytes := genesisHash.CloneBytes()
	reverseBytes(hashBytes)
	copy(blockHash[:], hashBytes)

	// Get expected target from block's difficulty bits
	expectedTarget := big.NewInt(1)
	expectedTarget.Lsh(expectedTarget, 256)
	expectedTarget.Div(expectedTarget, big.NewInt(int64(genesisBlock.Header.Bits)))

	// Deploy BtcPrism contract
	_ = anvilHandler.DeployBtcPrismContract(
		t,
		genesisHeight,
		blockHash,
		uint32(genesisBlock.Header.Timestamp.Unix()),
		expectedTarget,
		true, // isTestnet = true for regtest
	)

	// Create Ethereum backend
	ethCfg := anvilHandler.GetEthereumConfig()
	ethBackend, err := reporter.NewEthereumBackend(ethCfg, logger)
	require.NoError(t, err)
	defer ethBackend.Stop()

	// Create reporter with Ethereum backend
	// Increase cache size to handle all blocks during bootstrap
	tm.Config.Reporter.BTCCacheSize = 3000

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
		ethBackend,
		tm.BabylonClient,
		btcNotifier,
		tm.Config.Common.RetrySleepTime,
		tm.Config.Common.MaxRetrySleepTime,
		tm.Config.Common.MaxRetryTimes,
		reporterMetrics,
	)
	require.NoError(t, err)

	// Generate some BTC blocks
	tm.BitcoindHandler.GenerateBlocks(5)

	// Start reporter
	vigilantReporter.Start()
	defer vigilantReporter.Stop()

	// Wait for headers to be submitted to Ethereum
	require.Eventually(t, func() bool {
		return ethereumChainMatchesBtc(t, tm, anvilHandler)
	}, longEventuallyWaitTimeOut, eventuallyPollTime)

	t.Logf("Ethereum contract successfully received BTC headers up to height 5")
}

// TestEthereumReporter_HandleReorg tests that the reporter handles Bitcoin reorgs correctly
func TestEthereumReporter_HandleReorg(t *testing.T) {
	t.Parallel()

	numMatureOutputs := uint32(150)
	tm := StartManager(t, WithNumMatureOutputs(numMatureOutputs), WithEpochInterval(defaultEpochInterval))
	defer tm.Stop(t)

	// Generate blocks to reach minimum height required by contract (>2016)
	tm.BitcoindHandler.GenerateBlocks(2020)

	// Start Anvil
	anvilHandler := NewAnvilHandler(t, tm.manger)
	_, err := anvilHandler.Start(t)
	require.NoError(t, err)

	// Get BTC tip height for contract deployment
	btcTipHeight, err := tm.TestRpcClient.GetBlockCount()
	require.NoError(t, err)

	// Deploy contract near the tip to avoid exceeding cache limit during bootstrap
	genesisHeight := uint64(btcTipHeight - 50)
	genesisHash, err := tm.TestRpcClient.GetBlockHash(int64(genesisHeight))
	require.NoError(t, err)
	genesisBlock, err := tm.TestRpcClient.GetBlock(genesisHash)
	require.NoError(t, err)

	var blockHash [32]byte
	hashBytes := genesisHash.CloneBytes()
	reverseBytes(hashBytes)
	copy(blockHash[:], hashBytes)

	expectedTarget := big.NewInt(1)
	expectedTarget.Lsh(expectedTarget, 256)
	expectedTarget.Div(expectedTarget, big.NewInt(int64(genesisBlock.Header.Bits)))

	_ = anvilHandler.DeployBtcPrismContract(
		t,
		genesisHeight,
		blockHash,
		uint32(genesisBlock.Header.Timestamp.Unix()),
		expectedTarget,
		true,
	)

	// Create Ethereum backend
	ethCfg := anvilHandler.GetEthereumConfig()
	ethBackend, err := reporter.NewEthereumBackend(ethCfg, logger)
	require.NoError(t, err)
	defer ethBackend.Stop()

	// Create reporter
	// Increase cache size to handle all blocks during bootstrap
	tm.Config.Reporter.BTCCacheSize = 3000

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
		ethBackend,
		tm.BabylonClient,
		btcNotifier,
		tm.Config.Common.RetrySleepTime,
		tm.Config.Common.MaxRetrySleepTime,
		tm.Config.Common.MaxRetryTimes,
		reporterMetrics,
	)
	require.NoError(t, err)

	// Start reporter
	vigilantReporter.Start()
	defer vigilantReporter.Stop()

	// Generate initial blocks
	tm.BitcoindHandler.GenerateBlocks(3)

	// Wait for sync
	require.Eventually(t, func() bool {
		return ethereumChainMatchesBtc(t, tm, anvilHandler)
	}, longEventuallyWaitTimeOut, eventuallyPollTime)

	// Trigger a reorg: invalidate last 2 blocks and mine 4 new ones
	// We mine 4 instead of 3 so the new chain has a higher tip than the old chain
	// This ensures the btcNotifier sends block notifications for the new blocks
	tm.GenerateAndSubmitBlockNBlockStartingFromDepth(t, 4, 2)

	// Wait for reporter to handle reorg and sync new chain
	require.Eventually(t, func() bool {
		return ethereumChainMatchesBtc(t, tm, anvilHandler)
	}, longEventuallyWaitTimeOut, eventuallyPollTime)

	t.Logf("Ethereum reporter successfully handled BTC reorg")
}

// TestEthereumReporter_ContinuousBlocks tests continuous block submission
func TestEthereumReporter_ContinuousBlocks(t *testing.T) {
	t.Parallel()

	numMatureOutputs := uint32(150)
	tm := StartManager(t, WithNumMatureOutputs(numMatureOutputs), WithEpochInterval(defaultEpochInterval))
	defer tm.Stop(t)

	// Generate blocks to reach minimum height required by contract (>2016)
	tm.BitcoindHandler.GenerateBlocks(2020)

	// Start Anvil
	anvilHandler := NewAnvilHandler(t, tm.manger)
	_, err := anvilHandler.Start(t)
	require.NoError(t, err)

	// Get BTC tip height for contract deployment
	btcTipHeight, err := tm.TestRpcClient.GetBlockCount()
	require.NoError(t, err)

	// Deploy contract near the tip to avoid exceeding cache limit during bootstrap
	genesisHeight := uint64(btcTipHeight - 50)
	genesisHash, err := tm.TestRpcClient.GetBlockHash(int64(genesisHeight))
	require.NoError(t, err)
	genesisBlock, err := tm.TestRpcClient.GetBlock(genesisHash)
	require.NoError(t, err)

	var blockHash [32]byte
	hashBytes := genesisHash.CloneBytes()
	reverseBytes(hashBytes)
	copy(blockHash[:], hashBytes)

	expectedTarget := big.NewInt(1)
	expectedTarget.Lsh(expectedTarget, 256)
	expectedTarget.Div(expectedTarget, big.NewInt(int64(genesisBlock.Header.Bits)))

	anvilHandler.DeployBtcPrismContract(
		t,
		genesisHeight,
		blockHash,
		uint32(genesisBlock.Header.Timestamp.Unix()),
		expectedTarget,
		true,
	)

	// Create Ethereum backend
	ethCfg := anvilHandler.GetEthereumConfig()
	ethBackend, err := reporter.NewEthereumBackend(ethCfg, logger)
	require.NoError(t, err)
	defer ethBackend.Stop()

	// Create reporter
	// Increase cache size to handle all blocks during bootstrap
	tm.Config.Reporter.BTCCacheSize = 3000

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
		ethBackend,
		tm.BabylonClient,
		btcNotifier,
		tm.Config.Common.RetrySleepTime,
		tm.Config.Common.MaxRetrySleepTime,
		tm.Config.Common.MaxRetryTimes,
		reporterMetrics,
	)
	require.NoError(t, err)

	// Start reporter
	vigilantReporter.Start()
	defer vigilantReporter.Stop()

	// Generate blocks in batches to test continuous submission
	for i := 0; i < 3; i++ {
		tm.BitcoindHandler.GenerateBlocks(5)

		// Wait for sync after each batch
		require.Eventually(t, func() bool {
			return ethereumChainMatchesBtc(t, tm, anvilHandler)
		}, longEventuallyWaitTimeOut, eventuallyPollTime)

		t.Logf("Batch %d: Successfully synced %d blocks", i+1, (i+1)*5)
	}

	t.Logf("Successfully submitted 15 blocks in 3 batches")
}

// ethereumChainMatchesBtc checks if the Ethereum contract's BTC chain tip matches the actual BTC tip
func ethereumChainMatchesBtc(t *testing.T, tm *TestManager, anvilHandler *AnvilTestHandler) bool {
	// Get BTC tip
	btcTipHeight, err := tm.TestRpcClient.GetBlockCount()
	require.NoError(t, err)
	btcTipHash, err := tm.TestRpcClient.GetBlockHash(btcTipHeight)
	require.NoError(t, err)

	// Get Ethereum contract tip
	contract, err := btcprism.NewBtcprism(anvilHandler.GetContractAddress(), anvilHandler.GetClient())
	require.NoError(t, err)

	ethTipHeight, err := contract.GetLatestBlockHeight(&bind.CallOpts{Context: context.Background()})
	if err != nil {
		t.Logf("failed to get contract tip height: %v", err)
		return false
	}

	// Check if heights match
	if ethTipHeight.Uint64() != uint64(btcTipHeight) {
		t.Logf("height mismatch: eth=%d, btc=%d", ethTipHeight.Uint64(), btcTipHeight)
		return false
	}

	// Check if hashes match
	ethTipHash, err := contract.GetBlockHash(&bind.CallOpts{Context: context.Background()}, ethTipHeight)
	if err != nil {
		t.Logf("failed to get contract tip hash: %v", err)
		return false
	}

	// BTC hashes are little-endian, contract stores big-endian
	btcHashBytes := btcTipHash.CloneBytes()
	reverseBytes(btcHashBytes)

	ethTipHashHex := fmt.Sprintf("0x%x", ethTipHash[:])
	btcHashHex := fmt.Sprintf("0x%x", btcHashBytes)

	if ethTipHashHex != btcHashHex {
		t.Logf("hash mismatch: eth=%s, btc(reversed)=%s, btc(original)=%s", ethTipHashHex, btcHashHex, btcTipHash.String())

		return false
	}

	t.Logf("✓ Ethereum contract in sync: height=%d, hash=%s", ethTipHeight.Uint64(), btcTipHash.String())

	return true
}

// reverseBytes reverses a byte slice in place
func reverseBytes(b []byte) {
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
}
