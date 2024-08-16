package relayer

import (
	"bytes"
	"errors"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/mempool"
	"github.com/btcsuite/btcd/wire"
)

func calculateTxVirtualSize(tx *wire.MsgTx) (int64, error) {
	var txBytes bytes.Buffer
	if err := tx.Serialize(&txBytes); err != nil {
		return 0, err
	}

	btcTx, err := btcutil.NewTxFromBytes(txBytes.Bytes())
	if err != nil {
		return 0, err
	}

	return mempool.GetTxVirtualSize(btcTx), nil
}

// IndexOfChangeAddr returns the index of the first transaction output with a non-zero value,
// indicating a change address, or an error if none is found.
func IndexOfChangeAddr(outs []*wire.TxOut) (uint, error) {
	if outs == nil {
		return 0, errors.New("nil outs param")
	}

	for index, out := range outs {
		// A change address output must have a non-zero value.
		if out.Value != 0 {
			return uint(index), nil
		}
	}

	return 0, errors.New("no TxOut with change addr found")
}
