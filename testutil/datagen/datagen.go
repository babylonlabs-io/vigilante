package datagen

import (
	"math/rand"

	"github.com/babylonlabs-io/babylon/testutil/datagen"
	"github.com/babylonlabs-io/vigilante/types"
)

func GenerateRandomCheckpointRecord(r *rand.Rand) *types.CheckpointRecord {
	rawCheckpoint := datagen.GenRandomRawCheckpoint(r)
	btcHeight := datagen.RandomIntOtherThan(r, 0, 1000)

	return &types.CheckpointRecord{
		RawCheckpoint:      rawCheckpoint,
		FirstSeenBtcHeight: uint32(btcHeight),
	}
}
