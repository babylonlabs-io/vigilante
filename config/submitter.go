package config

import (
	"errors"

	"github.com/babylonlabs-io/vigilante/types"
)

const (
	DefaultCheckpointCacheMaxEntries = 100
	DefaultPollingIntervalSeconds    = 60   // in seconds
	DefaultResendIntervalSeconds     = 1800 // 30 minutes
	DefaultResubmitFeeMultiplier     = 1
	DefaultInsufficientFeeMargin     = 0.15
	DefaultInsufficientFeerateMargin = 0.15
	DefaultFeeIncrementMargin        = 0.15
)

// SubmitterConfig defines configuration for the gRPC-web server.
type SubmitterConfig struct {
	// NetParams defines the BTC network params, which should be mainnet|testnet|simnet|signet
	NetParams string `mapstructure:"netparams" yaml:"netparams"`
	// BufferSize defines the number of raw checkpoints stored in the buffer
	BufferSize uint `mapstructure:"buffer-size" yaml:"buffer-size"`
	// ResubmitFeeMultiplier is used to multiply the estimated bumped fee in resubmission
	ResubmitFeeMultiplier float64 `mapstructure:"resubmit-fee-multiplier" yaml:"resubmit-fee-multiplier"`
	// PollingIntervalSeconds defines the intervals (in seconds) between each polling of Babylon checkpoints
	PollingIntervalSeconds int64 `mapstructure:"polling-interval-seconds" yaml:"polling-interval-seconds"`
	// ResendIntervalSeconds defines the time (in seconds) which the submitter awaits
	// before resubmitting checkpoints to BTC
	ResendIntervalSeconds uint `mapstructure:"resend-interval-seconds" yaml:"resend-interval-seconds"`
	// DatabaseConfig stores last submitted txn
	DatabaseConfig *DBConfig `mapstructure:"dbconfig" yaml:"dbconfig"`
	// InsufficientFeeMargin is the margin of fee adjustment when the fee is insufficient, e.g. 0.15 for 15%
	// generally this shouldn't be a large percentage 5%-15% is reasonable
	InsufficientFeeMargin float64 `mapstructure:"insufficient_fee_margin" yaml:"insufficient_fee_margin"`
	// InsufficientFeerateMargin is the margin of feerate adjustment when the feerate is insufficient, e.g. 0.15 for 15%
	// generally this shouldn't be a large percentage 5%-15% is reasonable
	InsufficientFeerateMargin float64 `mapstructure:"insufficient_feerate_margin" yaml:"insufficient_feerate_margin"` // e.g. 0.15 for 15%
	// FeeIncrementMargin is the margin of fee increment when the fee is insufficient, e.g. 0.15 for 15%
	// generally this shouldn't be a large percentage 5%-15% is reasonable
	FeeIncrementMargin float64 `mapstructure:"fee_increment_margin" yaml:"fee_increment_margin"`
}

func (cfg *SubmitterConfig) Validate() error {
	if _, ok := types.GetValidNetParams()[cfg.NetParams]; !ok {
		return errors.New("invalid net params")
	}

	if cfg.ResubmitFeeMultiplier < 1 {
		return errors.New("invalid resubmit-fee-multiplier, should not be less than 1")
	}

	if cfg.PollingIntervalSeconds < 0 {
		return errors.New("invalid polling-interval-seconds, should be positive")
	}

	if cfg.ResendIntervalSeconds <= 0 {
		return errors.New("invalid resend-interval-seconds, should be positive")
	}

	if cfg.BufferSize <= 0 {
		return errors.New("invalid buffer-size, should be positive")
	}

	if cfg.DatabaseConfig == nil {
		return errors.New("invalid dbconfig")
	}

	if cfg.InsufficientFeeMargin < 0 {
		return errors.New("invalid insufficient_fee_margin, should be positive")
	}

	if cfg.InsufficientFeerateMargin < 0 {
		return errors.New("invalid insufficient_feerate_margin, should be positive")
	}

	if cfg.FeeIncrementMargin < 0 {
		return errors.New("invalid fee_increment_margin, should be positive")
	}

	return nil
}

func DefaultSubmitterConfig() SubmitterConfig {
	return SubmitterConfig{
		NetParams:                 types.BtcSimnet.String(),
		BufferSize:                DefaultCheckpointCacheMaxEntries,
		ResubmitFeeMultiplier:     DefaultResubmitFeeMultiplier,
		PollingIntervalSeconds:    DefaultPollingIntervalSeconds,
		ResendIntervalSeconds:     DefaultResendIntervalSeconds,
		InsufficientFeerateMargin: DefaultInsufficientFeerateMargin,
		InsufficientFeeMargin:     DefaultInsufficientFeeMargin,
		FeeIncrementMargin:        DefaultFeeIncrementMargin,
	}
}
