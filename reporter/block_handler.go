package reporter

import (
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
		// If the block doesn't connect, clear the cache and bootstrap
		r.btcCache.RemoveAll()

		return fmt.Errorf("block does not connect to the cache, diff hash, bootstrap required")
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
