package config

// GRPCConfig defines configuration for the gRPC server.
type GRPCConfig struct {
	OneTimeTLSKey bool     `mapstructure:"onetime-tls-key"`
	RPCKeyFile    string   `mapstructure:"rpc-key"`
	RPCCertFile   string   `mapstructure:"rpc-cert"`
	Endpoints     []string `mapstructure:"endpoints"`
}

func (cfg *GRPCConfig) Validate() error {
	return nil
}

func DefaultGRPCConfig() GRPCConfig {
	return GRPCConfig{
		OneTimeTLSKey: true,
		RPCKeyFile:    defaultRPCKeyFile,
		RPCCertFile:   defaultRPCCertFile,
		Endpoints:     []string{"localhost:8080"},
	}
}
