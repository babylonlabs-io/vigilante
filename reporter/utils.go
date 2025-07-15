package reporter

import (
	"context"
	"fmt"
	"strconv"

	"github.com/babylonlabs-io/babylon/v3/client/babylonclient"
	"github.com/babylonlabs-io/vigilante/retrywrap"
	"github.com/cockroachdb/errors"

	coserrors "cosmossdk.io/errors"
	"github.com/avast/retry-go/v4"
	btcctypes "github.com/babylonlabs-io/babylon/v3/x/btccheckpoint/types"
	btclctypes "github.com/babylonlabs-io/babylon/v3/x/btclightclient/types"
	"github.com/babylonlabs-io/vigilante/types"
)

func chunkBy[T any](items []T, chunkSize int) [][]T {
	var chunks [][]T
	for chunkSize < len(items) {
		items, chunks = items[chunkSize:], append(chunks, items[0:chunkSize:chunkSize])
	}

	return append(chunks, items)
}

// getHeaderMsgsToSubmit creates a set of MsgInsertHeaders messages corresponding to headers that
// should be submitted to Babylon from a given set of indexed blocks
func (r *Reporter) getHeaderMsgsToSubmit(signer string, ibs []*types.IndexedBlock) ([]*btclctypes.MsgInsertHeaders, error) {
	var (
		startPoint  = -1
		ibsToSubmit []*types.IndexedBlock
		err         error
	)

	// find the first header that is not contained in BBN header chain, then submit since this header
	for i, header := range ibs {
		blockHash := header.BlockHash()
		var res *btclctypes.QueryContainsBytesResponse
		err = retrywrap.Do(func() error {
			res, err = r.babylonClient.ContainsBTCBlock(&blockHash)

			return err
		},
			retry.Delay(r.retrySleepTime),
			retry.MaxDelay(r.maxRetrySleepTime),
		)
		if err != nil {
			return nil, err
		}
		if !res.Contains {
			startPoint = i

			break
		}
	}

	// all headers are duplicated, no need to submit
	if startPoint == -1 {
		r.logger.Info("All headers are duplicated, no need to submit")

		return []*btclctypes.MsgInsertHeaders{}, nil
	}

	// wrap the headers to MsgInsertHeaders msgs from the subset of indexed blocks
	ibsToSubmit = ibs[startPoint:]

	blockChunks := chunkBy(ibsToSubmit, int(r.cfg.MaxHeadersInMsg))

	headerMsgsToSubmit := make([]*btclctypes.MsgInsertHeaders, 0, len(blockChunks))

	for _, ibChunk := range blockChunks {
		msgInsertHeaders := types.NewMsgInsertHeaders(signer, ibChunk)
		headerMsgsToSubmit = append(headerMsgsToSubmit, msgInsertHeaders)
	}

	return headerMsgsToSubmit, nil
}

// submitHeaderMsgs attempts to submit a batch of headers to the Babylon client.
// It handles specific expected errors like ErrForkStartWithKnownHeader gracefully,
// and records relevant metrics for both success and failure cases.
func (r *Reporter) submitHeaderMsgs(msg *btclctypes.MsgInsertHeaders) error {
	err := retrywrap.Do(
		func() error {
			res, err := r.babylonClient.ReliablySendMsg(
				context.Background(),
				msg,
				[]*coserrors.Error{btclctypes.ErrForkStartWithKnownHeader}, // expected
				nil, // abort errors
			)
			if err != nil {
				return fmt.Errorf("could not submit headers: %w", err)
			}

			if res == nil {
				// This indicates an expected error occurred (handled internally by ReliablySendMsg)
				r.logger.Infof(
					"expected ErrForkStartWithKnownHeader encountered for %d headers",
					len(msg.Headers),
				)

				return btclctypes.ErrForkStartWithKnownHeader
			}

			r.logger.Infof(
				"successfully submitted %d headers (code: %v)",
				len(msg.Headers), res.Code,
			)

			return nil
		},
		retry.Delay(r.retrySleepTime),
		retry.MaxDelay(r.maxRetrySleepTime),
		retry.Attempts(r.maxRetryTimes),
	)

	if err != nil {
		r.metrics.FailedHeadersCounter.Add(float64(len(msg.Headers)))

		switch {
		case errors.Is(err, babylonclient.ErrTimeoutAfterWaitingForTxBroadcast):
			r.metrics.HeadersCensorshipGauge.Inc()

			return fmt.Errorf("tx broadcast timeout: %w", err)

		case errors.Is(err, btclctypes.ErrForkStartWithKnownHeader):
			// Not a failure, but reported upwards to allow calling code to decide how to handle
			return err

		default:
			return fmt.Errorf("failed to submit headers: %w", err)
		}
	}

	// Success path
	r.metrics.SuccessfulHeadersCounter.Add(float64(len(msg.Headers)))
	r.metrics.SecondsSinceLastHeaderGauge.Set(0)

	for _, header := range msg.Headers {
		r.metrics.NewReportedHeaderGaugeVec.
			WithLabelValues(header.Hash().String()).
			SetToCurrentTime()
	}

	return nil
}

// ProcessHeaders extracts and reports headers from a list of blocks
// It returns the number of headers that need to be reported (after deduplication)
func (r *Reporter) ProcessHeaders(signer string, ibs []*types.IndexedBlock) (int, error) {
	// get a list of MsgInsertHeader msgs with headers to be submitted
	headerMsgsToSubmit, err := r.getHeaderMsgsToSubmit(signer, ibs)
	if err != nil {
		return 0, fmt.Errorf("failed to find headers to submit: %w", err)
	}
	// skip if no header to submit
	if len(headerMsgsToSubmit) == 0 {
		return 0, nil
	}

	var numSubmitted int
	// submit each chunk of headers
	for i := 0; i < len(headerMsgsToSubmit); i++ {
		msgs := headerMsgsToSubmit[i]
		if err := r.submitHeaderMsgs(msgs); err != nil {
			if !errors.Is(err, btclctypes.ErrForkStartWithKnownHeader) {
				return numSubmitted, fmt.Errorf("failed to submit headers: %w", err)
			}

			res, err := r.babylonClient.BTCHeaderChainTip()
			if err != nil {
				return numSubmitted, fmt.Errorf("failed to get BTC header chain tip: %w", err)
			}

			slicedBlocks, shouldRetry := FilterAlreadyProcessedBlocks(ibs, res.Header.Height)
			if !shouldRetry {
				return numSubmitted, nil
			}

			newHeaderMsgs, newErr := r.getHeaderMsgsToSubmit(signer, slicedBlocks)
			if newErr != nil {
				return numSubmitted, fmt.Errorf("failed to find headers to submit: %w", newErr)
			}

			if len(newHeaderMsgs) == 0 {
				r.logger.Info("No new headers to submit after slicing")

				return numSubmitted, nil
			}

			// Replace the remaining headers with the new set and reset the counter
			headerMsgsToSubmit = newHeaderMsgs
			i = -1 // Will be incremented to 0 at the start of the next loop

			continue
		}

		numSubmitted += len(msgs.Headers)
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
