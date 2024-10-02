package stakingeventwatcher

import (
	"fmt"
	"sync"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

type TrackedDelegation struct {
	StakingTx        *wire.MsgTx
	StakingOutputIdx uint32
	UnbondingOutput  *wire.TxOut
}

type TrackedDelegations struct {
	mu sync.Mutex
	// key: staking tx hash
	mapping map[chainhash.Hash]*TrackedDelegation
}

func NewTrackedDelegations() *TrackedDelegations {
	return &TrackedDelegations{
		mapping: make(map[chainhash.Hash]*TrackedDelegation),
	}
}

// GetDelegation returns the tracked delegation for the given staking tx hash or nil if not found.
func (dt *TrackedDelegations) GetDelegation(stakingTxHash chainhash.Hash) *TrackedDelegation {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	del, ok := dt.mapping[stakingTxHash]

	if !ok {
		return nil
	}

	return del
}

// GetDelegations returns all tracked delegations as a slice.
func (dt *TrackedDelegations) GetDelegations() []*TrackedDelegation {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	// Create a slice to hold all delegations
	delegations := make([]*TrackedDelegation, 0, len(dt.mapping))

	// Iterate over the map and collect all values (TrackedDelegation)
	for _, delegation := range dt.mapping {
		delegations = append(delegations, delegation)
	}

	return delegations
}

func (dt *TrackedDelegations) AddDelegation(
	StakingTx *wire.MsgTx,
	StakingOutputIdx uint32,
	UnbondingOutput *wire.TxOut,
) (*TrackedDelegation, error) {
	delegation := &TrackedDelegation{
		StakingTx:        StakingTx,
		StakingOutputIdx: StakingOutputIdx,
		UnbondingOutput:  UnbondingOutput,
	}

	stakingTxHash := StakingTx.TxHash()

	dt.mu.Lock()
	defer dt.mu.Unlock()

	if _, ok := dt.mapping[stakingTxHash]; ok {
		return nil, fmt.Errorf("delegation already tracked for staking tx hash %s", stakingTxHash)
	}

	dt.mapping[stakingTxHash] = delegation
	return delegation, nil
}

func (dt *TrackedDelegations) RemoveDelegation(stakingTxHash chainhash.Hash) {
	dt.mu.Lock()
	defer dt.mu.Unlock()

	delete(dt.mapping, stakingTxHash)
}
