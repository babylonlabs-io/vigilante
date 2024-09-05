package e2etest

import (
	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/monitor"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMonitor(t *testing.T) {
	t.Skip()
	numMatureOutputs := uint32(300)

	tm := StartManager(t, numMatureOutputs)
	defer tm.Stop(t)

	backend, err := btcclient.NewNodeBackend(
		btcclient.ToBitcoindConfig(tm.Config.BTC),
		&chaincfg.RegressionNetParams,
		&btcclient.EmptyHintCache{},
	)
	require.NoError(t, err)

	err = backend.Start()
	require.NoError(t, err)

	dbBackend, err := tm.Config.Monitor.DatabaseConfig.GetDbBackend()
	require.NoError(t, err)

	monitorMetrics := metrics.NewMonitorMetrics()

	mon, err := monitor.New(
		&tm.Config.Monitor,
		&tm.Config.Common,
		logger,
		nil,
		tm.BabylonClient,
		tm.BTCClient,
		backend,
		monitorMetrics,
		dbBackend,
	)
	require.NoError(t, err)

	mon.Start()

	defer mon.Stop()

}
