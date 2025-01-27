package e2etest

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/babylonlabs-io/babylon/client/babylonclient"
	"github.com/babylonlabs-io/vigilante/e2etest/container"
	"github.com/btcsuite/btcd/txscript"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"testing"
	"time"

	bbnclient "github.com/babylonlabs-io/babylon/client/client"
	bbn "github.com/babylonlabs-io/babylon/types"
	btclctypes "github.com/babylonlabs-io/babylon/x/btclightclient/types"
	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

var (
	submitterAddrStr = "bbn1eppc73j56382wjn6nnq3quu5eye4pmm087xfdh" //nolint:unused
	babylonTag       = []byte{1, 2, 3, 4}                           //nolint:unused
	babylonTagHex    = hex.EncodeToString(babylonTag)               //nolint:unused

	eventuallyWaitTimeOut = 40 * time.Second
	eventuallyPollTime    = 1 * time.Second
	regtestParams         = &chaincfg.RegressionNetParams
	defaultEpochInterval  = uint(400) //nolint:unused
)

func defaultVigilanteConfig() *config.Config {
	defaultConfig := config.DefaultConfig()
	defaultConfig.BTC.NetParams = regtestParams.Name
	defaultConfig.BTC.Endpoint = "127.0.0.1:18443"
	// Config setting necessary to connect btcwallet daemon
	defaultConfig.BTC.WalletPassword = "pass"
	defaultConfig.BTC.Username = "user"
	defaultConfig.BTC.Password = "pass"
	defaultConfig.BTC.ZmqSeqEndpoint = config.DefaultZmqSeqEndpoint

	return defaultConfig
}

type TestManager struct {
	TestRpcClient   *rpcclient.Client
	BitcoindHandler *BitcoindTestHandler
	BabylonClient   *bbnclient.Client
	BTCClient       *btcclient.Client
	Config          *config.Config
	WalletPrivKey   *btcec.PrivateKey
	manger          *container.Manager
}

func initBTCClientWithSubscriber(t *testing.T, cfg *config.Config) *btcclient.Client {
	client, err := btcclient.NewWallet(cfg, zap.NewNop())
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
// NOTE: uses btc client with zmq
func StartManager(t *testing.T, numMatureOutputsInWallet uint32, epochInterval uint) *TestManager {
	manager, err := container.NewManager(t)
	require.NoError(t, err)

	btcHandler := NewBitcoindHandler(t, manager)
	bitcoind := btcHandler.Start(t)
	passphrase := "pass"
	_ = btcHandler.CreateWallet("default", passphrase)

	cfg := defaultVigilanteConfig()

	cfg.BTC.Endpoint = fmt.Sprintf("127.0.0.1:%s", bitcoind.GetPort("18443/tcp"))

	testRpcClient, err := rpcclient.New(&rpcclient.ConnConfig{
		Host:                 cfg.BTC.Endpoint,
		User:                 cfg.BTC.Username,
		Pass:                 cfg.BTC.Password,
		DisableTLS:           true,
		DisableConnectOnNew:  true,
		DisableAutoReconnect: false,
		HTTPPostMode:         true,
	}, nil)
	require.NoError(t, err)

	err = testRpcClient.WalletPassphrase(passphrase, 600)
	require.NoError(t, err)

	walletPrivKey, err := importPrivateKey(btcHandler)
	require.NoError(t, err)
	blocksResponse := btcHandler.GenerateBlocks(int(numMatureOutputsInWallet))

	btcClient := initBTCClientWithSubscriber(t, cfg)

	var buff bytes.Buffer
	err = regtestParams.GenesisBlock.Header.Serialize(&buff)
	require.NoError(t, err)
	baseHeaderHex := hex.EncodeToString(buff.Bytes())

	minerAddressDecoded, err := btcutil.DecodeAddress(blocksResponse.Address, regtestParams)
	require.NoError(t, err)

	pkScript, err := txscript.PayToAddrScript(minerAddressDecoded)
	require.NoError(t, err)

	// start Babylon node

	tmpDir, err := tempDir(t)
	require.NoError(t, err)

	babylond, err := manager.RunBabylondResource(t, tmpDir, baseHeaderHex, hex.EncodeToString(pkScript), epochInterval)
	require.NoError(t, err)

	// create Babylon client
	cfg.Babylon.KeyDirectory = filepath.Join(tmpDir, "node0", "babylond")
	cfg.Babylon.Key = "test-spending-key" // keyring to bbn node
	cfg.Babylon.GasAdjustment = 3.0

	// update port with the dynamically allocated one from docker
	cfg.Babylon.RPCAddr = fmt.Sprintf("http://localhost:%s", babylond.GetPort("26657/tcp"))
	cfg.Babylon.GRPCAddr = fmt.Sprintf("https://localhost:%s", babylond.GetPort("9090/tcp"))

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

	return &TestManager{
		TestRpcClient:   testRpcClient,
		BabylonClient:   babylonClient,
		BitcoindHandler: btcHandler,
		BTCClient:       btcClient,
		Config:          cfg,
		WalletPrivKey:   walletPrivKey,
		manger:          manager,
	}
}

func (tm *TestManager) Stop(t *testing.T) {
	if tm.BabylonClient.IsRunning() {
		err := tm.BabylonClient.Stop()
		require.NoError(t, err)
	}
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

func (tm *TestManager) InsertBTCHeadersToBabylon(headers []*wire.BlockHeader) (*babylonclient.RelayerTxResponse, error) {
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

func importPrivateKey(btcHandler *BitcoindTestHandler) (*btcec.PrivateKey, error) {
	privKey, err := btcec.NewPrivateKey()
	if err != nil {
		return nil, err
	}

	wif, err := btcutil.NewWIF(privKey, regtestParams, true)
	if err != nil {
		return nil, err
	}

	// "combo" allows us to import a key and handle multiple types of btc scripts with a single descriptor command.
	descriptor := fmt.Sprintf("combo(%s)", wif.String())

	// Create the JSON descriptor object.
	descJSON, err := json.Marshal([]map[string]interface{}{
		{
			"desc":      descriptor,
			"active":    true,
			"timestamp": "now", // tells Bitcoind to start scanning from the current blockchain height
			"label":     "test key",
		},
	})

	if err != nil {
		return nil, err
	}

	btcHandler.ImportDescriptors(string(descJSON))

	return privKey, nil
}

func tempDir(t *testing.T) (string, error) {
	tempPath, err := os.MkdirTemp(os.TempDir(), "babylon-test-*")
	if err != nil {
		return "", err
	}

	if err = os.Chmod(tempPath, 0777); err != nil {
		return "", err
	}

	t.Cleanup(func() {
		_ = os.RemoveAll(tempPath)
	})

	return tempPath, err
}
