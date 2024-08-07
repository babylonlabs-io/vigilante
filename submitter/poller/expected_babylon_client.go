package poller

import (
	checkpointingtypes "github.com/babylonlabs-io/babylon/x/checkpointing/types"
	sdkquerytypes "github.com/cosmos/cosmos-sdk/types/query"
)

type BabylonQueryClient interface {
	RawCheckpointList(status checkpointingtypes.CheckpointStatus, pagination *sdkquerytypes.PageRequest) (*checkpointingtypes.QueryRawCheckpointListResponse, error)
}
