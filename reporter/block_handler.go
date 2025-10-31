package reporter

import (
	"context"
	"fmt"
	"github.com/babylonlabs-io/vigilante/types"
	"github.com/btcsuite/btcd/wire"
	"github.com/lightningnetwork/lnd/chainntnfs"
)

// blockEventHandler handles connected and disconnected blocks from the BTC client.
func (r *Reporter) blockEventHandler(blockNotifier *chainntnfs.BlockEpochEvent) {
	defer r.wg.Done()
	quit := r.quitChan()

	defer blockNotifier.Cancel()

	for {
		select {
		case epoch, open := <-blockNotifier.Epochs:
			if !open {
				r.logger.Errorf("Block event channel is closed")

				return // channel closed
			}

			if epoch.Height < 0 {
				panic(fmt.Errorf("received negative epoch height: %d", epoch.Height)) // software bug, panic
			}

			if err := r.handleNewBlock(uint32(epoch.Height), epoch.BlockHeader); err != nil {
				r.logger.Warnf("Due to error in event processing: %v, bootstrap process need to be restarted", err)
				r.bootstrapWithRetries()
			}
		case <-quit:
			// We have been asked to stop
			return
		}
	}
}

// handleNewBlock processes a new block, checking if it connects to the cache or requires bootstrapping.
func (r *Reporter) handleNewBlock(height uint32, header *wire.BlockHeader) error {
	cacheTip := r.btcCache.Tip()
	if cacheTip == nil {
		return fmt.Errorf("cache is empty, restart bootstrap process")
	}

	if cacheTip.Height >= height {
		r.logger.Debugf(
			"the connecting block (height: %d, hash: %s) is too early, skipping the block",
			height,
			header.BlockHash().String(),
		)

		return nil
	}

	if cacheTip.Height+1 < height {
		return fmt.Errorf("missing blocks, expected block height: %d, got: %d", cacheTip.Height+1, height)
	}

	// Check if the new block connects to the cache cacheTip
	parentHash := header.PrevBlock
	if parentHash != cacheTip.BlockHash() {
		// Reorg detected - handle it
		return r.handleReorg(height, header)
	}

	// Block connects to the current chain, add it to the cache
	blockHash := header.BlockHash()
	ib, _, err := r.btcClient.GetBlockByHash(&blockHash)
	if err != nil {
		return fmt.Errorf("failed to get block %v with height %d: %w", blockHash, height, err)
	}

	r.btcCache.Add(ib)

	// Process the new block (submit headers, checkpoints, etc.)
	return r.processNewBlock(ib)
}

// processNewBlock handles further processing of a newly added block.
func (r *Reporter) processNewBlock(ib *types.IndexedBlock) error {
	var headersToProcess []*types.IndexedBlock
	headersToProcess = append(headersToProcess, ib)

	if len(headersToProcess) == 0 {
		r.logger.Debug("No new headers to submit to Babylon")

		return nil
	}

	signer := r.babylonClient.MustGetAddr()
	// Process headers
	if _, err := r.ProcessHeaders(signer, headersToProcess); err != nil {
		r.logger.Warnf("Failed to submit headers: %v", err)

		return fmt.Errorf("failed to submit headers: %w", err)
	}

	// Process checkpoints
	_, _ = r.ProcessCheckpoints(signer, headersToProcess)

	return nil
}

// handleReorg handles a Bitcoin reorg by finding the common ancestor and submitting
// the competing fork to the backend.
func (r *Reporter) handleReorg(newHeight uint32, newHeader *wire.BlockHeader) error {
	r.logger.Infow("Reorg detected",
		"new_height", newHeight,
		"new_hash", newHeader.BlockHash().String(),
		"cache_tip_height", r.btcCache.Tip().Height,
		"cache_tip_hash", r.btcCache.Tip().BlockHash().String(),
	)

	// For Ethereum backend, we need to explicitly submit the competing fork
	// so the contract can detect the reorg and switch chains if the new one is heavier
	if _, ok := r.backend.(*EthereumBackend); ok {
		if err := r.handleEthereumReorg(newHeight, newHeader); err != nil {
			r.logger.Warnf("Failed to handle Ethereum reorg: %v, will trigger bootstrap", err)
		}
	}

	// Clear cache and trigger bootstrap to resync state
	// For Babylon, this is the main mechanism (Babylon handles reorgs internally)
	// For Ethereum, we've already submitted the fork above, now just resync
	r.btcCache.RemoveAll()
	return fmt.Errorf("reorg detected, bootstrap required to resync cache")
}

// handleEthereumReorg finds the common ancestor between the old and new chains,
// then submits blocks from the fork point to trigger the contract's reorg logic.
func (r *Reporter) handleEthereumReorg(newHeight uint32, newHeader *wire.BlockHeader) error {
	// Find the common ancestor by walking back the new chain
	commonAncestorHeight, err := r.findCommonAncestor(newHeight, newHeader)
	if err != nil {
		return fmt.Errorf("failed to find common ancestor: %w", err)
	}

	r.logger.Infow("Found common ancestor for reorg",
		"ancestor_height", commonAncestorHeight,
		"new_chain_tip", newHeight,
		"reorg_depth", newHeight-commonAncestorHeight,
	)

	// Fetch blocks from BTC starting from common ancestor to current tip
	// These blocks represent the new competing fork
	blocks, err := r.btcClient.FindTailBlocksByHeight(commonAncestorHeight)
	if err != nil {
		return fmt.Errorf("failed to fetch blocks from height %d: %w", commonAncestorHeight, err)
	}

	// Submit the competing fork to the Ethereum contract
	// The contract will detect the fork (since we're submitting from a height < current tip)
	// and will switch to the new chain if it has higher total difficulty
	signer := r.babylonClient.MustGetAddr()
	if _, err := r.ProcessHeaders(signer, blocks); err != nil {
		return fmt.Errorf("failed to submit reorg blocks starting from height %d: %w", commonAncestorHeight, err)
	}

	r.logger.Infow("Successfully submitted competing fork to Ethereum contract",
		"fork_start_height", commonAncestorHeight,
		"fork_end_height", newHeight,
		"num_blocks", len(blocks),
	)

	return nil
}

// findCommonAncestor walks back from the new chain tip until it finds a block
// that exists in the backend. This is the fork point between old and new chains.
func (r *Reporter) findCommonAncestor(height uint32, header *wire.BlockHeader) (uint32, error) {
	currentHeight := height
	currentHash := header.BlockHash()

	// Walk backwards through the new chain until we find a block in the backend
	for {
		// Check if backend has this block
		contains, err := r.backend.ContainsBlock(context.Background(), &currentHash)
		if err != nil {
			return 0, fmt.Errorf("failed to check if backend contains block %v at height %d: %w",
				currentHash, currentHeight, err)
		}

		if contains {
			// Found the common ancestor - this block exists in both chains
			r.logger.Debugw("Found common ancestor",
				"height", currentHeight,
				"hash", currentHash.String(),
			)
			return currentHeight, nil
		}

		// Move to parent block
		if currentHeight == 0 {
			return 0, fmt.Errorf("reached genesis without finding common ancestor")
		}

		// Get the parent block to continue walking backwards
		ib, _, err := r.btcClient.GetBlockByHash(&currentHash)
		if err != nil {
			return 0, fmt.Errorf("failed to get block %v at height %d: %w",
				currentHash, currentHeight, err)
		}

		currentHeight--
		currentHash = ib.Header.PrevBlock
	}
}
