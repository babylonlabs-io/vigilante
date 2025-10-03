package e2etest

import (
	bbnclient "github.com/babylonlabs-io/babylon/v3/client/client"
	"github.com/babylonlabs-io/vigilante/btcclient"
	bst "github.com/babylonlabs-io/vigilante/btcstaking-tracker"
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
	"sync"
	"testing"
	"time"
)

type activationTestSetup struct {
	tm                *TestManager
	activationMonitor *monitor.ActivationUnbondingMonitor
	metrics           *metrics.ActivationUnbondingMonitorMetrics
	done              chan struct{}
	btcNode           *btcclient.NodeBackend
	bsTracker         *bst.BTCStakingTracker
}

func setupActivationTest(t *testing.T, timeoutSeconds int64) *activationTestSetup {
	numMatureOutputs := uint32(300)

	tm := StartManager(t, WithNumMatureOutputs(numMatureOutputs), WithEpochInterval(defaultEpochInterval))
	tm.CatchUpBTCLightClient(t)

	emptyHintCache := btcclient.EmptyHintCache{}

	bbnClient, err := bbnclient.New(&tm.Config.Babylon, nil)
	require.NoError(t, err)

	btcNode, err := btcclient.NewNodeBackend(
		btcclient.ToBitcoindConfig(tm.Config.BTC),
		&chaincfg.RegressionNetParams,
		&emptyHintCache,
	)
	require.NoError(t, err)
	err = btcNode.Start()
	require.NoError(t, err)

	babyAdapter := monitor.NewBabylonAdaptorClientAdapter(bbnClient, &tm.Config.Monitor)

	monitorCfg := config.DefaultMonitorConfig()
	monitorCfg.ActivationTimeoutSeconds = timeoutSeconds
	monitorCfg.TimingCheckIntervalSeconds = 2

	commonCfg := config.DefaultCommonConfig()
	bstCfg := config.DefaultBTCStakingTrackerConfig()
	bstCfg.CheckDelegationsInterval = 1 * time.Second
	bstCfg.IndexerAddr = tm.Config.BTCStakingTracker.IndexerAddr
	stakingTrackerMetrics := metrics.NewBTCStakingTrackerMetrics()

	bsTracker := bst.NewBTCStakingTracker(
		tm.BTCClient,
		btcNode,
		tm.BabylonClient,
		&bstCfg,
		&commonCfg,
		zap.NewNop(),
		stakingTrackerMetrics,
		testutil.MakeTestBackend(t),
	)
	bsTracker.Start()

	activationMetrics := metrics.NewActivationUnbondingMonitorMetrics()
	activationMonitor := monitor.NewActivationUnbondingMonitor(
		babyAdapter,
		tm.BTCClient,
		&monitorCfg,
		zap.NewNop(),
		activationMetrics,
	)

	done := make(chan struct{})

	go func() {
		ticker := time.NewTicker(time.Duration(monitorCfg.TimingCheckIntervalSeconds) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := activationMonitor.CheckActivationTiming(); err != nil {
					t.Logf("Error checking activation timing: %v", err)
				}
			case <-done:
				return
			}
		}
	}()

	return &activationTestSetup{
		tm:                tm,
		activationMonitor: activationMonitor,
		metrics:           activationMetrics,
		done:              done,
		btcNode:           btcNode,
		bsTracker:         bsTracker,
	}
}

func (s *activationTestSetup) cleanup(t *testing.T) {
	close(s.done)
	s.bsTracker.Stop()
	s.btcNode.Stop()
	s.tm.Stop(t)
}

