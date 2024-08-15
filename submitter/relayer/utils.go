package relayer

import (
	"bytes"
	"errors"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/mempool"
	"github.com/btcsuite/btcd/wire"
)

func isSegWit(addr btcutil.Address) (bool, error) {
	switch addr.(type) {
	case *btcutil.AddressPubKeyHash, *btcutil.AddressScriptHash, *btcutil.AddressPubKey:
		return false, nil
	case *btcutil.AddressWitnessPubKeyHash, *btcutil.AddressWitnessScriptHash:
		return true, nil
	default:
		return false, errors.New("non-supported address type")
	}
}

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

// IndexOfTxOut searches for the first TxOut with a PkScript of the given length.
// It returns the index of the first match and true if found, otherwise 0 and false.
func IndexOfTxOut(outs []*wire.TxOut, searchLen int) (uint, error) {
	if outs == nil {
		return 0, errors.New("nil outs param")
	}

	for index, out := range outs {
		if len(out.PkScript) == searchLen {
			return uint(index), nil
		}
	}

	return 0, errors.New("no TxOut with PkScript of search len found")
}
