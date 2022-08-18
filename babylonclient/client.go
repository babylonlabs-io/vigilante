package babylonclient

import (
	"github.com/babylonchain/vigilante/config"
	lensclient "github.com/strangelove-ventures/lens/client"
	"go.uber.org/zap"
)

type Client struct {
	*lensclient.ChainClient
	Cfg *config.BabylonConfig
}

func New(cfg *config.BabylonConfig) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// init Zap logger, which is required by ChainClient
	logger, _ := zap.NewProduction()
	defer logger.Sync()
	// create chainClient
	cc, err := lensclient.NewChainClient(
		logger,
		cfg.Unwrap(),
		cfg.Home,
		nil, // TODO: figure out this field
		nil, // TODO: figure out this field
	)
	if err != nil {
		return nil, err
	}

	// show addresses in the key ring
	// TODO: specify multiple addresses in config
	addrs, err := cc.ListAddresses()
	if err != nil {
		return nil, err
	}
	log.Debugf("All Babylon addresses: %v", addrs)

	// TODO: is context necessary here?
	// ctx := client.Context{}.
	// 	WithClient(cc.RPCClient).
	// 	WithInterfaceRegistry(cc.Codec.InterfaceRegistry).
	// 	WithChainID(cc.Config.ChainID).
	// 	WithCodec(cc.Codec.Marshaler)

	// wrap to our type
	client := &Client{cc, cfg}
	log.Infof("Successfully created the Babylon client")

	return client, nil
}

func (c *Client) Stop() {
	if c.RPCClient != nil && c.RPCClient.IsRunning() {
		<-c.RPCClient.Quit()
	}
}
