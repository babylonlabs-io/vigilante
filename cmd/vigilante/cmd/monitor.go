package cmd

import (
	"fmt"

	bbnqccfg "github.com/babylonlabs-io/babylon/v3/client/config"
	bbnqc "github.com/babylonlabs-io/babylon/v3/client/query"
	"github.com/spf13/cobra"

	bbnclient "github.com/babylonlabs-io/babylon/v3/client/client"
	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/monitor"
	"github.com/babylonlabs-io/vigilante/types"
)

const (
	genesisFileNameFlag    = "genesis"
	GenesisFileNameDefault = "genesis.json"
)

// GetMonitorCmd returns the CLI commands for the monitor
func GetMonitorCmd() *cobra.Command {
	var genesisFile string
	var cfgFile = ""
	// Group monitor queries under a subcommand
	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "Vigilante monitor constantly checks the consistency between the Babylon node and BTC and detects censorship of BTC checkpoints",
		Run: func(_ *cobra.Command, _ []string) {
			var (
				err              error
				cfg              config.Config
				btcClient        *btcclient.Client
				bbnQueryClient   *bbnqc.QueryClient
				vigilanteMonitor *monitor.Monitor
			)

			// get the config from the given file or the default file
			cfg, err = config.New(cfgFile)
			if err != nil {
				panic(fmt.Errorf("failed to load config: %w", err))
			}
			rootLogger, err := cfg.CreateLogger()
			if err != nil {
				panic(fmt.Errorf("failed to create logger: %w", err))
			}

			// create Babylon query client. Note that requests from Babylon client are ad hoc
			queryCfg := &bbnqccfg.BabylonQueryConfig{
				RPCAddr: cfg.Babylon.RPCAddr,
				Timeout: cfg.Babylon.Timeout,
			}
			if err := queryCfg.Validate(); err != nil {
				panic(fmt.Errorf("invalid config for query client: %w", err))
			}
			bbnQueryClient, err = bbnqc.New(queryCfg)
			if err != nil {
				panic(fmt.Errorf("failed to create babylon query client: %w", err))
			}

			// create BTC client and connect to BTC server
			btcClient, err = btcclient.NewWallet(&cfg, rootLogger)
			if err != nil {
				panic(fmt.Errorf("failed to open BTC client: %w", err))
			}
			genesisInfo, err := types.GetGenesisInfoFromFile(genesisFile)
			if err != nil {
				panic(fmt.Errorf("failed to read genesis file: %w", err))
			}

			// register monitor metrics
			monitorMetrics := metrics.NewMonitorMetrics()

			// create the chain notifier
			btcNotifier, err := btcclient.NewNodeBackendWithParams(cfg.BTC)
			if err != nil {
				panic(err)
			}

			dbBackend, err := cfg.Monitor.DatabaseConfig.GetDBBackend()
			if err != nil {
				panic(err)
			}

			babylonClient, err := bbnclient.New(&cfg.Babylon, nil)
			if err != nil {
				panic(fmt.Errorf("failed to create babylon client: %w", err))
			}

			babylonAdaptorClient := monitor.NewBabylonAdaptorClientAdapter(babylonClient, &cfg.Monitor)

			// create monitor
			vigilanteMonitor, err = monitor.New(
				&cfg.Monitor,
				&cfg.Common,
				rootLogger,
				genesisInfo,
				bbnQueryClient,
				btcClient,
				btcNotifier,
				monitorMetrics,
				babylonAdaptorClient,
				dbBackend,
			)
			if err != nil {
				panic(fmt.Errorf("failed to create vigilante monitor: %w", err))
			}

			// start
			go vigilanteMonitor.Start(genesisInfo.GetBaseBTCHeight())

			// start Prometheus metrics server
			addr := fmt.Sprintf("%s:%d", cfg.Metrics.Host, cfg.Metrics.ServerPort)
			metrics.Start(addr, monitorMetrics.Registry)

			addInterruptHandler(func() {
				rootLogger.Info("Stopping monitor...")
				vigilanteMonitor.Stop()
				rootLogger.Info("Monitor shutdown")
			})

			<-interruptHandlersDone
			rootLogger.Info("Shutdown complete")
		},
	}
	cmd.Flags().StringVar(&genesisFile, genesisFileNameFlag, GenesisFileNameDefault, "genesis file")
	cmd.Flags().StringVar(&cfgFile, "config", config.DefaultConfigFile(), "config file")

	return cmd
}
