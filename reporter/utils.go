package reporter

import (
	"context"
	"fmt"
	"strconv"

	"github.com/babylonlabs-io/babylon/v4/client/babylonclient"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/retrywrap"
	"github.com/cockroachdb/errors"

	coserrors "cosmossdk.io/errors"
	"github.com/avast/retry-go/v4"
	btcctypes "github.com/babylonlabs-io/babylon/v4/x/btccheckpoint/types"
	btclctypes "github.com/babylonlabs-io/babylon/v4/x/btclightclient/types"
	"github.com/babylonlabs-io/vigilante/types"
)

func chunkBy[T any](items []T, chunkSize int) [][]T {
	var chunks [][]T
	for chunkSize < len(items) {
		items, chunks = items[chunkSize:], append(chunks, items[0:chunkSize:chunkSize])
	}

	return append(chunks, items)
}

// getHeadersToSubmit identifies which headers need to be submitted to the backend.
// It checks each header against the backend to avoid duplicates and returns chunks of headers
// ready for submission.
func (r *Reporter) getHeadersToSubmit(ibs []*types.IndexedBlock) ([][]*types.IndexedBlock, error) {
	var (
		startPoint  = -1
		ibsToSubmit []*types.IndexedBlock
		err         error
	)

	// find the first header that is not contained in the backend, then submit since this header
	for i, ib := range ibs {
		blockHash := ib.BlockHash()
		blockHeight := ib.Height
		var contains bool
		err = retrywrap.Do(func() error {
			contains, err = r.backend.ContainsBlock(context.Background(), &blockHash, blockHeight)

			return err
		},
			retry.Delay(r.retrySleepTime),
			retry.MaxDelay(r.maxRetrySleepTime),
		)
		if err != nil {
			return nil, err
		}
		if !contains {
			startPoint = i

			break
		}
	}

	// all headers are duplicated, no need to submit
	if startPoint == -1 {
		r.logger.Info("All headers are duplicated, no need to submit")

		return [][]*types.IndexedBlock{}, nil
	}

	// get the headers to submit from the subset of indexed blocks
	ibsToSubmit = ibs[startPoint:]

	// chunk the headers based on max headers per message
	blockChunks := chunkBy(ibsToSubmit, int(r.cfg.MaxHeadersInMsg))

	return blockChunks, nil
}

// submitHeaders submits a batch of headers to the backend.
// It extracts raw header bytes from IndexedBlocks and calls backend.SubmitHeaders.
func (r *Reporter) submitHeaders(ibs []*types.IndexedBlock) error {
	if len(ibs) == 0 {
		return fmt.Errorf("no headers to submit")
	}

	// Extract raw header bytes from IndexedBlocks
	headers := make([][]byte, len(ibs))
	for i, ib := range ibs {
		headerBytes, err := types.SerializeBlockHeader(ib.Header)
		if err != nil {
			return fmt.Errorf("failed to serialize header at index %d: %w", i, err)
		}
		headers[i] = headerBytes
	}

	startHeight := uint64(ibs[0].Height)
	endHeight := uint64(ibs[len(ibs)-1].Height)

	r.logger.Infof("Preparing to submit %d headers: heights %d to %d", len(ibs), startHeight, endHeight)

	// Submit to backend with retry logic
	err := retrywrap.Do(
		func() error {
			// Before submitting, check if headers are already confirmed on-chain.
			// This handles the case where a previous attempt succeeded but timed out waiting for confirmation.
			tipHeight, _, tipErr := r.backend.GetTip(context.Background())
			if tipErr == nil && uint64(tipHeight) >= endHeight {
				r.logger.Infof("Headers already confirmed on-chain (tip=%d >= endHeight=%d), skipping submission",
					tipHeight, endHeight)

				return nil // Success - headers already submitted
			}

			err := r.backend.SubmitHeaders(context.Background(), startHeight, headers)
			// Gap error is unrecoverable - retrying won't help, needs bootstrap
			if errors.Is(err, ErrGapInHeaderChain) {
				return retry.Unrecoverable(err)
			}

			return err
		},
		retry.Delay(r.retrySleepTime),
		retry.MaxDelay(r.maxRetrySleepTime),
		retry.Attempts(r.maxRetryTimes),
		retry.OnRetry(func(n uint, err error) {
			r.logger.Warnf("Failed to submit headers: %v. Attempt: %d, Max attempts: %d", err, n+1, r.maxRetryTimes)
		}),
	)

	if err != nil {
		r.metrics.FailedHeadersCounter.Add(float64(len(headers)))

		// Detect censorship: if submission timed out waiting for tx broadcast
		if errors.Is(err, babylonclient.ErrTimeoutAfterWaitingForTxBroadcast) {
			r.metrics.HeadersCensorshipGauge.Inc()
		}

		return fmt.Errorf("failed to submit headers: %w", err)
	}

	// Success path
	r.metrics.SuccessfulHeadersCounter.Add(float64(len(headers)))
	r.metrics.SecondsSinceLastHeaderGauge.Set(0)

	for _, ib := range ibs {
		r.metrics.NewReportedHeaderGaugeVec.
			WithLabelValues(ib.BlockHash().String()).
			SetToCurrentTime()
	}

	r.logger.Infof("Successfully submitted %d headers starting at height %d", len(headers), startHeight)

	return nil
}

