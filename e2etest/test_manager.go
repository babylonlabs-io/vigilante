package e2etest

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdkquerytypes "github.com/cosmos/cosmos-sdk/types/query"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/babylonlabs-io/babylon/v4/client/babylonclient"
	"github.com/babylonlabs-io/vigilante/e2etest/container"
	"github.com/btcsuite/btcd/txscript"
	"github.com/ory/dockertest/v3"
	"go.uber.org/zap"

	bbnclient "github.com/babylonlabs-io/babylon/v4/client/client"
	bbn "github.com/babylonlabs-io/babylon/v4/types"
	btclctypes "github.com/babylonlabs-io/babylon/v4/x/btclightclient/types"
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

type TestManagerOption func(*TestManagerConfig)
type TestManagerConfig struct {
	NumMatureOutputsInWallet uint32
	EpochInterval            uint
	NumCovenants             uint
}

func defaultTestManagerConfig() *TestManagerConfig {
	return &TestManagerConfig{
		NumMatureOutputsInWallet: 300,
		EpochInterval:            defaultEpochInterval,
		NumCovenants:             1,
	}
}

func WithNumMatureOutputs(num uint32) TestManagerOption {
	return func(config *TestManagerConfig) {
		config.NumMatureOutputsInWallet = num
	}
}

func WithEpochInterval(interval uint) TestManagerOption {
	return func(config *TestManagerConfig) {
		config.EpochInterval = interval
	}
}

func WithNumCovenants(numCovenants uint) TestManagerOption {
	return func(config *TestManagerConfig) {
		config.NumCovenants = numCovenants
	}
}

type TestManager struct {
	TestRpcClient    *rpcclient.Client
	BitcoindHandler  *BitcoindTestHandler
	Electrs          *ElectrsTestHandler
	BabylonClient    *bbnclient.Client
	BTCClient        *btcclient.Client
	Config           *config.Config
	WalletPrivKey    *btcec.PrivateKey
	manger           *container.Manager
	CovenantPrivKeys []*btcec.PrivateKey
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
func StartManager(t *testing.T, options ...TestManagerOption) *TestManager {
	manager, err := container.NewManager(t)
	require.NoError(t, err)

	tmCfg := defaultTestManagerConfig()
	for _, option := range options {
		option(tmCfg)
	}

	btcHandler := NewBitcoindHandler(t, manager)
	var bitcoind *dockertest.Resource
	var bitcoindPath string
	require.Eventually(t, func() bool {
		bitcoind, bitcoindPath, err = btcHandler.Start(t)
		if err != nil {
			t.Logf("failed to start bitcoind: %v", err)
			errResource := btcHandler.Remove(fmt.Sprintf("bitcoind-%s", t.Name()))
			require.NoError(t, errResource)
		}
		return err == nil
	}, 25*time.Second, 500*time.Millisecond)

	passphrase := "pass"
	_ = btcHandler.CreateWallet("default", passphrase)

	internalBtcRpc := fmt.Sprintf("%s:18443", bitcoind.Container.NetworkSettings.IPAddress)
	electrsHandler := NewElectrsHandler(t, manager)
	var electrs *dockertest.Resource
	require.Eventually(t, func() bool {
		electrs, err = electrsHandler.Start(t, bitcoindPath, internalBtcRpc)
		if err != nil {
			t.Logf("failed to start electrs: %v", err)
			errResource := electrsHandler.Remove(fmt.Sprintf("electrs-%s", t.Name()))
			require.NoError(t, errResource)
		}
		return err == nil
	}, 25*time.Second, 500*time.Millisecond)

	cfg := defaultVigilanteConfig()
	cfg.BTCStakingTracker.IndexerAddr = fmt.Sprintf("http://localhost:%s", electrs.GetPort("3000/tcp"))
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

	err = testRpcClient.WalletPassphrase(passphrase, 60000)
	require.NoError(t, err)

	walletPrivKey, err := importPrivateKey(btcHandler)
	require.NoError(t, err)
	blocksResponse := btcHandler.GenerateBlocks(int(tmCfg.NumMatureOutputsInWallet))

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

	covenants := generateCovenants(t, tmCfg.NumCovenants)
	covPubKeys := make([]*btcec.PublicKey, len(covenants))
	for i, pk := range covenants {
		covPubKeys[i] = pk.PubKey()
	}

	var babylond *dockertest.Resource
	require.Eventually(t, func() bool {
		babylond, err = manager.RunBabylondResource(
			t, tmpDir, baseHeaderHex, hex.EncodeToString(pkScript), tmCfg.EpochInterval, covPubKeys...)
		if err != nil {
			t.Logf("failed to start babylond, test: %s err: %v", t.Name(), err)
			errResource := manager.RemoveContainer(fmt.Sprintf("babylond-%s", t.Name()))
			require.NoError(t, errResource)
		}
		return err == nil
	}, 25*time.Second, 500*time.Millisecond)

	// create Babylon client
	cfg.Babylon.KeyDirectory = filepath.Join(tmpDir, "node0", "babylond")
	cfg.Babylon.Key = "test-spending-key" // keyring to bbn node
	cfg.Babylon.GasAdjustment = 3.0
	cfg.Babylon.BlockTimeout = 30 * time.Second

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
		TestRpcClient:    testRpcClient,
		BabylonClient:    babylonClient,
		BitcoindHandler:  btcHandler,
		Electrs:          electrsHandler,
		BTCClient:        btcClient,
		Config:           cfg,
		WalletPrivKey:    walletPrivKey,
		manger:           manager,
		CovenantPrivKeys: covenants,
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
func (tm *TestManager) RetrieveTransactionFromMempool(t *testing.T, hashes []*chainhash.Hash) ([]*btcutil.Tx, error) {
	var txs []*btcutil.Tx
	for _, txHash := range hashes {
		tx, err := tm.BTCClient.GetRawTransaction(txHash)
		if err != nil {
			return nil, err
		}
		txs = append(txs, tx)
	}

	return txs, nil
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

	for headersChunk := range slices.Chunk(headers, 100) {
		_, err := tm.InsertBTCHeadersToBabylon(headersChunk)
		require.NoError(t, err)

	}
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

func (tm *TestManager) DeployCwContract(t *testing.T) string {
	err := StoreWasmCode(t.Context(), tm.BabylonClient, "./bytecode/testdata.wasm")
	require.NoError(t, err)

	var codeId uint64
	require.Eventually(t, func() bool {
		codeId, _ = GetLatestCodeID(t.Context(), tm.BabylonClient)
		return codeId > 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	require.Equal(t, uint64(1), codeId, "first deployed contract code_id should be 1")
	initMsgBz := []byte("{}")

	err = InstantiateContract(tm.BabylonClient, t.Context(), codeId, initMsgBz)
	require.NoError(t, err)

	var listContractsResponse *wasmtypes.QueryContractsByCodeResponse
	require.Eventually(t, func() bool {
		listContractsResponse, err = ListContractsByCode(
			t.Context(),
			tm.BabylonClient,
			codeId,
			&sdkquerytypes.PageRequest{},
		)
		return err == nil
	}, eventuallyWaitTimeOut, eventuallyPollTime)
	require.Len(t, listContractsResponse.Contracts, 1)
	address := listContractsResponse.Contracts[0]

	return address
}

func generateCovenants(t *testing.T, num uint) []*btcec.PrivateKey {
	covs := make([]*btcec.PrivateKey, 0, num)
	for i := 0; i < int(num); i++ {
		covenantPrivKey, err := btcec.NewPrivateKey()
		require.NoError(t, err)
		covs = append(covs, covenantPrivKey)
	}

	return covs
}
