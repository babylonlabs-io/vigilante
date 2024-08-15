package btcclient

import (
	"fmt"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/netparams"
	"github.com/babylonlabs-io/vigilante/types"
)

// NewWallet creates a new BTC wallet
// used by vigilant submitter
// a wallet is essentially a BTC client
// that connects to the btcWallet daemon
func NewWallet(cfg *config.BTCConfig, parentLogger *zap.Logger) (*Client, error) {
	params, err := netparams.GetBTCParams(cfg.NetParams)
	if err != nil {
		return nil, err
	}
	wallet := &Client{}
	wallet.Cfg = cfg
	wallet.Params = params
	wallet.logger = parentLogger.With(zap.String("module", "btcclient_wallet")).Sugar()

	connCfg := &rpcclient.ConnConfig{}
	switch cfg.BtcBackend {
	case types.Bitcoind:
		// TODO Currently we are not using Params field of rpcclient.ConnConfig due to bug in btcd
		// when handling signet.
		connCfg = &rpcclient.ConnConfig{
			// this will work with node loaded with multiple wallets
			Host:         cfg.Endpoint + "/wallet/" + cfg.WalletName,
			HTTPPostMode: true,
			User:         cfg.Username,
			Pass:         cfg.Password,
			DisableTLS:   cfg.DisableClientTLS,
		}
	case types.Btcd:
		// TODO Currently we are not using Params field of rpcclient.ConnConfig due to bug in btcd
		// when handling signet.
		connCfg = &rpcclient.ConnConfig{
			Host:         cfg.WalletEndpoint,
			Endpoint:     "ws", // websocket
			User:         cfg.Username,
			Pass:         cfg.Password,
			DisableTLS:   cfg.DisableClientTLS,
			Certificates: cfg.ReadWalletCAFile(),
		}
	}

	rpcClient, err := rpcclient.New(connCfg, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create rpc client to BTC for %s backend: %w", cfg.BtcBackend, err)
	}

	wallet.logger.Infof("Successfully connected to %s backend", cfg.BtcBackend)

	wallet.Client = rpcClient

	return wallet, nil
}

func (c *Client) GetWalletPass() string {
	return c.Cfg.WalletPassword
}

func (c *Client) GetWalletLockTime() int64 {
	return c.Cfg.WalletLockTime
}

func (c *Client) GetNetParams() *chaincfg.Params {
	net, err := netparams.GetBTCParams(c.Cfg.NetParams)
	if err != nil {
		panic(fmt.Errorf("failed to get BTC network params: %w", err))
	}
	return net
}

func (c *Client) GetBTCConfig() *config.BTCConfig {
	return c.Cfg
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

func (c *Client) GetRawChangeAddress(account string) (btcutil.Address, error) {
	return c.Client.GetRawChangeAddress(account)
}

func (c *Client) WalletPassphrase(passphrase string, timeoutSecs int64) error {
	return c.Client.WalletPassphrase(passphrase, timeoutSecs)
}

func (c *Client) DumpPrivKey(address btcutil.Address) (*btcutil.WIF, error) {
	return c.Client.DumpPrivKey(address)
}

// GetHighUTXO returns the UTXO that has the highest amount
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

// CalculateTxFee calculates tx fee based on the given fee rate (BTC/kB) and the tx size
func CalculateTxFee(feeRateAmount btcutil.Amount, size uint64) (uint64, error) {
	return uint64(feeRateAmount.MulF64(float64(size) / 1024)), nil
}

func (c *Client) FundRawTransaction(tx *wire.MsgTx, opts btcjson.FundRawTransactionOpts, isWitness *bool) (*btcjson.FundRawTransactionResult, error) {
	return c.Client.FundRawTransaction(tx, opts, isWitness)
}

func (c *Client) SignRawTransactionWithWallet(tx *wire.MsgTx) (*wire.MsgTx, bool, error) {
	return c.Client.SignRawTransactionWithWallet(tx)
}