// ProcessHeaders extracts and reports headers from a list of blocks
// It returns the number of headers that need to be reported (after deduplication)
func (r *Reporter) ProcessHeaders(_ string, ibs []*types.IndexedBlock) (int, error) {
	// get chunks of headers to be submitted
	headerChunks, err := r.getHeadersToSubmit(ibs)
	if err != nil {
		return 0, fmt.Errorf("failed to find headers to submit: %w", err)
	}
	// skip if no header to submit
	if len(headerChunks) == 0 {
		return 0, nil
	}

	var numSubmitted int
	// submit each chunk of headers with special handling for Babylon fork errors
	for i := 0; i < len(headerChunks); i++ {
		chunk := headerChunks[i]
		if err := r.submitHeaders(chunk); err != nil {
			// Special handling for Babylon's ErrForkStartWithKnownHeader
			// This occurs when headers are already in Babylon (race condition with another reporter)
			if !errors.Is(err, btclctypes.ErrForkStartWithKnownHeader) {
				return numSubmitted, fmt.Errorf("failed to submit headers: %w", err)
			}

			// For Babylon backend: get tip and retry with remaining blocks
			// This matches the logic from main branch to handle concurrent reporters
			_, isBabylon := r.backend.(*BabylonBackend)
			if !isBabylon {
				// Non-Babylon backends shouldn't return this error, but if they do, treat as failure
				return numSubmitted, fmt.Errorf("unexpected ErrForkStartWithKnownHeader from non-Babylon backend: %w", err)
			}

			// Get current Babylon tip to determine which blocks are already processed
			tipHeight, _, err := r.backend.GetTip(context.Background())
			if err != nil {
				return numSubmitted, fmt.Errorf("failed to get backend tip after fork error: %w", err)
			}

			// Filter out blocks that are already processed (at or below tip)
			slicedBlocks, shouldRetry := FilterAlreadyProcessedBlocks(ibs, tipHeight)
			if !shouldRetry {
				// All blocks are already processed, nothing left to submit
				r.logger.Info("All blocks already processed after fork error, no retry needed")

				return numSubmitted, nil
			}

			// Get new chunks for the remaining blocks
			newHeaderChunks, err := r.getHeadersToSubmit(slicedBlocks)
			if err != nil {
				return numSubmitted, fmt.Errorf("failed to find headers to submit after filtering: %w", err)
			}

			if len(newHeaderChunks) == 0 {
				r.logger.Info("No new headers to submit after slicing")

				return numSubmitted, nil
			}

			// Replace the remaining chunks with the new set and reset the counter
			headerChunks = newHeaderChunks
			i = -1 // Will be incremented to 0 at the start of the next loop

			continue
		}

		numSubmitted += len(chunk)
	}

	return numSubmitted, nil
}

func FilterAlreadyProcessedBlocks(ibs []*types.IndexedBlock, height uint32) ([]*types.IndexedBlock, bool) {
	if len(ibs) == 0 {
		return []*types.IndexedBlock{}, false
	}

	lastHeaderHeight := ibs[len(ibs)-1].Height

	if height >= lastHeaderHeight {
		return nil, false
	}

	for i, block := range ibs {
		if block.Height > height {
			newSlice := make([]*types.IndexedBlock, len(ibs)-i)
			copy(newSlice, ibs[i:])

			return newSlice, true
		}
	}

	return nil, false
}

func (r *Reporter) extractCheckpoints(ib *types.IndexedBlock) int {
	// for each tx, try to extract a ckpt segment from it.
	// If there is a ckpt segment, cache it to ckptCache locally
	numCkptSegs := 0

	for _, tx := range ib.Txs {
		if tx == nil {
			r.logger.Warnf("Found a nil tx in block %v", ib.BlockHash())

			continue
		}

		// cache the segment to ckptCache
		ckptSeg := types.NewCkptSegment(r.checkpointCache.Tag, r.checkpointCache.Version, ib, tx)
		if ckptSeg != nil {
			r.logger.Infof("Found a checkpoint segment in tx %v with index %d: %v", tx.Hash(), ckptSeg.Index, ckptSeg.Data)
			if err := r.checkpointCache.AddSegment(ckptSeg); err != nil {
				r.logger.Errorf("Failed to add the ckpt segment in tx %v to the ckptCache: %v", tx.Hash(), err)

				continue
			}
			numCkptSegs++
		}
	}

	return numCkptSegs
}

