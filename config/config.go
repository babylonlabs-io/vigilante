package config

import (
	"bytes"
	"errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"time"

	bbncfg "github.com/babylonlabs-io/babylon/client/config"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

const (
	defaultConfigFilename = "vigilante.yml"
	defaultDataDirname    = "data"
)

var (
	defaultAppDataDir = btcutil.AppDataDir("babylon-vigilante", false)
	defaultConfigFile = filepath.Join(defaultAppDataDir, defaultConfigFilename)
)

func DataDir(homePath string) string {
	return filepath.Join(homePath, defaultDataDirname)
}

// Config defines the server's top level configuration
type Config struct {
	Common            CommonConfig            `mapstructure:"common"`
	BTC               BTCConfig               `mapstructure:"btc"`
	Babylon           bbncfg.BabylonConfig    `mapstructure:"babylon"`
	Metrics           MetricsConfig           `mapstructure:"metrics"`
	Submitter         SubmitterConfig         `mapstructure:"submitter"`
	Reporter          ReporterConfig          `mapstructure:"reporter"`
	Monitor           MonitorConfig           `mapstructure:"monitor"`
	BTCStakingTracker BTCStakingTrackerConfig `mapstructure:"btcstaking-tracker"`
}

func (cfg *Config) Validate() error {
	if err := cfg.Common.Validate(); err != nil {
		return fmt.Errorf("invalid config in common: %w", err)
	}

	if err := cfg.BTC.Validate(); err != nil {
		return fmt.Errorf("invalid config in btc: %w", err)
	}

	if err := cfg.Babylon.Validate(); err != nil {
		return fmt.Errorf("invalid config in babylon: %w", err)
	}

	if err := cfg.Metrics.Validate(); err != nil {
		return fmt.Errorf("invalid config in metrics: %w", err)
	}

	if err := cfg.Submitter.Validate(); err != nil {
		return fmt.Errorf("invalid config in submitter: %w", err)
	}

	if err := cfg.Reporter.Validate(); err != nil {
		return fmt.Errorf("invalid config in reporter: %w", err)
	}

	if err := cfg.Monitor.Validate(); err != nil {
		return fmt.Errorf("invalid config in monitor: %w", err)
	}

	if err := cfg.BTCStakingTracker.Validate(); err != nil {
		return fmt.Errorf("invalid config in BTC staking tracker: %w", err)
	}

	return nil
}

func (cfg *Config) CreateLogger() (*zap.Logger, error) {
	return cfg.Common.CreateLogger()
}

func DefaultConfigFile() string {
	return defaultConfigFile
}

// DefaultConfig returns server's default configuration.
func DefaultConfig() *Config {
	defaultBbnCfg := bbncfg.DefaultBabylonConfig()
	defaultBbnCfg.BlockTimeout = 10 * time.Minute

	return &Config{
		Common:            DefaultCommonConfig(),
		BTC:               DefaultBTCConfig(),
		Babylon:           defaultBbnCfg,
		Metrics:           DefaultMetricsConfig(),
		Submitter:         DefaultSubmitterConfig(),
		Reporter:          DefaultReporterConfig(),
		Monitor:           DefaultMonitorConfig(),
		BTCStakingTracker: DefaultBTCStakingTrackerConfig(),
	}
}

// New returns a fully parsed Config object from a given file directory
func New(configFile string) (Config, error) {
	if _, err := os.Stat(configFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// The given config file does not exist
			return Config{}, fmt.Errorf("no config file found at %s", configFile)
		}
		// Other errors
		return Config{}, err
	}

	// File exists, so parse it
	viper.SetConfigFile(configFile)
	if err := viper.ReadInConfig(); err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return Config{}, err
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// SaveToYAML saves the configuration to a YAML file
func (cfg *Config) SaveToYAML(filePath string) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)

	if err := enc.Encode(cfg); err != nil {
		return fmt.Errorf("error marshaling config to YAML: %w", err)
	}

	if err := enc.Close(); err != nil {
		return fmt.Errorf("error closing YAML encoder: %w", err)
	}

	if err := os.WriteFile(filePath, buf.Bytes(), 0600); err != nil {
		return fmt.Errorf("error writing YAML to file: %w", err)
	}

	return nil
}
