//go:build e2e

package e2etest

import (
	"context"
	"testing"
	"time"

	"github.com/babylonlabs-io/vigilante/btcclient"
	bst "github.com/babylonlabs-io/vigilante/btcstaking-tracker"
	"github.com/babylonlabs-io/vigilante/btcstaking-tracker/indexer"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/monitor"
	"github.com/babylonlabs-io/vigilante/testutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestActivationMonitor(t *testing.T) {
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

	babylonAdapter := monitor.NewBabylonAdaptorClientAdapter(tm.BabylonClient, &tm.Config.Monitor)

	indexerClient := indexer.NewHTTPIndexerClient(
		tm.Config.BTCStakingTracker.IndexerAddr,
		30*time.Second,
		*zap.NewNop(),
	)

	monitorCfg := config.DefaultMonitorConfig()
	monitorCfg.ActivationTimeoutSeconds = 300

	activationMetrics := metrics.NewActivationUnbondingMonitorMetrics()
	activationMonitor := monitor.NewActivationUnbondingMonitor(
		babylonAdapter,
		tm.BTCClient,
		indexerClient,
		&monitorCfg,
		zap.NewNop(),
		activationMetrics,
	)

	err = activationMonitor.Start()
	require.NoError(t, err)
	defer activationMonitor.Stop()

	_, fpSK := tm.CreateFinalityProvider(t)
	stakingMsgTx, _, _, _ := tm.CreateBTCDelegationWithoutIncl(t, fpSK)
	stakingMsgTxHash := stakingMsgTx.TxHash()

	_, err = tm.BTCClient.SendRawTransaction(stakingMsgTx, true)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		txns, err := tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{&stakingMsgTxHash})
		require.NoError(t, err)
		return len(txns) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	mBlock := tm.mineBlock(t)
	require.Equal(t, 2, len(mBlock.Transactions))

	require.Eventually(t, func() bool {
		_, err := tm.BTCClient.GetRawTransaction(&stakingMsgTxHash)
		return err == nil
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	go func() {
		btccParamsResp, err := tm.BabylonClient.BTCCheckpointParams()
		if err != nil {
			return
		}
		for i := 0; i < int(btccParamsResp.Params.BtcConfirmationDepth); i++ {
			select {
			case <-ctx.Done():
				return
			default:
				tm.mineBlock(t)
			}
		}
		tm.CatchUpBTCLightClient(t)
	}()

	require.Eventually(t, func() bool {
		return promtestutil.ToFloat64(activationMetrics.TrackedActivationGauge) > 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	require.Eventually(t, func() bool {
		resp, err := tm.BabylonClient.BTCDelegation(stakingMsgTx.TxHash().String())
		if err != nil {
			return false
		}
		return resp.BtcDelegation.Active
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	require.Eventually(t, func() bool {
		return promtestutil.ToFloat64(activationMetrics.TrackedActivationGauge) == 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	metric := &dto.Metric{}
	err = activationMetrics.ActivationDelayHistogram.Write(metric)
	require.NoError(t, err)
	hist := metric.GetHistogram()
	require.Greater(t, hist.GetSampleCount(), uint64(0), "histogram should record activation")

	timeoutCount := promtestutil.CollectAndCount(activationMetrics.ActivationTimeoutsCounter)
	require.Equal(t, 0, timeoutCount, "no timeouts should occur with long timeout")
}

func TestActivationTimeout(t *testing.T) {
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

	babylonAdapter := monitor.NewBabylonAdaptorClientAdapter(tm.BabylonClient, &tm.Config.Monitor)

	indexerClient := indexer.NewHTTPIndexerClient(
		tm.Config.BTCStakingTracker.IndexerAddr,
		30*time.Second,
		*zap.NewNop(),
	)

	monitorCfg := config.DefaultMonitorConfig()
	monitorCfg.ActivationTimeoutSeconds = 1

	activationMetrics := metrics.NewActivationUnbondingMonitorMetrics()
	activationMonitor := monitor.NewActivationUnbondingMonitor(
		babylonAdapter,
		tm.BTCClient,
		indexerClient,
		&monitorCfg,
		zap.NewNop(),
		activationMetrics,
	)

	err = activationMonitor.Start()
	require.NoError(t, err)
	defer activationMonitor.Stop()

	_, fpSK := tm.CreateFinalityProvider(t)
	stakingMsgTx, _, _, _ := tm.CreateBTCDelegationWithoutIncl(t, fpSK)
	stakingMsgTxHash := stakingMsgTx.TxHash()

	_, err = tm.BTCClient.SendRawTransaction(stakingMsgTx, true)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		txns, err := tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{&stakingMsgTxHash})
		require.NoError(t, err)
		return len(txns) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	mBlock := tm.mineBlock(t)
	require.Equal(t, 2, len(mBlock.Transactions))

	require.Eventually(t, func() bool {
		_, err := tm.BTCClient.GetRawTransaction(&stakingMsgTxHash)
		return err == nil
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	go func() {
		btccParamsResp, err := tm.BabylonClient.BTCCheckpointParams()
		if err != nil {
			return
		}
		for i := 0; i < int(btccParamsResp.Params.BtcConfirmationDepth); i++ {
			select {
			case <-ctx.Done():
				return
			default:
				tm.mineBlock(t)
			}
		}
		tm.CatchUpBTCLightClient(t)
	}()

	require.Eventually(t, func() bool {
		return promtestutil.ToFloat64(activationMetrics.TrackedActivationGauge) > 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	require.Eventually(t, func() bool {
		return promtestutil.CollectAndCount(activationMetrics.ActivationTimeoutsCounter) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime, "alert should have fired once")

	require.Never(t, func() bool {
		return promtestutil.CollectAndCount(activationMetrics.ActivationTimeoutsCounter) > 1
	}, 10*time.Second, eventuallyPollTime, "alert should only fire once due to HasAlerted flag")

	require.Eventually(t, func() bool {
		resp, err := tm.BabylonClient.BTCDelegation(stakingMsgTx.TxHash().String())
		if err != nil {
			return false
		}
		return resp.BtcDelegation.Active
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	require.Eventually(t, func() bool {
		return promtestutil.ToFloat64(activationMetrics.TrackedActivationGauge) == 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	metric := &dto.Metric{}
	err = activationMetrics.ActivationDelayHistogram.Write(metric)
	require.NoError(t, err)
	hist := metric.GetHistogram()
	require.Greater(t, hist.GetSampleCount(), uint64(0), "histogram should record activation")
}

func TestMultipleDelegationsActivation(t *testing.T) {
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

	babylonAdapter := monitor.NewBabylonAdaptorClientAdapter(tm.BabylonClient, &tm.Config.Monitor)

	indexerClient := indexer.NewHTTPIndexerClient(
		tm.Config.BTCStakingTracker.IndexerAddr,
		30*time.Second,
		*zap.NewNop(),
	)

	monitorCfg := config.DefaultMonitorConfig()
	monitorCfg.ActivationTimeoutSeconds = 300

	activationMetrics := metrics.NewActivationUnbondingMonitorMetrics()
	activationMonitor := monitor.NewActivationUnbondingMonitor(
		babylonAdapter,
		tm.BTCClient,
		indexerClient,
		&monitorCfg,
		zap.NewNop(),
		activationMetrics,
	)

	err = activationMonitor.Start()
	require.NoError(t, err)
	defer activationMonitor.Stop()

	_, fpSK := tm.CreateFinalityProvider(t)

	var stakingTxs []*wire.MsgTx
	numDelegations := 3

	for i := 0; i < numDelegations; i++ {
		stakingMsgTx, _, _, _ := tm.CreateBTCDelegationWithoutIncl(t, fpSK)
		stakingMsgTxHash := stakingMsgTx.TxHash()

		_, err := tm.BTCClient.SendRawTransaction(stakingMsgTx, true)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			txns, err := tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{&stakingMsgTxHash})
			require.NoError(t, err)
			return len(txns) == 1
		}, eventuallyWaitTimeOut, eventuallyPollTime)

		mBlock := tm.mineBlock(t)
		require.Equal(t, 2, len(mBlock.Transactions))

		require.Eventually(t, func() bool {
			_, err := tm.BTCClient.GetRawTransaction(&stakingMsgTxHash)
			return err == nil
		}, eventuallyWaitTimeOut, eventuallyPollTime)

		stakingTxs = append(stakingTxs, stakingMsgTx)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	go func() {
		btccParamsResp, err := tm.BabylonClient.BTCCheckpointParams()
		if err != nil {
			return
		}
		for i := 0; i < int(btccParamsResp.Params.BtcConfirmationDepth); i++ {
			select {
			case <-ctx.Done():
				return
			default:
				tm.mineBlock(t)
			}
		}
		tm.CatchUpBTCLightClient(t)
	}()

	require.Eventually(t, func() bool {
		gauge := promtestutil.ToFloat64(activationMetrics.TrackedActivationGauge)
		return gauge == float64(numDelegations)
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	require.Eventually(t, func() bool {
		activeCount := 0
		for _, tx := range stakingTxs {
			resp, err := tm.BabylonClient.BTCDelegation(tx.TxHash().String())
			if err == nil && resp.BtcDelegation.Active {
				activeCount++
			}
		}
		return activeCount == numDelegations
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	require.Eventually(t, func() bool {
		gauge := promtestutil.ToFloat64(activationMetrics.TrackedActivationGauge)
		return gauge == 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	metric := &dto.Metric{}
	err = activationMetrics.ActivationDelayHistogram.Write(metric)
	require.NoError(t, err)
	hist := metric.GetHistogram()
	require.Equal(t, uint64(numDelegations), hist.GetSampleCount(), "histogram should record all 3 activations")

	timeoutCount := promtestutil.CollectAndCount(activationMetrics.ActivationTimeoutsCounter)
	require.Equal(t, 0, timeoutCount, "no timeouts should occur with long timeout")
}
