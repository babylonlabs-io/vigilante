package config

import (
	"fmt"
	"time"
)

// EthereumConfig holds configuration for submitting BTC headers to Ethereum
type EthereumConfig struct {
	// RPC endpoint URL
	RPCURL string `mapstructure:"rpc-url"`

	// Private key in hex format (with or without 0x prefix)
	// TODO: Replace with keystore support in future PR (for production security)
	PrivateKey string `mapstructure:"private-key"`

	// BtcPrism contract address
	ContractAddress string `mapstructure:"contract-address"`

	// Ethereum chain ID (1 for mainnet, 11155111 for Sepolia, etc.)
	ChainID uint64 `mapstructure:"chain-id"`

	// Gas limit for submit transactions (0 for auto-estimation)
	GasLimit uint64 `mapstructure:"gas-limit"`

	// Maximum gas price in Gwei (0 for no limit)
	MaxGasPrice uint64 `mapstructure:"max-gas-price"`

	// Transaction confirmation mode: "receipt" (fast), "safe" (recommended), or "finalized"
	ConfirmationMode string `mapstructure:"confirmation-mode"`

	// Timeout for waiting for transaction confirmation
	ConfirmationTimeout time.Duration `mapstructure:"confirmation-timeout"`

	// Maximum number of blocks to wait for safe/finalized confirmation
	MaxConfirmationBlocks uint64 `mapstructure:"max-confirmation-blocks"`
}

// Validate checks the Ethereum configuration
func (cfg *EthereumConfig) Validate() error {
	if cfg.RPCURL == "" {
		return fmt.Errorf("ethereum rpc-url is required")
	}

	if cfg.PrivateKey == "" {
		return fmt.Errorf("ethereum private-key is required")
	}

	if cfg.ContractAddress == "" {
		return fmt.Errorf("ethereum contract-address is required")
	}

	if cfg.ChainID == 0 {
		return fmt.Errorf("ethereum chain-id is required")
	}

	// Validate confirmation mode
	switch cfg.ConfirmationMode {
	case "receipt", "safe", "finalized":
		// valid modes
	case "":
		cfg.ConfirmationMode = "safe" // default
	default:
		return fmt.Errorf("invalid confirmation-mode: %s (must be receipt, safe, or finalized)", cfg.ConfirmationMode)
	}

	if cfg.ConfirmationTimeout == 0 {
		cfg.ConfirmationTimeout = 5 * time.Minute // default
	}

	if cfg.MaxConfirmationBlocks == 0 {
		cfg.MaxConfirmationBlocks = 100 // default
	}

	return nil
}

// DefaultEthereumConfig returns default Ethereum configuration
func DefaultEthereumConfig() EthereumConfig {
	return EthereumConfig{
		RPCURL:                "",
		PrivateKey:            "",
		ContractAddress:       "",
		ChainID:               1, // mainnet
		GasLimit:              0, // auto-estimate
		MaxGasPrice:           0, // no limit
		ConfirmationMode:      "safe",
		ConfirmationTimeout:   15 * time.Minute,
		MaxConfirmationBlocks: 100,
	}
}
