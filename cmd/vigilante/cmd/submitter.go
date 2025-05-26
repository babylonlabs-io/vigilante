package cmd

import (
	"fmt"

	bbnqccfg "github.com/babylonlabs-io/babylon/v4/client/config"
	bbnqc "github.com/babylonlabs-io/babylon/v4/client/query"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"

	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/submitter"
)

// GetSubmitterCmd returns the CLI commands for the submitter
func GetSubmitterCmd() *cobra.Command {
	var cfgFile = ""
	// Group epoching queries under a subcommand
	cmd := &cobra.Command{
		Use:   "submitter",
		Short: "Vigilant submitter",
		Run: func(_ *cobra.Command, _ []string) {
			// get the config from the given file or the default file
			cfg, err := config.New(cfgFile)
			if err != nil {
				panic(fmt.Errorf("failed to load config: %w", err))
			}
			rootLogger, err := cfg.CreateLogger()
			if err != nil {
				panic(fmt.Errorf("failed to create logger: %w", err))
			}

			// create BTC wallet and connect to BTC server
			btcWallet, err := btcclient.NewWallet(&cfg, rootLogger)
			if err != nil {
				panic(fmt.Errorf("failed to open BTC client: %w", err))
			}

			// create Babylon query client
			queryCfg := &bbnqccfg.BabylonQueryConfig{
				RPCAddr: cfg.Babylon.RPCAddr,
				Timeout: cfg.Babylon.Timeout,
			}
			err = queryCfg.Validate()
			if err != nil {
				panic(fmt.Errorf("invalid config for the query client: %w", err))
			}
			queryClient, err := bbnqc.New(queryCfg)
			if err != nil {
				panic(fmt.Errorf("failed to create babylon query client: %w", err))
			}

			// get submitter address
			submitterAddr, err := sdk.AccAddressFromBech32(cfg.Babylon.SubmitterAddress)
			if err != nil {
				panic(fmt.Errorf("invalid submitter address from config: %w", err))
			}

			// register submitter metrics
			submitterMetrics := metrics.NewSubmitterMetrics()

			dbBackend, err := cfg.Submitter.DatabaseConfig.GetDBBackend()
			if err != nil {
				panic(err)
			}

			// create submitter
			vigilantSubmitter, err := submitter.New(
				&cfg.Submitter,
				rootLogger,
				btcWallet,
				queryClient,
				submitterAddr,
				cfg.Common.RetrySleepTime,
				cfg.Common.MaxRetrySleepTime,
				cfg.Common.MaxRetryTimes,
				submitterMetrics,
				dbBackend,
				cfg.BTC.WalletName,
			)
			if err != nil {
				panic(fmt.Errorf("failed to create vigilante submitter: %w", err))
			}

			// start submitter and sync
			vigilantSubmitter.Start()

			// start Prometheus metrics server
			addr := fmt.Sprintf("%s:%d", cfg.Metrics.Host, cfg.Metrics.ServerPort)
			metrics.Start(addr, submitterMetrics.Registry)

			// SIGINT handling stuff
			addInterruptHandler(func() {
				rootLogger.Info("Stopping submitter...")
				vigilantSubmitter.Stop()
				rootLogger.Info("Submitter shutdown")
			})

			<-interruptHandlersDone
			rootLogger.Info("Shutdown complete")
		},
	}
	cmd.Flags().StringVar(&cfgFile, "config", config.DefaultConfigFile(), "config file")

	return cmd
}
