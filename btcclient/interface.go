package btcclient

import (
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"

	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/types"
)

type BTCClient interface {
	Stop()
	WaitForShutdown()
	GetBestBlock() (*chainhash.Hash, uint64, error)
	GetBlockByHash(blockHash *chainhash.Hash) (*types.IndexedBlock, *wire.MsgBlock, error)
	FindTailBlocksByHeight(height uint64) ([]*types.IndexedBlock, error)
	GetBlockByHeight(height uint64) (*types.IndexedBlock, *wire.MsgBlock, error)
	GetTxOut(txHash *chainhash.Hash, index uint32, mempool bool) (*btcjson.GetTxOutResult, error)
	SendRawTransaction(tx *wire.MsgTx, allowHighFees bool) (*chainhash.Hash, error)
	GetTransaction(txHash *chainhash.Hash) (*btcjson.GetTransactionResult, error)
	GetRawTransaction(txHash *chainhash.Hash) (*btcutil.Tx, error)
}

type BTCWallet interface {
	Stop()
	GetWalletPass() string
	GetWalletLockTime() int64
	GetNetParams() *chaincfg.Params
	GetBTCConfig() *config.BTCConfig
	ListUnspent() ([]btcjson.ListUnspentResult, error)
	ListReceivedByAddress() ([]btcjson.ListReceivedByAddressResult, error)
	SendRawTransaction(tx *wire.MsgTx, allowHighFees bool) (*chainhash.Hash, error)
	GetRawChangeAddress(account string) (btcutil.Address, error)
	WalletPassphrase(passphrase string, timeoutSecs int64) error
	GetHighUTXOAndSum() (*btcjson.ListUnspentResult, float64, error)
	FundRawTransaction(tx *wire.MsgTx, opts btcjson.FundRawTransactionOpts, isWitness *bool) (*btcjson.FundRawTransactionResult, error)
	SignRawTransactionWithWallet(tx *wire.MsgTx) (*wire.MsgTx, bool, error)
}
