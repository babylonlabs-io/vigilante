package relayer

import (
	"bytes"
	"errors"

	"github.com/babylonlabs-io/vigilante/types"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/mempool"
	"github.com/btcsuite/btcd/txscript"
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

func completeTxIn(tx *wire.MsgTx, isSegWit bool, privKey *btcec.PrivateKey, utxo *types.UTXO) (*wire.MsgTx, error) {
	if !isSegWit {
		sig, err := txscript.SignatureScript(
			tx,
			0,
			utxo.ScriptPK,
			txscript.SigHashAll,
			privKey,
			true,
		)
		if err != nil {
			return nil, err
		}
		tx.TxIn[0].SignatureScript = sig
	} else {
		sighashes := txscript.NewTxSigHashes(
			tx,
			// Use the CannedPrevOutputFetcher which is only able to return information about a single UTXO
			// See https://github.com/btcsuite/btcd/commit/e781b66e2fb9a354a14bfa7fbdd44038450cc13f
			// for details on the output fetchers
			txscript.NewCannedPrevOutputFetcher(utxo.ScriptPK, int64(utxo.Amount)))
		wit, err := txscript.WitnessSignature(
			tx,
			sighashes,
			0,
			int64(utxo.Amount),
			utxo.ScriptPK,
			txscript.SigHashAll,
			privKey,
			true,
		)
		if err != nil {
			return nil, err
		}
		tx.TxIn[0].Witness = wit
	}

	return tx, nil
}

func IndexOfTxOut(outs []*wire.TxOut, searchLen int) (uint, bool) {
	for index, out := range outs {
		if len(out.PkScript) == searchLen {
			return uint(index), true
		}
	}

	return 0, false
}
