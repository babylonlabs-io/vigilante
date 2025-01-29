package btcclient

import (
	"fmt"
	notifier "github.com/lightningnetwork/lnd/chainntnfs"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/netparams"
)

type TxStatus int

const (
	TxNotFound TxStatus = iota
	TxInMemPool
	TxInChain
)
const (
	txNotFoundErrMsgBitcoind = "No such mempool or blockchain transaction"
)

// NewWallet creates a new BTC wallet
// used by vigilant submitter
// a wallet is essentially a BTC client
// that connects to the btcWallet daemon
func NewWallet(cfg *config.Config, parentLogger *zap.Logger) (*Client, error) {
	params, err := netparams.GetBTCParams(cfg.BTC.NetParams)
	if err != nil {
		return nil, err
	}
	wallet := &Client{}
	wallet.cfg = &cfg.BTC
	wallet.params = params
	wallet.logger = parentLogger.With(zap.String("module", "btcclient_wallet")).Sugar()
	wallet.retrySleepTime = cfg.Common.RetrySleepTime
	wallet.maxRetryTimes = cfg.Common.MaxRetryTimes
	wallet.maxRetrySleepTime = cfg.Common.MaxRetrySleepTime

	connCfg := &rpcclient.ConnConfig{
		// this will work with node loaded with multiple wallets
		Host:         rpcHostURL(cfg.BTC.Endpoint, cfg.BTC.WalletName),
		HTTPPostMode: true,
		User:         cfg.BTC.Username,
		Pass:         cfg.BTC.Password,
		DisableTLS:   true,
	}

	rpcClient, err := rpcclient.New(connCfg, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create rpc client to BTC: %w", err)
	}

	wallet.logger.Infof("Successfully connected to bitcoind")

	wallet.Client = rpcClient

	return wallet, nil
}

func (c *Client) GetWalletPass() string {
	return c.cfg.WalletPassword
}

func (c *Client) GetWalletLockTime() int64 {
	return c.cfg.WalletLockTime
}

func (c *Client) GetNetParams() *chaincfg.Params {
	net, err := netparams.GetBTCParams(c.cfg.NetParams)
	if err != nil {
		panic(fmt.Errorf("failed to get BTC network params: %w", err))
	}

	return net
}

func (c *Client) GetBTCConfig() *config.BTCConfig {
	return c.cfg
}

func (c *Client) ListUnspent() ([]btcjson.ListUnspentResult, error) {
	return c.Client.ListUnspent()
}

func (c *Client) SendRawTransaction(tx *wire.MsgTx, allowHighFees bool) (*chainhash.Hash, error) {
	return c.Client.SendRawTransaction(tx, allowHighFees)
}

func (c *Client) ListReceivedByAddress() ([]btcjson.ListReceivedByAddressResult, error) {
	return c.Client.ListReceivedByAddress()
}

func (c *Client) GetNewAddress(account string) (btcutil.Address, error) {
	return c.Client.GetNewAddress(account)
}

func (c *Client) WalletPassphrase(passphrase string, timeoutSecs int64) error {
	return c.Client.WalletPassphrase(passphrase, timeoutSecs)
}

// GetHighUTXOAndSum returns the UTXO that has the highest amount
func (c *Client) GetHighUTXOAndSum() (*btcjson.ListUnspentResult, float64, error) {
	utxos, err := c.ListUnspent()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list unspent UTXOs: %w", err)
	}
	if len(utxos) == 0 {
		return nil, 0, fmt.Errorf("lack of spendable transactions in the wallet")
	}

	highUTXO := utxos[0] // freshest UTXO
	sum := float64(0)
	for _, utxo := range utxos {
		if highUTXO.Amount < utxo.Amount {
			highUTXO = utxo
		}
		sum += utxo.Amount
	}

	return &highUTXO, sum, nil
}

func (c *Client) FundRawTransaction(tx *wire.MsgTx, opts btcjson.FundRawTransactionOpts, isWitness *bool) (*btcjson.FundRawTransactionResult, error) {
	return c.Client.FundRawTransaction(tx, opts, isWitness)
}

func (c *Client) SignRawTransactionWithWallet(tx *wire.MsgTx) (*wire.MsgTx, bool, error) {
	return c.Client.SignRawTransactionWithWallet(tx)
}

func (c *Client) GetRawTransaction(txHash *chainhash.Hash) (*btcutil.Tx, error) {
	return c.Client.GetRawTransaction(txHash)
}

func notifierStateToWalletState(state notifier.TxConfStatus) TxStatus {
	switch state {
	case notifier.TxNotFoundIndex:
		return TxNotFound
	case notifier.TxFoundMempool:
		return TxInMemPool
	case notifier.TxFoundIndex:
		return TxInChain
	case notifier.TxNotFoundManually:
		return TxNotFound
	case notifier.TxFoundManually:
		return TxInChain
	default:
		panic(fmt.Sprintf("unknown notifier state: %s", state))
	}
}

func (c *Client) getTxDetails(req notifier.ConfRequest, msg string) (*notifier.TxConfirmation, TxStatus, error) {
	res, state, err := notifier.ConfDetailsFromTxIndex(c.Client, req, msg)

	if err != nil {
		return nil, TxNotFound, err
	}

	return res, notifierStateToWalletState(state), nil
}

// TxDetails Fetch info about transaction from mempool or blockchain, requires node to have enabled  transaction index
func (c *Client) TxDetails(txHash *chainhash.Hash, pkScript []byte) (*notifier.TxConfirmation, TxStatus, error) {
	req, err := notifier.NewConfRequest(txHash, pkScript)

	if err != nil {
		return nil, TxNotFound, err
	}

	return c.getTxDetails(req, txNotFoundErrMsgBitcoind)
}

func rpcHostURL(host, walletName string) string {
	if len(walletName) > 0 {
		return host + "/wallet/" + walletName
	}

	return host
}
