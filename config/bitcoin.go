package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/lightningnetwork/lnd/lnwallet/chainfee"

	"github.com/babylonlabs-io/vigilante/types"
)

// BTCConfig defines configuration for the Bitcoin client
type BTCConfig struct {
	DisableClientTLS  bool                      `mapstructure:"no-client-tls"`
	CAFile            string                    `mapstructure:"ca-file"`
	Endpoint          string                    `mapstructure:"endpoint"`
	WalletEndpoint    string                    `mapstructure:"wallet-endpoint"`
	WalletPassword    string                    `mapstructure:"wallet-password"`
	WalletName        string                    `mapstructure:"wallet-name"`
	WalletCAFile      string                    `mapstructure:"wallet-ca-file"`
	WalletLockTime    int64                     `mapstructure:"wallet-lock-time"` // time duration in which the wallet remains unlocked, in seconds
	TxFeeMin          chainfee.SatPerKVByte     `mapstructure:"tx-fee-min"`       // minimum tx fee, sat/kvb
	TxFeeMax          chainfee.SatPerKVByte     `mapstructure:"tx-fee-max"`       // maximum tx fee, sat/kvb
	DefaultFee        chainfee.SatPerKVByte     `mapstructure:"default-fee"`      // default BTC tx fee in case estimation fails, sat/kvb
	EstimateMode      string                    `mapstructure:"estimate-mode"`    // the BTC tx fee estimate mode, which is only used by bitcoind, must be either ECONOMICAL or CONSERVATIVE
	TargetBlockNum    int64                     `mapstructure:"target-block-num"` // this implies how soon the tx is estimated to be included in a block, e.g., 1 means the tx is estimated to be included in the next block
	NetParams         string                    `mapstructure:"net-params"`
	Username          string                    `mapstructure:"username"`
	Password          string                    `mapstructure:"password"`
	ReconnectAttempts int                       `mapstructure:"reconnect-attempts"`
	BtcBackend        types.SupportedBtcBackend `mapstructure:"btc-backend"`
	ZmqSeqEndpoint    string                    `mapstructure:"zmq-seq-endpoint"`
	ZmqBlockEndpoint  string                    `mapstructure:"zmq-block-endpoint"`
	ZmqTxEndpoint     string                    `mapstructure:"zmq-tx-endpoint"`
}

func (cfg *BTCConfig) Validate() error {
	if cfg.ReconnectAttempts < 0 {
		return errors.New("reconnect-attempts must be non-negative")
	}

	if _, ok := types.GetValidNetParams()[cfg.NetParams]; !ok {
		return errors.New("invalid net params")
	}

	if _, ok := types.GetValidBtcBackends()[cfg.BtcBackend]; !ok {
		return errors.New("invalid btc backend")
	}

	if cfg.BtcBackend == types.Bitcoind {
		// TODO: implement regex validation for zmq endpoint
		if cfg.ZmqBlockEndpoint == "" {
			return errors.New("zmq block endpoint cannot be empty")
		}

		if cfg.ZmqTxEndpoint == "" {
			return errors.New("zmq tx endpoint cannot be empty")
		}

		if cfg.ZmqSeqEndpoint == "" {
			return errors.New("zmq seq endpoint cannot be empty")
		}

		if cfg.EstimateMode != "ECONOMICAL" && cfg.EstimateMode != "CONSERVATIVE" {
			return errors.New("estimate-mode must be either ECONOMICAL or CONSERVATIVE when the backend is bitcoind")
		}
	}

	if cfg.TargetBlockNum <= 0 {
		return errors.New("target-block-num should be positive")
	}

	if cfg.TxFeeMax <= 0 {
		return errors.New("tx-fee-max must be positive")
	}

	if cfg.TxFeeMin <= 0 {
		return errors.New("tx-fee-min must be positive")
	}

	if cfg.TxFeeMin > cfg.TxFeeMax {
		return errors.New("tx-fee-min is larger than tx-fee-max")
	}

	if cfg.DefaultFee <= 0 {
		return errors.New("default-fee must be positive")
	}

	if cfg.DefaultFee < cfg.TxFeeMin || cfg.DefaultFee > cfg.TxFeeMax {
		return fmt.Errorf("default-fee should be in the range of [%v, %v]", cfg.TxFeeMin, cfg.TxFeeMax)
	}

	return nil
}

const (
	// Config for polling jittner in bitcoind client, with polling enabled
	DefaultTxPollingJitter     = 0.5
	DefaultRpcBtcNodeHost      = "127.0.01:18556"
	DefaultBtcNodeRpcUser      = "rpcuser"
	DefaultBtcNodeRpcPass      = "rpcpass"
	DefaultBtcNodeEstimateMode = "CONSERVATIVE"
	DefaultBtcblockCacheSize   = 20 * 1024 * 1024 // 20 MB
	DefaultZmqSeqEndpoint      = "tcp://127.0.0.1:28333"
	DefaultZmqBlockEndpoint    = "tcp://127.0.0.1:29001"
	DefaultZmqTxEndpoint       = "tcp://127.0.0.1:29002"
)

func DefaultBTCConfig() BTCConfig {
	return BTCConfig{
		DisableClientTLS:  false,
		CAFile:            defaultBtcCAFile,
		Endpoint:          DefaultRpcBtcNodeHost,
		WalletEndpoint:    "localhost:18554",
		WalletPassword:    "walletpass",
		WalletName:        "default",
		WalletCAFile:      defaultBtcWalletCAFile,
		WalletLockTime:    10,
		BtcBackend:        types.Btcd,
		TxFeeMax:          chainfee.SatPerKVByte(20 * 1000), // 20,000sat/kvb = 20sat/vbyte
		TxFeeMin:          chainfee.SatPerKVByte(1 * 1000),  // 1,000sat/kvb = 1sat/vbyte
		DefaultFee:        chainfee.SatPerKVByte(1 * 1000),  // 1,000sat/kvb = 1sat/vbyte
		EstimateMode:      DefaultBtcNodeEstimateMode,
		TargetBlockNum:    1,
		NetParams:         types.BtcSimnet.String(),
		Username:          DefaultBtcNodeRpcUser,
		Password:          DefaultBtcNodeRpcPass,
		ReconnectAttempts: 3,
		ZmqSeqEndpoint:    DefaultZmqSeqEndpoint,
		ZmqBlockEndpoint:  DefaultZmqBlockEndpoint,
		ZmqTxEndpoint:     DefaultZmqTxEndpoint,
	}
}

func (cfg *BTCConfig) ReadCAFile() []byte {
	if cfg.DisableClientTLS {
		return nil
	}

	// Read certificate file if TLS is not disabled.
	certs, err := os.ReadFile(cfg.CAFile)
	if err != nil {
		// If there's an error reading the CA file, continue
		// with nil certs and without the client connection.
		return nil
	}

	return certs
}

func (cfg *BTCConfig) ReadWalletCAFile() []byte {
	if cfg.DisableClientTLS {
		// Chain server RPC TLS is disabled
		return nil
	}

	// Read certificate file if TLS is not disabled.
	certs, err := os.ReadFile(cfg.WalletCAFile)
	if err != nil {
		// If there's an error reading the CA file, continue
		// with nil certs and without the client connection.
		return nil
	}
	return certs
}
