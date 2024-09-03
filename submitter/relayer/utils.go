package relayer

import (
	"errors"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/mempool"
	"github.com/btcsuite/btcd/wire"
)

func calculateTxVirtualSize(tx *wire.MsgTx) (int64, error) {
	if tx == nil {
		return -1, errors.New("tx param nil")
	}

	return mempool.GetTxVirtualSize(btcutil.NewTx(tx)), nil
}
