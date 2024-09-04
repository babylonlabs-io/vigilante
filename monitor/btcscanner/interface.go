package btcscanner

import (
	"github.com/babylonlabs-io/vigilante/types"
)

type Scanner interface {
	Start()
	Stop()

	GetCheckpointsChan() chan *types.CheckpointRecord
	GetConfirmedBlocksChan() chan *types.IndexedBlock
}
