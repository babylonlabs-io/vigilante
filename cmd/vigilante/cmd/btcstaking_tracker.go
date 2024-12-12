package cmd

import (
	"fmt"

	bbnclient "github.com/babylonlabs-io/babylon/client/client"
	"github.com/babylonlabs-io/vigilante/btcclient"
	bst "github.com/babylonlabs-io/vigilante/btcstaking-tracker"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/rpcserver"
	"github.com/spf13/cobra"
)

func GetBTCStakingTracker() *cobra.Command {
	var babylonKeyDir string
	var cfgFile = ""
	var startHeight uint64

	cmd := &cobra.Command{
		Use:   "bstracker",
		Short: "BTC staking tracker",
		Run: func(_ *cobra.Command, _ []string) {
			var (
				err    error
				cfg    config.Config
				server *rpcserver.Server
			)

			// get the config from the given file or the default file
			cfg, err = config.New(cfgFile)
			if err != nil {
				panic(fmt.Errorf("failed to load config: %w", err))
			}
			// apply the flags from CLI
			if len(babylonKeyDir) != 0 {
				cfg.Babylon.KeyDirectory = babylonKeyDir
			}

			rootLogger, err := cfg.CreateLogger()
			if err != nil {
				panic(fmt.Errorf("failed to create logger: %w", err))
			}

			// apply the flags from CLI
			if len(babylonKeyDir) != 0 {
				cfg.Babylon.KeyDirectory = babylonKeyDir
			}

			// create Babylon client. Note that requests from Babylon client are ad hoc
			bbnClient, err := bbnclient.New(&cfg.Babylon, nil)
			if err != nil {
				panic(fmt.Errorf("failed to open Babylon client: %w", err))
			}

			// start Babylon client so that WebSocket subscriber can work
			if err := bbnClient.Start(); err != nil {
				panic(fmt.Errorf("failed to start WebSocket connection with Babylon: %w", err))
			}

			// create BTC client and connect to BTC server
			// Note that monitor needs to subscribe to new BTC blocks
			btcClient, err := btcclient.NewWallet(&cfg, rootLogger)

			if err != nil {
				panic(fmt.Errorf("failed to open BTC client: %w", err))
			}

			// create BTC notifier
			// TODO: is it possible to merge BTC client and BTC notifier?
			btcNotifier, err := btcclient.NewNodeBackendWithParams(cfg.BTC)
			if err != nil {
				panic(err)
			}

			bsMetrics := metrics.NewBTCStakingTrackerMetrics()

			bstracker := bst.NewBTCStakingTracker(
				btcClient,
				btcNotifier,
				bbnClient,
				&cfg.BTCStakingTracker,
				&cfg.Common,
				rootLogger,
				bsMetrics,
			)

			// create RPC server
			server, err = rpcserver.New(&cfg.GRPC, rootLogger, nil, nil, nil, bstracker)
			if err != nil {
				panic(fmt.Errorf("failed to create reporter's RPC server: %w", err))
			}

			if err := btcNotifier.Start(); err != nil {
				panic(fmt.Errorf("failed to start btc chain notifier: %w", err))
			}

			// bootstrap
			if err := bstracker.Bootstrap(startHeight); err != nil {
				panic(err)
			}

			err = bstracker.Start()

			if err != nil {
				panic(fmt.Errorf("failed to start unbonding watcher: %w", err))
			}

			// start RPC server
			server.Start()
			// start Prometheus metrics server
			addr := fmt.Sprintf("%s:%d", cfg.Metrics.Host, cfg.Metrics.ServerPort)
			metrics.Start(addr, bsMetrics.Registry)

			// SIGINT handling stuff
			addInterruptHandler(func() {
				// TODO: Does this need to wait for the grpc server to finish up any requests?
				rootLogger.Info("Stopping RPC server...")
				server.Stop()
				rootLogger.Info("RPC server shutdown")
			})
			addInterruptHandler(func() {
				rootLogger.Info("Stopping unbonding watcher...")
				if err := bstracker.Stop(); err != nil {
					panic(fmt.Errorf("failed to stop unbonding watcher: %w", err))
				}
				rootLogger.Info("Unbonding watcher shutdown")
			})
			addInterruptHandler(func() {
				rootLogger.Info("Stopping BTC notifier...")
				if err := bstracker.Stop(); err != nil {
					panic(fmt.Errorf("failed to stop btc chain notifier: %w", err))
				}
				rootLogger.Info("BTC notifier shutdown")
			})
			addInterruptHandler(func() {
				rootLogger.Info("Stopping Babylon client...")
				if err := bbnClient.Stop(); err != nil {
					panic(fmt.Errorf("failed to stop Babylon client: %w", err))
				}
				rootLogger.Info("Babylon client shutdown")
			})

			<-interruptHandlersDone
			rootLogger.Info("Shutdown complete")
		},
	}
	cmd.Flags().StringVar(&babylonKeyDir, "babylon-key", "", "Directory of the Babylon key")
	cmd.Flags().StringVar(&cfgFile, "config", config.DefaultConfigFile(), "config file")
	cmd.Flags().Uint64Var(&startHeight, "start-height", 0, "height that the BTC slasher starts scanning for evidences")

	return cmd
}