func (r *Reporter) matchAndSubmitCheckpoints(signer string) int {
	var (
		proofs               []*btcctypes.BTCSpvProof
		msgInsertBTCSpvProof *btcctypes.MsgInsertBTCSpvProof
	)

	// get matched ckpt parts from the ckptCache
	// Note that Match() has ensured the checkpoints are always ordered by epoch number
	r.checkpointCache.Match()
	numMatchedCkpts := r.checkpointCache.NumCheckpoints()

	if numMatchedCkpts == 0 {
		r.logger.Debug("Found no matched pair of checkpoint segments in this match attempt")

		return numMatchedCkpts
	}

	// for each matched checkpoint, wrap to MsgInsertBTCSpvProof and send to Babylon
	// Note that this is a while loop that keeps popping checkpoints in the cache
	for {
		// pop the earliest checkpoint
		// if popping a nil checkpoint, then all checkpoints are popped, break the for loop
		ckpt := r.checkpointCache.PopEarliestCheckpoint()
		if ckpt == nil {
			break
		}

		r.logger.Info("Found a matched pair of checkpoint segments!")

		// fetch the first checkpoint in cache and construct spv proof
		proofs = ckpt.MustGenSPVProofs()

		// wrap to MsgInsertBTCSpvProof
		msgInsertBTCSpvProof = types.MustNewMsgInsertBTCSpvProof(signer, proofs)
		tx1Block := ckpt.Segments[0].AssocBlock
		tx2Block := ckpt.Segments[1].AssocBlock

		// submit the checkpoint to Babylon
		res, err := r.babylonClient.ReliablySendMsg(
			context.Background(),
			msgInsertBTCSpvProof,
			[]*coserrors.Error{
				btcctypes.ErrDuplicatedSubmission,
				btcctypes.ErrEpochAlreadyFinalized,
			},
			[]*coserrors.Error{},
		)

		// nolint:gocritic // preferred over using a switch statement
		if err != nil {
			r.logger.Errorf("Failed to submit MsgInsertBTCSpvProof with error %v", err)
			r.metrics.FailedCheckpointsCounter.Inc()

			if errors.Is(err, babylonclient.ErrTimeoutAfterWaitingForTxBroadcast) {
				r.logger.Warnf("Censorship detected in inserting checkpoints to Babylon, for epoch %d, tx1 %s, tx2 %s, height %d",
					ckpt.Epoch,
					tx1Block.Txs[ckpt.Segments[0].TxIdx].Hash().String(),
					tx2Block.Txs[ckpt.Segments[1].TxIdx].Hash().String(),
					strconv.Itoa(int(tx1Block.Height)))

				r.metrics.CheckpointCensorshipGauge.WithLabelValues(
					strconv.FormatUint(ckpt.Epoch, 10),
					strconv.Itoa(int(tx1Block.Height)),
					tx1Block.Txs[ckpt.Segments[0].TxIdx].Hash().String(),
					tx2Block.Txs[ckpt.Segments[1].TxIdx].Hash().String(),
				).Inc()
			}

			continue
		} else if res == nil {
			r.logger.Infof("Checkpoint (MsgInsertBTCSpvProof) already submitted, for epoch: %d, tx1: %s, tx2: %s, height: %s",
				ckpt.Epoch,
				tx1Block.Txs[ckpt.Segments[0].TxIdx].Hash().String(),
				tx2Block.Txs[ckpt.Segments[1].TxIdx].Hash().String(),
				strconv.Itoa(int(tx1Block.Height)))
		} else {
			r.logger.Infof("Successfully submitted checkpoint (MsgInsertBTCSpvProof) with response %d, for epoch %d, height %d",
				res.Code, ckpt.Epoch, tx1Block.Height)
		}

		// either way if we or some other reporter submitted the checkpoint, we want to update the metrics
		r.metrics.SuccessfulCheckpointsCounter.Inc()
		r.metrics.SecondsSinceLastCheckpointGauge.Set(0)
		r.metrics.NewReportedCheckpointGaugeVec.WithLabelValues(
			strconv.FormatUint(ckpt.Epoch, 10),
			strconv.Itoa(int(tx1Block.Height)),
			tx1Block.Txs[ckpt.Segments[0].TxIdx].Hash().String(),
			tx2Block.Txs[ckpt.Segments[1].TxIdx].Hash().String(),
		).SetToCurrentTime()
	}

	return numMatchedCkpts
}

// ProcessCheckpoints tries to extract checkpoint segments from a list of blocks, find matched checkpoint segments, and report matched checkpoints
// It returns the number of extracted checkpoint segments, and the number of matched checkpoints
func (r *Reporter) ProcessCheckpoints(signer string, ibs []*types.IndexedBlock) (int, int) {
	if r.cfg.BackendType == config.BackendTypeEthereum {
		return 0, 0
	}
	var numCkptSegs int

	// extract ckpt segments from the blocks
	for _, ib := range ibs {
		numCkptSegs += r.extractCheckpoints(ib)
	}

	if numCkptSegs > 0 {
		r.logger.Infof("Found %d checkpoint segments", numCkptSegs)
	}

	// match and submit checkpoint segments
	numMatchedCkpts := r.matchAndSubmitCheckpoints(signer)

	return numCkptSegs, numMatchedCkpts
}
