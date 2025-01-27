//go:build e2e
// +build e2e

package e2etest

import (
	"fmt"
	bbnclient "github.com/babylonlabs-io/babylon/client/client"
	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/monitor"
	"github.com/babylonlabs-io/vigilante/reporter"
	"github.com/babylonlabs-io/vigilante/submitter"
	"github.com/babylonlabs-io/vigilante/testutil"
	"github.com/babylonlabs-io/vigilante/types"
	"github.com/btcsuite/btcd/chaincfg"
	sdk "github.com/cosmos/cosmos-sdk/types"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"time"

	"testing"
)

// TestMonitorBootstrap - validates that after a restart monitor bootstraps from DB
func TestMonitorBootstrap(t *testing.T) {
	t.Parallel()
	numMatureOutputs := uint32(150)

	tm := StartManager(t, numMatureOutputs, 2)
	defer tm.Stop(t)

	backend, err := btcclient.NewNodeBackend(
		btcclient.ToBitcoindConfig(tm.Config.BTC),
		&chaincfg.RegressionNetParams,
		&btcclient.EmptyHintCache{},
	)
	require.NoError(t, err)

	err = backend.Start()
	require.NoError(t, err)

	dbBackend := testutil.MakeTestBackend(t)

	monitorMetrics := metrics.NewMonitorMetrics()
	genesisPath := fmt.Sprintf("%s/config/genesis.json", tm.Config.Babylon.KeyDirectory)
	genesisInfo, err := types.GetGenesisInfoFromFile(genesisPath)
	require.NoError(t, err)

	tm.Config.Submitter.PollingIntervalSeconds = 1
	subAddr, _ := sdk.AccAddressFromBech32(submitterAddrStr)

	// create submitter
	vigilantSubmitter, err := submitter.New(
		&tm.Config.Submitter,
		logger,
		tm.BTCClient,
		tm.BabylonClient,
		subAddr,
		tm.Config.Common.RetrySleepTime,
		tm.Config.Common.MaxRetrySleepTime,
		tm.Config.Common.MaxRetryTimes,
		metrics.NewSubmitterMetrics(),
		testutil.MakeTestBackend(t),
		tm.Config.BTC.WalletName,
	)

	require.NoError(t, err)

	vigilantSubmitter.Start()
	defer vigilantSubmitter.Stop()

	vigilantReporter, err := reporter.New(
		&tm.Config.Reporter,
		logger,
		tm.BTCClient,
		tm.BabylonClient,
		backend,
		tm.Config.Common.RetrySleepTime,
		tm.Config.Common.MaxRetrySleepTime,
		metrics.NewReporterMetrics(),
	)
	require.NoError(t, err)

	defer func() {
		vigilantSubmitter.Stop()
		vigilantSubmitter.WaitForShutdown()
	}()

	mon, err := monitor.New(
		&tm.Config.Monitor,
		&tm.Config.Common,
		zap.NewNop(),
		genesisInfo,
		tm.BabylonClient,
		tm.BTCClient,
		backend,
		monitorMetrics,
		dbBackend,
	)
	require.NoError(t, err)
	vigilantReporter.Start()
	defer vigilantReporter.Stop()

	go func() {
		ticker := time.NewTicker(3 * time.Second)
		timer := time.NewTimer(15 * time.Second)
		defer timer.Stop()
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				tm.mineBlock(t)
			case <-timer.C:
				return
			}
		}
	}()

	go mon.Start(genesisInfo.GetBaseBTCHeight())

	time.Sleep(15 * time.Second)
	mon.Stop()

	// use a new bbn client
	babylonClient, err := bbnclient.New(&tm.Config.Babylon, nil)
	require.NoError(t, err)
	defer babylonClient.Stop()

	mon, err = monitor.New(
		&tm.Config.Monitor,
		&tm.Config.Common,
		zap.NewNop(),
		genesisInfo,
		babylonClient,
		tm.BTCClient,
		backend,
		monitorMetrics,
		dbBackend,
	)
	require.NoError(t, err)
	go mon.Start(genesisInfo.GetBaseBTCHeight())

	defer mon.Stop()

	require.Zero(t, promtestutil.ToFloat64(mon.Metrics().InvalidBTCHeadersCounter))
	require.Zero(t, promtestutil.ToFloat64(mon.Metrics().InvalidEpochsCounter))
	require.Eventually(t, func() bool {
		return mon.BTCScanner.GetBaseHeight() > genesisInfo.GetBaseBTCHeight()
	}, eventuallyWaitTimeOut, eventuallyPollTime)
}
