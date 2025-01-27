package stakingeventwatcher

import (
	"fmt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/sasha-s/go-deadlock"
	"go.uber.org/atomic"
	"iter"
)

type TrackedDelegation struct {
	StakingTx             *wire.MsgTx
	StakingOutputIdx      uint32
	UnbondingOutput       *wire.TxOut
	DelegationStartHeight uint32
	ActivationInProgress  bool
}

type TrackedDelegations struct {
	mu deadlock.RWMutex
	// key: staking tx hash
	mapping map[chainhash.Hash]*TrackedDelegation
	count   atomic.Int32
}

func NewTrackedDelegations() *TrackedDelegations {
	return &TrackedDelegations{
		mapping: make(map[chainhash.Hash]*TrackedDelegation),
	}
}

// GetDelegation returns the tracked delegation for the given staking tx hash or nil if not found.
func (td *TrackedDelegations) GetDelegation(stakingTxHash chainhash.Hash) (*TrackedDelegation, bool) {
	td.mu.RLock()
	defer td.mu.RUnlock()

	del, ok := td.mapping[stakingTxHash]
	if !ok {
		return nil, false
	}

	return del, true
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

// Clone creates a deep copy of the TrackedDelegation
func (td *TrackedDelegation) Clone() *TrackedDelegation {
	if td == nil {
		return nil
	}

	// Deep copy the StakingTx
	var stakingTx *wire.MsgTx
	if td.StakingTx != nil {
		stakingTx = td.StakingTx.Copy()
	}

	// Deep copy the UnbondingOutput
	var unbondingOutput *wire.TxOut
	if td.UnbondingOutput != nil {
		unbondingOutput = &wire.TxOut{
			Value:    td.UnbondingOutput.Value,
			PkScript: append([]byte(nil), td.UnbondingOutput.PkScript...),
		}
	}

	return &TrackedDelegation{
		StakingTx:             stakingTx,
		StakingOutputIdx:      td.StakingOutputIdx,
		UnbondingOutput:       unbondingOutput,
		DelegationStartHeight: td.DelegationStartHeight,
		ActivationInProgress:  td.ActivationInProgress,
	}
}

// DelegationsIter returns an iterator that yields copies of delegations in chunks.
// If chunkSize <= 0, it defaults to 100.
func (td *TrackedDelegations) DelegationsIter(chunkSize int) iter.Seq[*TrackedDelegation] {
	if chunkSize <= 0 {
		chunkSize = 100
	}

	return func(yield func(*TrackedDelegation) bool) {
		// Pre-allocate the processed map with expected capacity
		td.mu.RLock()
		processedCap := len(td.mapping)
		td.mu.RUnlock()
		processed := make(map[chainhash.Hash]struct{}, processedCap)

		// Create a buffer for batch processing
		buffer := make([]*TrackedDelegation, 0, chunkSize)

		for {
			buffer = buffer[:0] // Reset buffer without reallocating

			td.mu.RLock()
			// Collect a batch of unprocessed items directly
			for k, v := range td.mapping {
				if _, ok := processed[k]; !ok {
					buffer = append(buffer, v.Clone())
					processed[k] = struct{}{}
					if len(buffer) >= chunkSize {
						break
					}
				}
			}
			mapSize := len(td.mapping)
			td.mu.RUnlock()

			// Process the batch
			for _, delegation := range buffer {
				if !yield(delegation) {
					return
				}
			}

			// Check if we've processed everything
			if len(processed) >= mapSize {
				break
			}
		}
	}
}

func (td *TrackedDelegations) AddDelegation(
	stakingTx *wire.MsgTx,
	stakingOutputIdx uint32,
	unbondingOutput *wire.TxOut,
	delegationStartHeight uint32,
	shouldUpdate bool,
) (*TrackedDelegation, error) {
	delegation := &TrackedDelegation{
		StakingTx:             stakingTx,
		StakingOutputIdx:      stakingOutputIdx,
		UnbondingOutput:       unbondingOutput,
		DelegationStartHeight: delegationStartHeight,
	}

	stakingTxHash := stakingTx.TxHash()

	td.mu.Lock()
	defer td.mu.Unlock()

	if _, ok := td.mapping[stakingTxHash]; ok {
		if shouldUpdate {
			// Update the existing delegation
			td.mapping[stakingTxHash] = delegation

			return delegation, nil
		}

		return nil, fmt.Errorf("delegation already tracked for staking tx hash %s", stakingTxHash)
	}

	td.mapping[stakingTxHash] = delegation
	td.count.Inc()

	return delegation, nil
}

func (td *TrackedDelegations) AddEmptyDelegation(txHash chainhash.Hash) error {
	td.mu.Lock()
	defer td.mu.Unlock()

	if _, ok := td.mapping[txHash]; ok {
		return fmt.Errorf("already tracked staking tx hash: %s", txHash)
	}

	td.mapping[txHash] = nil
	td.count.Inc()

	return nil
}

func (td *TrackedDelegations) RemoveDelegation(stakingTxHash chainhash.Hash) {
	td.mu.Lock()
	defer td.mu.Unlock()

	if _, exists := td.mapping[stakingTxHash]; exists {
		delete(td.mapping, stakingTxHash)
		td.count.Dec()
	}
}

func (td *TrackedDelegations) HasDelegationChanged(
	stakingTxHash chainhash.Hash,
	newDelegation *newDelegation,
) (bool, bool) {
	td.mu.RLock()
	defer td.mu.RUnlock()

	// Check if the delegation exists in the map
	existingDelegation, exists := td.mapping[stakingTxHash]
	if !exists {
		// If it doesn't exist, return false for changed, and false for exists
		return false, false
	}

	// Compare height to check if the delegation has changed
	if existingDelegation.DelegationStartHeight != newDelegation.delegationStartHeight {
		return true, true // The delegation has changed and it exists
	}

	// The delegation exists but hasn't changed
	return false, true
}

func (td *TrackedDelegations) UpdateActivation(tx chainhash.Hash, inProgress bool) error {
	td.mu.Lock()
	defer td.mu.Unlock()

	delegation, ok := td.mapping[tx]
	if !ok {
		return fmt.Errorf("delegation with tx hash %s not found", tx.String())
	}

	delegation.ActivationInProgress = inProgress

	return nil
}

func (td *TrackedDelegations) Count() int {
	return int(td.count.Load())
}
