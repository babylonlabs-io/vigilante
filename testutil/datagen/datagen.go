package datagen

import (
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	"github.com/babylonlabs-io/vigilante/types"
	"math/rand"
)

func GenerateRandomCheckpointRecord(r *rand.Rand) *types.CheckpointRecord {
	rawCheckpoint := datagen.GenRandomRawCheckpoint(r)
	btcHeight := datagen.RandomIntOtherThan(r, 0, 1000)

	return &types.CheckpointRecord{
		RawCheckpoint:      rawCheckpoint,
		FirstSeenBtcHeight: btcHeight,
	}
}
