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
	mu sync.RWMutex
	// key: staking tx hash
	mapping map[chainhash.Hash]*TrackedDelegation
}

func NewTrackedDelegations() *TrackedDelegations {
	return &TrackedDelegations{
		mapping: make(map[chainhash.Hash]*TrackedDelegation),
	}
}

// GetDelegation returns the tracked delegation for the given staking tx hash or nil if not found.
func (td *TrackedDelegations) GetDelegation(stakingTxHash chainhash.Hash) *TrackedDelegation {
	td.mu.RLock()
	defer td.mu.RUnlock()

	del, ok := td.mapping[stakingTxHash]

	if !ok {
		return nil
	}

	return del
}

// GetDelegations returns all tracked delegations as a slice.
func (td *TrackedDelegations) GetDelegations() []*TrackedDelegation {
	td.mu.RLock()
	defer td.mu.RUnlock()

	// Create a slice to hold all delegations
	delegations := make([]*TrackedDelegation, 0, len(td.mapping))

	// Iterate over the map and collect all values (TrackedDelegation)
	for _, delegation := range td.mapping {
		delegations = append(delegations, delegation)
	}

	return delegations
}

func (td *TrackedDelegations) AddDelegation(
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

	td.mu.Lock()
	defer td.mu.Unlock()

	if _, ok := td.mapping[stakingTxHash]; ok {
		return nil, fmt.Errorf("delegation already tracked for staking tx hash %s", stakingTxHash)
	}

	td.mapping[stakingTxHash] = delegation
	return delegation, nil
}

func (td *TrackedDelegations) RemoveDelegation(stakingTxHash chainhash.Hash) {
	td.mu.Lock()
	defer td.mu.Unlock()

	delete(td.mapping, stakingTxHash)
}
