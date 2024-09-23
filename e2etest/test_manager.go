package e2etest

import (
	"bytes"
	"context"
	sdkErr "cosmossdk.io/errors"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/avast/retry-go/v4"
	txformat "github.com/babylonlabs-io/babylon/btctxformatter"
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	btcctypes "github.com/babylonlabs-io/babylon/x/btccheckpoint/types"
	ckpttypes "github.com/babylonlabs-io/babylon/x/checkpointing/types"
	"github.com/babylonlabs-io/babylon/x/finality/types"
	"github.com/babylonlabs-io/vigilante/e2etest/container"
	"github.com/btcsuite/btcd/txscript"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	sdkquerytypes "github.com/cosmos/cosmos-sdk/types/query"
	"github.com/cosmos/relayer/v2/relayer/provider"
	pv "github.com/cosmos/relayer/v2/relayer/provider"
	"go.uber.org/zap"
	"math/rand"
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
	manager, err := container.NewManager()
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

func (tm *TestManager) WaitForFpPubRandTimestamped(t *testing.T, fpPk *btcec.PublicKey) {
	var lastCommittedHeight uint64
	var err error

	require.Eventually(t, func() bool {
		lastCommittedHeight, err = tm.GetLastCommittedHeight(fpPk)
		if err != nil {
			return false
		}
		return lastCommittedHeight > 0
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	t.Logf("public randomness is successfully committed, last committed height: %d", lastCommittedHeight)

	// wait until the last registered epoch is finalized
	currentEpoch, err := tm.BabylonClient.CurrentEpoch()
	require.NoError(t, err)

	tm.FinalizeUntilEpoch(t, currentEpoch.CurrentEpoch)

	res, err := tm.BabylonClient.LatestEpochFromStatus(ckpttypes.Finalized)
	require.NoError(t, err)
	t.Logf("last finalized epoch: %d", res.RawCheckpoint.EpochNum)

	t.Logf("public randomness is successfully timestamped, last finalized epoch: %d", currentEpoch)
}

// QueryLastCommittedPublicRand returns the last public randomness commitments
func (tm *TestManager) QueryLastCommittedPublicRand(fpPk *btcec.PublicKey, count uint64) (map[uint64]*types.PubRandCommitResponse, error) {
	fpBtcPk := bbn.NewBIP340PubKeyFromBTCPK(fpPk)

	pagination := &sdkquery.PageRequest{
		Limit:   count,
		Reverse: true,
	}

	res, err := tm.BabylonClient.QueryClient.ListPubRandCommit(fpBtcPk.MarshalHex(), pagination)
	if err != nil {
		return nil, fmt.Errorf("failed to query committed public randomness: %w", err)
	}

	return res.PubRandCommitMap, nil
}

func (tm *TestManager) lastCommittedPublicRandWithRetry(btcPk *btcec.PublicKey, count uint64) (map[uint64]*types.PubRandCommitResponse, error) {
	var response map[uint64]*types.PubRandCommitResponse
	if err := retry.Do(func() error {
		resp, err := tm.QueryLastCommittedPublicRand(btcPk, count)
		if err != nil {
			return err
		}
		response = resp
		return nil
	},
		retry.Attempts(15),
		retry.Delay(time.Millisecond*400),
		retry.LastErrorOnly(true)); err != nil {
		return nil, err
	}

	return response, nil
}

func (tm *TestManager) GetLastCommittedHeight(btcPk *btcec.PublicKey) (uint64, error) {
	pubRandCommitMap, err := tm.lastCommittedPublicRandWithRetry(btcPk, 1)
	if err != nil {
		return 0, err
	}

	// no committed randomness yet
	if len(pubRandCommitMap) == 0 {
		return 0, nil
	}

	if len(pubRandCommitMap) > 1 {
		return 0, fmt.Errorf("got more than one last committed public randomness")
	}
	var lastCommittedHeight uint64
	for startHeight, resp := range pubRandCommitMap {
		lastCommittedHeight = startHeight + resp.NumPubRand - 1
	}

	return lastCommittedHeight, nil
}

func (tm *TestManager) FinalizeUntilEpoch(t *testing.T, epoch uint64) {
	bbnClient := tm.BabylonClient

	// wait until the checkpoint of this epoch is sealed
	require.Eventually(t, func() bool {
		lastSealedCkpt, err := bbnClient.LatestEpochFromStatus(ckpttypes.Sealed)
		if err != nil {
			return false
		}
		return epoch <= lastSealedCkpt.RawCheckpoint.EpochNum
	}, eventuallyWaitTimeOut, 1*time.Second)

	t.Logf("start finalizing epochs till %d", epoch)
	// Random source for the generation of BTC data
	r := rand.New(rand.NewSource(time.Now().Unix()))

	// get all checkpoints of these epochs
	pagination := &sdkquerytypes.PageRequest{
		Key:   ckpttypes.CkptsObjectKey(0),
		Limit: epoch,
	}
	resp, err := bbnClient.RawCheckpoints(pagination)
	require.NoError(t, err)
	require.Equal(t, int(epoch), len(resp.RawCheckpoints))

	submitterAddr, err := sdk.AccAddressFromBech32(tm.BabylonClient.MustGetAddr())
	require.NoError(t, err)

	for _, checkpoint := range resp.RawCheckpoints {
		currentBtcTipResp, err := tm.BabylonClient.QueryClient.BTCHeaderChainTip()
		require.NoError(t, err)
		tipHeader, err := bbn.NewBTCHeaderBytesFromHex(currentBtcTipResp.Header.HeaderHex)
		require.NoError(t, err)

		rawCheckpoint, err := checkpoint.Ckpt.ToRawCheckpoint()
		require.NoError(t, err)

		btcCheckpoint, err := ckpttypes.FromRawCkptToBTCCkpt(rawCheckpoint, submitterAddr)
		require.NoError(t, err)

		babylonTagBytes, err := hex.DecodeString("01020304")
		require.NoError(t, err)

		p1, p2, err := txformat.EncodeCheckpointData(
			babylonTagBytes,
			txformat.CurrentVersion,
			btcCheckpoint,
		)
		require.NoError(t, err)

		tx1 := datagen.CreatOpReturnTransaction(r, p1)

		opReturn1 := datagen.CreateBlockWithTransaction(r, tipHeader.ToBlockHeader(), tx1)
		tx2 := datagen.CreatOpReturnTransaction(r, p2)
		opReturn2 := datagen.CreateBlockWithTransaction(r, opReturn1.HeaderBytes.ToBlockHeader(), tx2)

		// insert headers and proofs
		_, err = tm.InsertBtcBlockHeaders([]bbn.BTCHeaderBytes{
			opReturn1.HeaderBytes,
			opReturn2.HeaderBytes,
		})
		require.NoError(t, err)

		_, err = tm.InsertSpvProofs(submitterAddr.String(), []*btcctypes.BTCSpvProof{
			opReturn1.SpvProof,
			opReturn2.SpvProof,
		})
		require.NoError(t, err)

		// wait until this checkpoint is submitted
		require.Eventually(t, func() bool {
			ckpt, err := bbnClient.RawCheckpoint(checkpoint.Ckpt.EpochNum)
			if err != nil {
				return false
			}
			return ckpt.RawCheckpoint.Status == ckpttypes.Submitted
		}, eventuallyWaitTimeOut, eventuallyPollTime)
	}

	// insert w BTC headers
	tm.InsertWBTCHeaders(t, r)

	// wait until the checkpoint of this epoch is finalised
	require.Eventually(t, func() bool {
		lastFinalizedCkpt, err := bbnClient.LatestEpochFromStatus(ckpttypes.Finalized)
		if err != nil {
			t.Logf("failed to get last finalized epoch: %v", err)
			return false
		}
		return epoch <= lastFinalizedCkpt.RawCheckpoint.EpochNum
	}, eventuallyWaitTimeOut, 1*time.Second)

	t.Logf("epoch %d is finalised", epoch)
}

func (tm *TestManager) InsertBtcBlockHeaders(headers []bbn.BTCHeaderBytes) (*provider.RelayerTxResponse, error) {
	msg := &btclctypes.MsgInsertHeaders{
		Signer:  tm.MustGetBabylonSigner(),
		Headers: headers,
	}

	res, err := tm.BabylonClient.ReliablySendMsg(context.Background(), msg, []*sdkErr.Error{}, []*sdkErr.Error{})
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (tm *TestManager) InsertSpvProofs(submitter string, proofs []*btcctypes.BTCSpvProof) (*provider.RelayerTxResponse, error) {
	msg := &btcctypes.MsgInsertBTCSpvProof{
		Submitter: submitter,
		Proofs:    proofs,
	}

	res, err := tm.BabylonClient.ReliablySendMsg(context.Background(), msg, []*sdkErr.Error{}, []*sdkErr.Error{})
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (tm *TestManager) InsertWBTCHeaders(t *testing.T, r *rand.Rand) {
	ckptParamRes, err := tm.BabylonClient.QueryClient.BTCCheckpointParams()
	require.NoError(t, err)
	btcTipResp, err := tm.BabylonClient.QueryClient.BTCHeaderChainTip()
	require.NoError(t, err)
	tipHeader, err := bbn.NewBTCHeaderBytesFromHex(btcTipResp.Header.HeaderHex)
	require.NoError(t, err)
	kHeaders := datagen.NewBTCHeaderChainFromParentInfo(r, &btclctypes.BTCHeaderInfo{
		Header: &tipHeader,
		Hash:   tipHeader.Hash(),
		Height: btcTipResp.Header.Height,
		Work:   &btcTipResp.Header.Work,
	}, uint32(ckptParamRes.Params.CheckpointFinalizationTimeout))
	_, err = tm.InsertBtcBlockHeaders(kHeaders.ChainToBytes())
	require.NoError(t, err)
}

func (tm *TestManager) QueryBestBbnBlock() (uint64, error) {
	pagination := &sdkquery.PageRequest{
		Limit:   1,
		Reverse: true,
		Key:     nil,
	}

	res, err := tm.BabylonClient.QueryClient.ListBlocks(types.QueriedBlockStatus_ANY, pagination)
	if err != nil {
		return 0, fmt.Errorf("failed to query finalized blocks: %v", err)
	}

	if len(res.Blocks) == 0 {
		info, err := tm.BabylonClient.RPCClient.BlockchainInfo(context.Background(), 0, 0)
		if err != nil {
			return 0, fmt.Errorf("get blockchain info: %v", err)
		}

		return uint64(info.BlockMetas[0].Header.Height), nil
	}

	return res.Blocks[0].Height, nil
}
