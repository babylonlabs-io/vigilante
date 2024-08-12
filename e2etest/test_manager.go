package e2etest

import (
	"bytes"
	"context"
	"encoding/hex"
	"testing"
	"time"

	pv "github.com/cosmos/relayer/v2/relayer/provider"

	bbnclient "github.com/babylonlabs-io/babylon/client/client"
	bbn "github.com/babylonlabs-io/babylon/types"
	btclctypes "github.com/babylonlabs-io/babylon/x/btclightclient/types"
	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// bticoin params used for testing
var (
	netParams        = &chaincfg.SimNetParams
	submitterAddrStr = "bbn1eppc73j56382wjn6nnq3quu5eye4pmm087xfdh" //nolint:unused
	babylonTag       = []byte{1, 2, 3, 4}                           //nolint:unused
	babylonTagHex    = hex.EncodeToString(babylonTag)               //nolint:unused

	eventuallyWaitTimeOut = 40 * time.Second
	eventuallyPollTime    = 1 * time.Second
	regtestParams         = &chaincfg.RegressionNetParams
)

// keyToAddr maps the passed private to corresponding p2pkh address.
func keyToAddr(key *btcec.PrivateKey, net *chaincfg.Params) (btcutil.Address, error) {
	serializedKey := key.PubKey().SerializeCompressed()
	pubKeyAddr, err := btcutil.NewAddressPubKey(serializedKey, net)
	if err != nil {
		return nil, err
	}
	return pubKeyAddr.AddressPubKeyHash(), nil
}

func defaultVigilanteConfig() *config.Config {
	defaultConfig := config.DefaultConfig()
	// Config setting necessary to connect btcd daemon
	defaultConfig.BTC.NetParams = "regtest"
	defaultConfig.BTC.Endpoint = "127.0.0.1:18443"
	// Config setting necessary to connect btcwallet daemon
	defaultConfig.BTC.BtcBackend = types.Bitcoind
	defaultConfig.BTC.WalletEndpoint = "127.0.0.1:18554"
	defaultConfig.BTC.WalletPassword = "pass"
	defaultConfig.BTC.Username = "user"
	defaultConfig.BTC.Password = "pass"
	defaultConfig.BTC.DisableClientTLS = true
	return defaultConfig
}

type TestManager struct {
	TestRpcClient    *rpcclient.Client
	BitcoindHandler  *BitcoindTestHandler
	BtcWalletHandler *WalletHandler
	BabylonHandler   *BabylonNodeHandler
	BabylonClient    *bbnclient.Client
	BTCClient        *btcclient.Client
	BTCWalletClient  *btcclient.Client // todo probably not needed
	Config           *config.Config
	WalletPrivKey    *btcec.PrivateKey
}

func initBTCWalletClient(
	t *testing.T,
	cfg *config.Config) *btcclient.Client {

	var client *btcclient.Client

	require.Eventually(t, func() bool {
		btcWallet, err := btcclient.NewWallet(&cfg.BTC, logger)
		if err != nil {
			return false
		}

		client = btcWallet
		return true

	}, eventuallyWaitTimeOut, eventuallyPollTime)

	// let's wait until chain rpc becomes available
	// poll time is increase here to avoid spamming the btcwallet rpc server
	require.Eventually(t, func() bool {
		if _, _, err := client.GetBestBlock(); err != nil {
			return false
		}
		return true
	}, eventuallyWaitTimeOut, 1*time.Second)

	//waitForNOutputs(t, client, outputsToWaitFor)

	return client
}

func initBTCClientWithSubscriber(t *testing.T, cfg *config.Config) *btcclient.Client {
	btcCfg := &config.BTCConfig{
		NetParams:        cfg.BTC.NetParams,
		Username:         cfg.BTC.Username,
		Password:         cfg.BTC.Password,
		Endpoint:         cfg.BTC.Endpoint,
		DisableClientTLS: cfg.BTC.DisableClientTLS,
		BtcBackend:       types.Bitcoind,
		ZmqSeqEndpoint:   config.DefaultZmqSeqEndpoint,
	}
	rootLogger, err := cfg.CreateLogger()
	require.NoError(t, err)

	client, err := btcclient.NewWithBlockSubscriber(btcCfg, cfg.Common.RetrySleepTime, cfg.Common.MaxRetrySleepTime, rootLogger)
	require.NoError(t, err)

	// let's wait until chain rpc becomes available
	// poll time is increase here to avoid spamming the rpc server
	require.Eventually(t, func() bool {
		if _, err := client.GetBlockCount(); err != nil {
			log.Errorf("failed to get best block: %v", err)
			return false
		}

		return true
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	return client
}

// StartManager creates a test manager
// NOTE: if handlers.OnFilteredBlockConnected, handlers.OnFilteredBlockDisconnected
// and blockEventChan are all not nil, then the test manager will create a BTC
// client with a WebSocket subscriber
func StartManager(t *testing.T, numMatureOutputsInWallet uint32) *TestManager {
	btcHandler := NewBitcoindHandler(t)
	btcHandler.Start()
	passphrase := "pass"
	_ = btcHandler.CreateWallet("default", passphrase)
	blocksResponse := btcHandler.GenerateBlocks(int(numMatureOutputsInWallet))

	cfg := defaultVigilanteConfig()

	testRpcClient, err := rpcclient.New(&rpcclient.ConnConfig{
		Host:                 cfg.BTC.Endpoint,
		User:                 cfg.BTC.Username,
		Pass:                 cfg.BTC.Password,
		DisableTLS:           true,
		DisableConnectOnNew:  true,
		DisableAutoReconnect: false,
		// we use post mode as it sure it works with either bitcoind or btcwallet
		// we may need to re-consider it later if we need any notifications
		HTTPPostMode: true,
	}, nil)

	btcClient := initBTCClientWithSubscriber(t, cfg)

	// we always want BTC wallet client for sending txs
	btcWalletClient := initBTCWalletClient(t, cfg)

	var buff bytes.Buffer
	err = regtestParams.GenesisBlock.Header.Serialize(&buff)
	require.NoError(t, err)
	baseHeaderHex := hex.EncodeToString(buff.Bytes())

	// start Babylon node
	bh, err := NewBabylonNodeHandler(baseHeaderHex)
	require.NoError(t, err)
	err = bh.Start()
	require.NoError(t, err)

	// create Babylon client
	cfg.Babylon.KeyDirectory = bh.GetNodeDataDir()
	cfg.Babylon.Key = "test-spending-key" // keyring to bbn node
	cfg.Babylon.GasAdjustment = 3.0

	babylonClient, err := bbnclient.New(&cfg.Babylon, nil)
	require.NoError(t, err)

	// wait until Babylon is ready
	require.Eventually(t, func() bool {
		resp, err := babylonClient.CurrentEpoch()
		if err != nil {
			return false
		}
		log.Infof("Babylon is ready: %v", resp)
		return true
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	err = testRpcClient.WalletPassphrase(passphrase, 600)
	require.NoError(t, err)

	minerAddressDecoded, err := btcutil.DecodeAddress(blocksResponse.Address, regtestParams)
	require.NoError(t, err)

	walletPrivKey, err := testRpcClient.DumpPrivKey(minerAddressDecoded)
	require.NoError(t, err)

	return &TestManager{
		TestRpcClient:   testRpcClient,
		BabylonHandler:  bh,
		BabylonClient:   babylonClient,
		BitcoindHandler: btcHandler,
		BTCClient:       btcClient,
		BTCWalletClient: btcWalletClient,
		Config:          cfg,
		WalletPrivKey:   walletPrivKey.PrivKey,
	}
}

func (tm *TestManager) Stop(t *testing.T) {
	err := tm.BabylonHandler.Stop()
	require.NoError(t, err)

	if tm.BabylonClient.IsRunning() {
		err := tm.BabylonClient.Stop()
		require.NoError(t, err)
	}
}

// MineBlockWithTxs mines a single block to include the specifies
// transactions only. // todo Lazar, bitcoind doens't have this, check if we need txs
func (tm *TestManager) MineBlockWithTxs(t *testing.T, txs []*btcutil.Tx) *wire.MsgBlock {
	resp := tm.BitcoindHandler.GenerateBlocks(1)

	hash, err := chainhash.NewHashFromStr(resp.Blocks[0])
	require.NoError(t, err)

	header, err := tm.TestRpcClient.GetBlock(hash)
	require.NoError(t, err)

	return header
}

// mineBlock mines a single block
func (tm *TestManager) mineBlock(t *testing.T) *wire.MsgBlock {
	resp := tm.BitcoindHandler.GenerateBlocks(1)

	hash, err := chainhash.NewHashFromStr(resp.Blocks[0])
	require.NoError(t, err)

	header, err := tm.TestRpcClient.GetBlock(hash)
	require.NoError(t, err)

	return header
}

func (tm *TestManager) MustGetBabylonSigner() string {
	return tm.BabylonClient.MustGetAddr()
}

// RetrieveTransactionFromMempool fetches transactions from the mempool for the given hashes
func (tm *TestManager) RetrieveTransactionFromMempool(t *testing.T, hashes []*chainhash.Hash) []*btcutil.Tx {
	var txs []*btcutil.Tx
	for _, txHash := range hashes {
		tx, err := tm.BTCClient.GetRawTransaction(txHash)
		require.NoError(t, err)
		txs = append(txs, tx)
	}

	return txs
}

func (tm *TestManager) InsertBTCHeadersToBabylon(headers []*wire.BlockHeader) (*pv.RelayerTxResponse, error) {
	var headersBytes []bbn.BTCHeaderBytes

	for _, h := range headers {
		headersBytes = append(headersBytes, bbn.NewBTCHeaderBytesFromBlockHeader(h))
	}

	msg := btclctypes.MsgInsertHeaders{
		Headers: headersBytes,
		Signer:  tm.MustGetBabylonSigner(),
	}

	return tm.BabylonClient.InsertHeaders(context.Background(), &msg)
}

func (tm *TestManager) CatchUpBTCLightClient(t *testing.T) {
	btcHeight, err := tm.TestRpcClient.GetBlockCount()
	require.NoError(t, err)

	tipResp, err := tm.BabylonClient.BTCHeaderChainTip()
	require.NoError(t, err)
	btclcHeight := tipResp.Header.Height

	var headers []*wire.BlockHeader
	for i := int(btclcHeight + 1); i <= int(btcHeight); i++ {
		hash, err := tm.TestRpcClient.GetBlockHash(int64(i))
		require.NoError(t, err)
		header, err := tm.TestRpcClient.GetBlockHeader(hash)
		require.NoError(t, err)
		headers = append(headers, header)
	}

	_, err = tm.InsertBTCHeadersToBabylon(headers)
	require.NoError(t, err)
}

func waitForNOutputs(t *testing.T, walletClient *btcclient.Client, n int) {
	require.Eventually(t, func() bool {
		outputs, err := walletClient.ListUnspent()

		if err != nil {
			return false
		}

		return len(outputs) >= n
	}, eventuallyWaitTimeOut, eventuallyPollTime)
}
