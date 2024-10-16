package types

import (
	ckpttypes "github.com/babylonlabs-io/babylon/x/checkpointing/types"
)

type CheckpointRecord struct {
	RawCheckpoint      *ckpttypes.RawCheckpoint
	FirstSeenBtcHeight uint32
}

func NewCheckpointRecord(ckpt *ckpttypes.RawCheckpoint, height uint32) *CheckpointRecord {
	return &CheckpointRecord{RawCheckpoint: ckpt, FirstSeenBtcHeight: height}
}

// ID returns the hash of the raw checkpoint
func (cr *CheckpointRecord) ID() string {
	return cr.RawCheckpoint.Hash().String()
}

func (cr *CheckpointRecord) EpochNum() uint64 {
	return cr.RawCheckpoint.EpochNum
}
