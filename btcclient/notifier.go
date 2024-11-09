package btcclient

import (
	"fmt"
	"github.com/babylonlabs-io/vigilante/netparams"
	"net"
	"time"

	"github.com/babylonlabs-io/vigilante/config"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/lightningnetwork/lnd/blockcache"
	"github.com/lightningnetwork/lnd/chainntnfs"
	"github.com/lightningnetwork/lnd/chainntnfs/bitcoindnotify"
)

type Bitcoind struct {
	RPCHost              string
	RPCUser              string
	RPCPass              string
	ZMQPubRawBlock       string
	ZMQPubRawTx          string
	ZMQReadDeadline      time.Duration
	EstimateMode         string
	PrunedNodeMaxPeers   int
	RPCPolling           bool
	BlockPollingInterval time.Duration
	TxPollingInterval    time.Duration
	BlockCacheSize       uint64
}

func DefaultBitcoindConfig() Bitcoind {
	return Bitcoind{
		RPCHost:              config.DefaultRPCBtcNodeHost,
		RPCUser:              config.DefaultBtcNodeRPCUser,
		RPCPass:              config.DefaultBtcNodeRPCPass,
		RPCPolling:           true,
		BlockPollingInterval: 30 * time.Second,
		TxPollingInterval:    30 * time.Second,
		EstimateMode:         config.DefaultBtcNodeEstimateMode,
		BlockCacheSize:       config.DefaultBtcBlockCacheSize,
		ZMQPubRawBlock:       config.DefaultZmqBlockEndpoint,
		ZMQPubRawTx:          config.DefaultZmqTxEndpoint,
		ZMQReadDeadline:      30 * time.Second,
	}
}

func ToBitcoindConfig(cfg config.BTCConfig) *Bitcoind {
	defaultBitcoindCfg := DefaultBitcoindConfig()
	// Re-rewrite defaults by values from global cfg
	defaultBitcoindCfg.RPCHost = cfg.Endpoint
	defaultBitcoindCfg.RPCUser = cfg.Username
	defaultBitcoindCfg.RPCPass = cfg.Password
	defaultBitcoindCfg.ZMQPubRawBlock = cfg.ZmqBlockEndpoint
	defaultBitcoindCfg.ZMQPubRawTx = cfg.ZmqTxEndpoint
	defaultBitcoindCfg.EstimateMode = cfg.EstimateMode

	return &defaultBitcoindCfg
}

type NodeBackend struct {
	chainntnfs.ChainNotifier
}

type HintCache interface {
	chainntnfs.SpendHintCache
	chainntnfs.ConfirmHintCache
}

type EmptyHintCache struct{}

var _ HintCache = (*EmptyHintCache)(nil)

func (c *EmptyHintCache) CommitSpendHint(_ uint32, _ ...chainntnfs.SpendRequest) error {
	return nil
}
func (c *EmptyHintCache) QuerySpendHint(_ chainntnfs.SpendRequest) (uint32, error) {
	return 0, nil
}
func (c *EmptyHintCache) PurgeSpendHint(_ ...chainntnfs.SpendRequest) error {
	return nil
}

func (c *EmptyHintCache) CommitConfirmHint(_ uint32, _ ...chainntnfs.ConfRequest) error {
	return nil
}
func (c *EmptyHintCache) QueryConfirmHint(_ chainntnfs.ConfRequest) (uint32, error) {
	return 0, nil
}
func (c *EmptyHintCache) PurgeConfirmHint(_ ...chainntnfs.ConfRequest) error {
	return nil
}

// // TODO  This should be moved to a more appropriate place, most probably to config
// // and be connected to validation of rpc host/port.
// // According to chain.BitcoindConfig docs it should also support tor if node backend
// // works over tor.
func BuildDialer(rpcHost string) func(string) (net.Conn, error) {
	return func(_ string) (net.Conn, error) {
		return net.Dial("tcp", rpcHost)
	}
}

func NewNodeBackend(
	cfg *Bitcoind,
	params *chaincfg.Params,
	hintCache HintCache,
) (*NodeBackend, error) {
	bitcoindCfg := &chain.BitcoindConfig{
		ChainParams:        params,
		Host:               cfg.RPCHost,
		User:               cfg.RPCUser,
		Pass:               cfg.RPCPass,
		Dialer:             BuildDialer(cfg.RPCHost),
		PrunedModeMaxPeers: cfg.PrunedNodeMaxPeers,
	}

	if cfg.RPCPolling {
		bitcoindCfg.PollingConfig = &chain.PollingConfig{
			BlockPollingInterval:    cfg.BlockPollingInterval,
			TxPollingInterval:       cfg.TxPollingInterval,
			TxPollingIntervalJitter: config.DefaultTxPollingJitter,
		}
	} else {
		bitcoindCfg.ZMQConfig = &chain.ZMQConfig{
			ZMQBlockHost:           cfg.ZMQPubRawBlock,
			ZMQTxHost:              cfg.ZMQPubRawTx,
			ZMQReadDeadline:        cfg.ZMQReadDeadline,
			MempoolPollingInterval: cfg.TxPollingInterval,
			PollingIntervalJitter:  config.DefaultTxPollingJitter,
		}
	}

	bitcoindConn, err := chain.NewBitcoindConn(bitcoindCfg)
	if err != nil {
		return nil, err
	}

	if err := bitcoindConn.Start(); err != nil {
		return nil, fmt.Errorf("unable to connect to "+
			"bitcoind: %v", err)
	}

	chainNotifier := bitcoindnotify.New(
		bitcoindConn, params, hintCache,
		hintCache, blockcache.NewBlockCache(cfg.BlockCacheSize),
	)

	return &NodeBackend{
		ChainNotifier: chainNotifier,
	}, nil
}

// NewNodeBackendWithParams creates a new NodeBackend by incorporating parameter retrieval and config conversion.
func NewNodeBackendWithParams(cfg config.BTCConfig) (*NodeBackend, error) {
	btcParams, err := netparams.GetBTCParams(cfg.NetParams)
	if err != nil {
		return nil, fmt.Errorf("failed to get BTC net params: %w", err)
	}

	btcNotifier, err := NewNodeBackend(ToBitcoindConfig(cfg), btcParams, &EmptyHintCache{})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize notifier: %w", err)
	}

	return btcNotifier, nil
}
