package config

// GRPCWebConfig defines configuration for the gRPC-web server.
type GRPCWebConfig struct {
	Placeholder string `mapstructure:"placeholder"`
}

func (cfg *GRPCWebConfig) Validate() error {
	return nil
}

func DefaultGRPCWebConfig() GRPCWebConfig {
	return GRPCWebConfig{
		Placeholder: "grpcwebconfig",
	}
}
