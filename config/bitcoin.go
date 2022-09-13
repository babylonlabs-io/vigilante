package config

import "errors"

// BTCConfig defines configuration for the Bitcoin client
type BTCConfig struct {
	DisableClientTLS  bool   `mapstructure:"no-client-tls"`
	CAFile            string `mapstructure:"ca-file"`
	Endpoint          string `mapstructure:"endpoint"`
	WalletEndpoint    string `mapstructure:"wallet-endpoint"`
	WalletPassword    string `mapstructure:"wallet-password"`
	WalletName        string `mapstructure:"wallet-name"`
	WalletCAFile      string `mapstructure:"wallet-ca-file"`
	NetParams         string `mapstructure:"net-params"`
	Username          string `mapstructure:"username"`
	Password          string `mapstructure:"password"`
	ReconnectAttempts int    `mapstructure:"reconnect-attempts"`
}

func (cfg *BTCConfig) Validate() error {
	if cfg.ReconnectAttempts < 0 {
		return errors.New("reconnectAttempts must be positive")
	}
	return nil
}

func DefaultBTCConfig() BTCConfig {
	return BTCConfig{
		DisableClientTLS:  false,
		CAFile:            defaultBtcCAFile,
		Endpoint:          "localhost:18556",
		WalletEndpoint:    "localhost:18554",
		WalletPassword:    "walletpass",
		WalletName:        "default",
		WalletCAFile:      defaultBtcWalletCAFile,
		NetParams:         "simnet",
		Username:          "rpcuser",
		Password:          "rpcpass",
		ReconnectAttempts: 3,
	}
}
