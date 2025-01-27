package types

import (
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// CheckpointInfo stores information of a BTC checkpoint
type CheckpointInfo struct {
	Epoch uint64
	TS    time.Time // the timestamp of the checkpoint being sent
	Tx1   *BtcTxInfo
	Tx2   *BtcTxInfo
}

// BtcTxInfo stores information of a BTC tx as part of a checkpoint
type BtcTxInfo struct {
	TxID *chainhash.Hash
	Tx   *wire.MsgTx
	Size int64          // the size of the BTC tx
	Fee  btcutil.Amount // tx fee cost by the BTC tx
}
