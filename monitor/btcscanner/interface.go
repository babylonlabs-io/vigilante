package btcscanner

import (
	"github.com/babylonlabs-io/vigilante/types"
	"github.com/btcsuite/btcd/wire"
)

type Scanner interface {
	// common functions
	Start()
	Stop()

	GetCheckpointsChan() chan *types.CheckpointRecord
	GetHeadersChan() chan *wire.BlockHeader
}
