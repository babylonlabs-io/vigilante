package cmd

import (
	"fmt"

	bbnclient "github.com/babylonlabs-io/babylon/v4/client/client"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/reporter"
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
			btcClient, err = btcclient.NewWallet(&cfg, rootLogger)
			if err != nil {
				panic(fmt.Errorf("failed to open BTC client: %w", err))
			}

			// Only create Babylon client if using Babylon backend
			if cfg.Reporter.BackendType == config.BackendTypeBabylon {
				babylonClient, err = bbnclient.New(&cfg.Babylon, nil)
				if err != nil {
					panic(fmt.Errorf("failed to open Babylon client: %w", err))
				}
				rootLogger.Info("Using Babylon backend")
			} else {
				rootLogger.Info("Using Ethereum backend")
			}

			// register reporter metrics
			reporterMetrics := metrics.NewReporterMetrics()

			// create the chain notifier
			btcNotifier, err := btcclient.NewNodeBackendWithParams(cfg.BTC)
			if err != nil {
				panic(err)
			}

			// create backend based on configuration
			var backend reporter.Backend
			switch cfg.Reporter.BackendType {
			case config.BackendTypeBabylon:
				backend = reporter.NewBabylonBackend(babylonClient)
				rootLogger.Info("Using Babylon backend for BTC header submission")
			case config.BackendTypeEthereum:
				backend, err = reporter.NewEthereumBackend(&cfg.Ethereum, rootLogger)
				if err != nil {
					panic(fmt.Errorf("failed to create Ethereum backend: %w", err))
				}
				rootLogger.Info("Using Ethereum backend for BTC header submission")
			default:
				panic(fmt.Errorf("unsupported backend type: %s", cfg.Reporter.BackendType))
			}

			// create reporter
			// Note: Must pass nil explicitly for ETH backend to avoid Go's typed-nil interface gotcha
			// (a typed nil pointer wrapped in an interface is not nil)
			var bbnClientForReporter reporter.BabylonClient
			if babylonClient != nil {
				bbnClientForReporter = babylonClient
			}
			vigilantReporter, err = reporter.New(
				&cfg.Reporter,
				rootLogger,
				btcClient,
				backend,
				bbnClientForReporter,
				btcNotifier,
				cfg.Common.RetrySleepTime,
				cfg.Common.MaxRetrySleepTime,
				cfg.Common.MaxRetryTimes,
				reporterMetrics,
			)
			if err != nil {
				panic(fmt.Errorf("failed to create vigilante reporter: %w", err))
			}

			// start Prometheus metrics server
			addr := fmt.Sprintf("%s:%d", cfg.Metrics.Host, cfg.Metrics.ServerPort)
			metrics.Start(addr, reporterMetrics.Registry)

			// start normal-case execution
			vigilantReporter.Start()

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

			// Only stop Babylon client if it exists
			if babylonClient != nil {
				addInterruptHandler(func() {
					rootLogger.Info("Stopping Babylon client...")
					if err := babylonClient.Stop(); err != nil {
						rootLogger.Warn("Error stopping Babylon client", zap.Error(err))
					}
					rootLogger.Info("Babylon client shutdown")
				})
			}

			<-interruptHandlersDone
			rootLogger.Info("Shutdown complete")
		},
	}
	cmd.Flags().StringVar(&babylonKeyDir, "babylon-key-dir", "", "Directory of the Babylon key")
	cmd.Flags().StringVar(&cfgFile, "config", config.DefaultConfigFile(), "config file")

	return cmd
}
