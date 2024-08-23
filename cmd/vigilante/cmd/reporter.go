package cmd

import (
	"fmt"
	"github.com/babylonlabs-io/vigilante/netparams"

	bbnclient "github.com/babylonlabs-io/babylon/client/client"
	"github.com/spf13/cobra"

	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/reporter"
	"github.com/babylonlabs-io/vigilante/rpcserver"
)

// GetReporterCmd returns the CLI commands for the reporter
func GetReporterCmd() *cobra.Command {
	var babylonKeyDir string
	var cfgFile = ""

	cmd := &cobra.Command{
		Use:   "reporter",
		Short: "Vigilant reporter",
		Run: func(_ *cobra.Command, _ []string) {
			var (
				err              error
				cfg              config.Config
				btcClient        *btcclient.Client
				babylonClient    *bbnclient.Client
				vigilantReporter *reporter.Reporter
				server           *rpcserver.Server
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

			// apply the flags from CLI
			if len(babylonKeyDir) != 0 {
				cfg.Babylon.KeyDirectory = babylonKeyDir
			}

			// create BTC client and connect to BTC server
			// Note that vigilant reporter needs to subscribe to new BTC blocks
			btcClient, err = btcclient.NewWallet(&cfg.BTC, rootLogger)
			if err != nil {
				panic(fmt.Errorf("failed to open BTC client: %w", err))
			}

			// create Babylon client. Note that requests from Babylon client are ad hoc
			babylonClient, err = bbnclient.New(&cfg.Babylon, nil)
			if err != nil {
				panic(fmt.Errorf("failed to open Babylon client: %w", err))
			}

			// register reporter metrics
			reporterMetrics := metrics.NewReporterMetrics()

			// create the chain notifier
			btcParams, err := netparams.GetBTCParams(cfg.BTC.NetParams)
			if err != nil {
				panic(fmt.Errorf("failed to get BTC net params: %w", err))
			}
			btcCfg := btcclient.CfgToBtcNodeBackendConfig(cfg.BTC, "")
			btcNotifier, err := btcclient.NewNodeBackend(btcCfg, btcParams, &btcclient.EmptyHintCache{})
			if err != nil {
				panic(fmt.Errorf("failed to initialize notifier: %w", err))
			}

			// create reporter
			vigilantReporter, err = reporter.New(
				&cfg.Reporter,
				rootLogger,
				btcClient,
				babylonClient,
				btcNotifier,
				cfg.Common.RetrySleepTime,
				cfg.Common.MaxRetrySleepTime,
				reporterMetrics,
			)
			if err != nil {
				panic(fmt.Errorf("failed to create vigilante reporter: %w", err))
			}

			// create RPC server
			server, err = rpcserver.New(&cfg.GRPC, rootLogger, nil, vigilantReporter, nil, nil)
			if err != nil {
				panic(fmt.Errorf("failed to create reporter's RPC server: %w", err))
			}

			// start normal-case execution
			vigilantReporter.Start()

			// start RPC server
			server.Start()
			// start Prometheus metrics server
			addr := fmt.Sprintf("%s:%d", cfg.Metrics.Host, cfg.Metrics.ServerPort)
			metrics.Start(addr, reporterMetrics.Registry)

			// SIGINT handling stuff
			addInterruptHandler(func() {
				// TODO: Does this need to wait for the grpc server to finish up any requests?
				rootLogger.Info("Stopping RPC server...")
				server.Stop()
				rootLogger.Info("RPC server shutdown")
			})
			addInterruptHandler(func() {
				rootLogger.Info("Stopping reporter...")
				vigilantReporter.Stop()
				rootLogger.Info("Reporter shutdown")
			})
			addInterruptHandler(func() {
				rootLogger.Info("Stopping BTC client...")
				btcClient.Stop()
				btcClient.WaitForShutdown()
				rootLogger.Info("BTC client shutdown")
			})

			<-interruptHandlersDone
			rootLogger.Info("Shutdown complete")

		},
	}
	cmd.Flags().StringVar(&babylonKeyDir, "babylon-key-dir", "", "Directory of the Babylon key")
	cmd.Flags().StringVar(&cfgFile, "config", config.DefaultConfigFile(), "config file")
	return cmd
}