func (s *activationTestSetup) createAndMineVerifiedDelegation(t *testing.T) (*wire.MsgTx, chainhash.Hash) {
	_, fpSK := s.tm.CreateFinalityProvider(t)

	stakingMsgTx, _, _, _ := s.tm.CreateBTCDelegationWithoutIncl(t, fpSK)
	stakingMsgTxHash := stakingMsgTx.TxHash()

	_, err := s.tm.BTCClient.SendRawTransaction(stakingMsgTx, true)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		txns, err := s.tm.RetrieveTransactionFromMempool(t, []*chainhash.Hash{&stakingMsgTxHash})
		require.NoError(t, err)
		return len(txns) == 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	mBlock := s.tm.mineBlock(t)
	require.Equal(t, 2, len(mBlock.Transactions))

	require.Eventually(t, func() bool {
		_, err := s.tm.BTCClient.GetRawTransaction(&stakingMsgTxHash)
		return err == nil
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	return stakingMsgTx, stakingMsgTxHash
}

func (s *activationTestSetup) mineKDeepBlocks(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(3 * time.Second)
		btccParamsResp, err := s.tm.BabylonClient.BTCCheckpointParams()
		require.NoError(t, err)
		for i := 0; i < int(btccParamsResp.Params.BtcConfirmationDepth); i++ {
			s.tm.mineBlock(t)
			time.Sleep(100 * time.Millisecond)
		}
		s.tm.CatchUpBTCLightClient(t)
	}()
	wg.Wait()
}

func TestActivationTimingMonitor(t *testing.T) {
	s := setupActivationTest(t, 300)
	defer s.cleanup(t)

	stakingMsgTx, _ := s.createAndMineVerifiedDelegation(t)
	startTime := time.Now()

	s.mineKDeepBlocks(t)

	require.Eventually(t, func() bool {
		return promtestutil.ToFloat64(s.metrics.TrackedActivationGauge) > 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	require.Eventually(t, func() bool {
		resp, err := s.tm.BabylonClient.BTCDelegation(stakingMsgTx.TxHash().String())
		if err != nil {
			return false
		}
		return resp.BtcDelegation.Active
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	require.Eventually(t, func() bool {
		return promtestutil.ToFloat64(s.metrics.TrackedActivationGauge) == 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	endTime := time.Now()
	activationDuration := endTime.Sub(startTime)
	t.Logf("Activation timing test completed in %v", activationDuration)
}

func TestActivationTimeout(t *testing.T) {
	s := setupActivationTest(t, 1)
	defer s.cleanup(t)

	s.createAndMineVerifiedDelegation(t)
	startTime := time.Now()

	s.mineKDeepBlocks(t)

	require.Eventually(t, func() bool {
		return promtestutil.ToFloat64(s.metrics.TrackedActivationGauge) > 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	time.Sleep(7 * time.Second)

	require.Eventually(t, func() bool {
		return promtestutil.CollectAndCount(s.metrics.ActivationTimeoutsCounter) > 0
	}, eventuallyWaitTimeOut, eventuallyPollTime, "the alert should have gone off")

	endTime := time.Now()
	activationDuration := endTime.Sub(startTime)
	t.Logf("activation timing test completed in %v", activationDuration)
}

func TestDelegationEventuallyActivates(t *testing.T) {
	s := setupActivationTest(t, 1)
	defer s.cleanup(t)

	stakingMsgTx, _ := s.createAndMineVerifiedDelegation(t)

	s.mineKDeepBlocks(t)

	require.Eventually(t, func() bool {
		return promtestutil.ToFloat64(s.metrics.TrackedActivationGauge) > 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	time.Sleep(7 * time.Second)

	require.Eventually(t, func() bool {
		return promtestutil.CollectAndCount(s.metrics.ActivationTimeoutsCounter) > 0
	}, eventuallyWaitTimeOut, eventuallyPollTime, "timeout alert should have fired")

	require.Eventually(t, func() bool {
		resp, err := s.tm.BabylonClient.BTCDelegation(stakingMsgTx.TxHash().String())
		if err != nil {
			return false
		}
		return resp.BtcDelegation.Active
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	require.Eventually(t, func() bool {
		return promtestutil.ToFloat64(s.metrics.TrackedActivationGauge) == 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	metric := &dto.Metric{}
	err := s.metrics.ActivationDelayHistogram.Write(metric)
	require.NoError(t, err)
	hist := metric.GetHistogram()
	require.Greater(t, hist.GetSampleCount(), uint64(0), "histogram should record activation")
}

func TestAlertOnce(t *testing.T) {
	s := setupActivationTest(t, 1)
	defer s.cleanup(t)

	_, _ = s.createAndMineVerifiedDelegation(t)

	s.mineKDeepBlocks(t)

	require.Eventually(t, func() bool {
		gauge := promtestutil.ToFloat64(s.metrics.TrackedActivationGauge)
		t.Logf("TrackedActivationGauge: %v", gauge)
		return gauge > 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	time.Sleep(7 * time.Second)

	counter1 := promtestutil.CollectAndCount(s.metrics.ActivationTimeoutsCounter)

	require.Equal(t, 1, counter1, "alert should have fired once")

	time.Sleep(10 * time.Second)

	counter2 := promtestutil.CollectAndCount(s.metrics.ActivationTimeoutsCounter)

	require.Equal(t, 1, counter2, "alert should only fire once due to HasAlerted flag")
}

func TestMultipleDels(t *testing.T) {
	s := setupActivationTest(t, 300)
	defer s.cleanup(t)

	var stakingTxs []*wire.MsgTx
	for i := 0; i < 3; i++ {
		stakingMsgTx, _ := s.createAndMineVerifiedDelegation(t)
		stakingTxs = append(stakingTxs, stakingMsgTx)
	}

	s.mineKDeepBlocks(t)

	require.Eventually(t, func() bool {
		gauge := promtestutil.ToFloat64(s.metrics.TrackedActivationGauge)
		return gauge == 3.0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	require.Eventually(t, func() bool {
		activeCount := 0
		for _, tx := range stakingTxs {
			resp, err := s.tm.BabylonClient.BTCDelegation(tx.TxHash().String())
			if err == nil && resp.BtcDelegation.Active {
				activeCount++
			}
		}
		return activeCount == 3
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	require.Eventually(t, func() bool {
		gauge := promtestutil.ToFloat64(s.metrics.TrackedActivationGauge)
		return gauge == 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	timeoutCount := promtestutil.CollectAndCount(s.metrics.ActivationTimeoutsCounter)
	require.Equal(t, 0, timeoutCount,
		"no timeouts should occur with long timeout")

	metric := &dto.Metric{}
	err := s.metrics.ActivationDelayHistogram.Write(metric)
	require.NoError(t, err)
	hist := metric.GetHistogram()
	require.Equal(t, uint64(3), hist.GetSampleCount(), "histogram should record all 3 activations")
}
