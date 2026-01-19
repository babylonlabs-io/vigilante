package config

import (
	"fmt"

	"github.com/babylonlabs-io/vigilante/types"
)

const (
	minBTCCacheSize = 1000
	maxHeadersInMsg = 100 // maximum number of headers in a MsgInsertHeaders message

	// Default ETH backend parameters (used when Babylon client is not available)
	DefaultBTCConfirmationDepth          = uint32(6)
	DefaultCheckpointFinalizationTimeout = uint32(100)
	DefaultCheckpointTag                 = "01020304"
)

// BackendType defines the type of backend for header submission
type BackendType string

const (
	BackendTypeBabylon  BackendType = "babylon"
	BackendTypeEthereum BackendType = "ethereum"
)

// ReporterConfig defines configuration for the reporter.
type ReporterConfig struct {
	NetParams       string      `mapstructure:"netparams"`          // should be mainnet|testnet|simnet|signet
	BTCCacheSize    uint32      `mapstructure:"btc_cache_size"`     // size of the BTC cache
	MaxHeadersInMsg uint32      `mapstructure:"max_headers_in_msg"` // maximum number of headers in a MsgInsertHeaders message
	BackendType     BackendType `mapstructure:"backend_type"`       // backend type: babylon or ethereum

	// ETH backend params (used when Babylon client is not available)
	BTCConfirmationDepth          uint32 `mapstructure:"btc_confirmation_depth"`
	CheckpointFinalizationTimeout uint32 `mapstructure:"checkpoint_finalization_timeout"`
	CheckpointTag                 string `mapstructure:"checkpoint_tag"`
}

func (cfg *ReporterConfig) Validate() error {
	if _, ok := types.GetValidNetParams()[cfg.NetParams]; !ok {
		return fmt.Errorf("invalid net params")
	}
	if cfg.BTCCacheSize < minBTCCacheSize {
		return fmt.Errorf("BTC cache size has to be at least %d", minBTCCacheSize)
	}
	if cfg.MaxHeadersInMsg < maxHeadersInMsg {
		return fmt.Errorf("max_headers_in_msg has to be at least %d", maxHeadersInMsg)
	}

	// Validate backend type
	switch cfg.BackendType {
	case BackendTypeBabylon, BackendTypeEthereum:
		// valid backend types
	case "":
		cfg.BackendType = BackendTypeBabylon // default to babylon for backwards compatibility
	default:
		return fmt.Errorf("invalid backend_type: %s (must be babylon or ethereum)", cfg.BackendType)
	}

	// Set defaults for ETH backend parameters
	if cfg.BackendType == BackendTypeEthereum {
		if cfg.BTCConfirmationDepth == 0 {
			cfg.BTCConfirmationDepth = DefaultBTCConfirmationDepth
		}
		if cfg.CheckpointFinalizationTimeout == 0 {
			cfg.CheckpointFinalizationTimeout = DefaultCheckpointFinalizationTimeout
		}
		if cfg.CheckpointTag == "" {
			cfg.CheckpointTag = DefaultCheckpointTag
		}
	}

	return nil
}

func DefaultReporterConfig() ReporterConfig {
	return ReporterConfig{
		NetParams:       types.BtcSimnet.String(),
		BTCCacheSize:    minBTCCacheSize,
		MaxHeadersInMsg: maxHeadersInMsg,
		BackendType:     BackendTypeBabylon, // default to babylon backend
	}
}
