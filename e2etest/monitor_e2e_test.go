package e2etest

import (
	"fmt"
	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/monitor"
	"github.com/babylonlabs-io/vigilante/testutil"
	"github.com/babylonlabs-io/vigilante/types"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMonitor(t *testing.T) {
	//t.Skip()
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

	dbBackend := testutil.MakeTestBackend(t)

	monitorMetrics := metrics.NewMonitorMetrics()
	genesisPath := fmt.Sprintf("%s/config/genesis.json", tm.Config.Babylon.KeyDirectory)
	genesisInfo, err := types.GetGenesisInfoFromFile(genesisPath)
	require.NoError(t, err)

	mon, err := monitor.New(
		&tm.Config.Monitor,
		&tm.Config.Common,
		logger,
		genesisInfo,
		tm.BabylonClient,
		tm.BTCClient,
		backend,
		monitorMetrics,
		dbBackend,
	)
	require.NoError(t, err)

	go mon.Start()

	// todo(lazar):
	// 1. pool for checkpoints on babylon
	// 2. send them to btc (fundrawtransaction/signrawtransaction/sendrawtransaction)
	// 3. stop monitor
	// 4. validate start from db

}
