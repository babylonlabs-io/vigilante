package config

import (
	"fmt"

	"github.com/babylonlabs-io/vigilante/types"
)

const (
	minBTCCacheSize = 1000
	maxHeadersInMsg = 100 // maximum number of headers in a MsgInsertHeaders message
)

// ReporterConfig defines configuration for the reporter.
type ReporterConfig struct {
	NetParams       string `mapstructure:"netparams"`          // should be mainnet|testnet|simnet|signet
	BTCCacheSize    uint32 `mapstructure:"btc_cache_size"`     // size of the BTC cache
	MaxHeadersInMsg uint32 `mapstructure:"max_headers_in_msg"` // maximum number of headers in a MsgInsertHeaders message
	BackendType     string `mapstructure:"backend_type"`       // backend type: babylon or ethereum
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
	case "babylon", "ethereum":
		// valid backend types
	case "":
		cfg.BackendType = "babylon" // default to babylon for backwards compatibility
	default:
		return fmt.Errorf("invalid backend_type: %s (must be babylon or ethereum)", cfg.BackendType)
	}

	return nil
}

func DefaultReporterConfig() ReporterConfig {
	return ReporterConfig{
		NetParams:       types.BtcSimnet.String(),
		BTCCacheSize:    minBTCCacheSize,
		MaxHeadersInMsg: maxHeadersInMsg,
		BackendType:     "babylon", // default to babylon backend
	}
}
