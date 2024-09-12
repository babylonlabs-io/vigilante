package utils

import (
	"bytes"
	"github.com/btcsuite/btcd/wire"
)

// SerializeMsgTx serializes wire.MsgTx to bytes
func SerializeMsgTx(tx *wire.MsgTx) ([]byte, error) {
	var txBuf bytes.Buffer
	if err := tx.Serialize(&txBuf); err != nil {
		return nil, err
	}

	return txBuf.Bytes(), nil
}

// DeserializeMsgTx deserializes bytes to wire.MsgTx
func DeserializeMsgTx(data []byte) (*wire.MsgTx, error) {
	var tx wire.MsgTx

	if err := tx.Deserialize(bytes.NewReader(data)); err != nil {
		return nil, err
	}

	return &tx, nil
}
