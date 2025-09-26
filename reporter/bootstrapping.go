package reporter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
	bbntypes "github.com/babylonlabs-io/babylon/v4/types"
	"github.com/babylonlabs-io/vigilante/types"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

var (
	bootstrapAttempts      = uint(60)
	bootstrapAttemptsAtt   = retry.Attempts(bootstrapAttempts)
	bootstrapRetryInterval = retry.Delay(30 * time.Second)
	bootstrapDelayType     = retry.DelayType(retry.FixedDelay)
	bootstrapErrReportType = retry.LastErrorOnly(true)
)

type consistencyCheckInfo struct {
	bbnLatestBlockHeight uint32
	startSyncHeight      uint32
}

// checkConsistency checks whether the `max(bbn_tip_height - confirmation_depth, bbn_base_height)` block is same
// between BBN header chain and BTC main chain.` This makes sure that already confirmed chain is the same from point
// of view of both chains.
func (r *Reporter) checkConsistency() (*consistencyCheckInfo, error) {
	tipRes, err := r.babylonClient.BTCHeaderChainTip()
	if err != nil {
		return nil, err
	}

	// Find the base height of BBN header chain
	baseRes, err := r.babylonClient.BTCBaseHeader()
	if err != nil {
		return nil, err
	}

	var consistencyCheckHeight uint32
	if tipRes.Header.Height >= baseRes.Header.Height+r.btcConfirmationDepth {
		consistencyCheckHeight = tipRes.Header.Height - r.btcConfirmationDepth
	} else {
		consistencyCheckHeight = baseRes.Header.Height
	}

	// this checks whether header at already confirmed height is the same in reporter btc cache and in babylon btc light client
	if err := r.checkHeaderConsistency(consistencyCheckHeight); err != nil {
		return nil, err
	}

	return &consistencyCheckInfo{
		bbnLatestBlockHeight: tipRes.Header.Height,
		// we are staring from the block after already confirmed block
		startSyncHeight: consistencyCheckHeight + 1,
	}, nil
}

func (r *Reporter) bootstrap() error {
	var (
		btcLatestBlockHeight uint32
		err                  error
	)

	r.bootstrapMutex.Lock()
	if r.bootstrapInProgress {
		r.bootstrapMutex.Unlock()
		// we only allow one bootstrap process at a time
		return fmt.Errorf("bootstrap already in progress")
	}
	r.bootstrapInProgress = true
	r.bootstrapWg.Add(1)
	r.bootstrapMutex.Unlock()

	// cleanup func in case we error, prevents deadlocks
	defer func() {
		r.bootstrapMutex.Lock()
		r.bootstrapInProgress = false
		r.bootstrapMutex.Unlock()
		r.bootstrapWg.Done()
	}()

	// ensure BTC has caught up with BBN header chain
	if err := r.waitUntilBTCSync(); err != nil {
		return err
	}

	// initialize cache with the latest blocks
	if err := r.initBTCCache(); err != nil {
		return err
	}
	r.logger.Debugf("BTC cache size: %d", r.btcCache.Size())

	consistencyInfo, err := r.checkConsistency()
	if err != nil {
		return err
	}

	// Get current BTC tip height for batched processing
	btcLatestBlockHeight, err = r.btcClient.GetBestBlock()
	if err != nil {
		return fmt.Errorf("failed to get BTC tip height: %w", err)
	}

	r.logger.Infof("BTC height: %d. BTCLightclient height: %d. Start syncing from height %d.",
		btcLatestBlockHeight, consistencyInfo.bbnLatestBlockHeight, consistencyInfo.startSyncHeight)

	// Use batched processing for header submission to avoid memory issues
	if err = r.bootstrapWithBatching(consistencyInfo.startSyncHeight, btcLatestBlockHeight); err != nil {
		// this can happen when there are two contentious vigilantes or if our btc node is behind.
		r.logger.Errorf("Failed to submit headers: %v", err)
		// returning error as it is up to the caller to decide what do next
		return err
	}

	// Cache is already properly initialized and trimmed by initBTCCache()
	// Batched processing has handled all header and checkpoint submission
	// trim cache to the latest k+w blocks on BTC (which are same as in BBN)
	maxEntries := r.btcConfirmationDepth + r.checkpointFinalizationTimeout
	if err = r.btcCache.Resize(maxEntries); err != nil {
		r.logger.Errorf("Failed to resize BTC cache: %v", err)
		panic(err)
	}
	r.btcCache.Trim()

	r.logger.Infof("Size of the BTC cache: %d", r.btcCache.Size())
	r.logger.Info("Successfully finished bootstrapping")

	return nil
}

// bootstrapWithBatching processes blocks in batches to avoid memory issues when catching up many blocks
func (r *Reporter) bootstrapWithBatching(startSyncHeight, tipHeight uint32) error {
	if startSyncHeight > tipHeight {
		r.logger.Debug("No blocks to sync")

		return nil
	}

	batchSize := r.cfg.BatchSize
	signer := r.babylonClient.MustGetAddr()

	r.logger.Infof("Starting batched bootstrap from height %d to %d with batch size %d",
		startSyncHeight, tipHeight, batchSize)

	totalBlocks := tipHeight - startSyncHeight + 1
	processedBlocks := 0

	// Process blocks in batches
	for currentHeight := startSyncHeight; currentHeight <= tipHeight; currentHeight += batchSize {
		// Check if we should stop
		select {
		case <-r.quitChan():
			return fmt.Errorf("bootstrap cancelled")
		default:
		}

		endHeight := currentHeight + batchSize - 1
		if endHeight > tipHeight {
			endHeight = tipHeight
		}

		r.logger.Debugf("Processing batch: heights %d to %d", currentHeight, endHeight)

		// Fetch batch of blocks
		batch, err := r.btcClient.FindBlocksByHeightRange(currentHeight, endHeight)
		if err != nil {
			return fmt.Errorf("failed to fetch blocks %d-%d: %w", currentHeight, endHeight, err)
		}

		// Process headers for this batch
		if _, err = r.ProcessHeaders(signer, batch); err != nil {
			r.logger.Errorf("Failed to submit headers for batch %d-%d: %v", currentHeight, endHeight, err)

			return fmt.Errorf("failed to submit headers: %w", err)
		}

		// Process checkpoints for this batch (async)
		go func(batchBlocks []*types.IndexedBlock) {
			_, _ = r.ProcessCheckpoints(signer, batchBlocks)
		}(batch)

		processedBlocks += len(batch)
		r.logger.Infof("Processed batch %d-%d (%d/%d blocks, %.1f%%)",
			currentHeight, endHeight, processedBlocks, totalBlocks,
			float64(processedBlocks)/float64(totalBlocks)*100)
	}

	r.logger.Info("Batched bootstrap completed successfully")

	return nil
}

func (r *Reporter) reporterQuitCtx() (context.Context, func()) {
	quit := r.quitChan()
	ctx, cancel := context.WithCancel(context.Background())
	r.wg.Add(1)
	go func() {
		defer cancel()
		defer r.wg.Done()

		select {
		case <-quit:

		case <-ctx.Done():
		}
	}()

	return ctx, cancel
}

func (r *Reporter) bootstrapWithRetries() {
	// if we are exiting, we need to cancel this process
	ctx, cancel := r.reporterQuitCtx()
	defer cancel()
	if err := retry.Do(func() error {
		// we don't want to allow concurrent bootstrap process, if bootstrap is already in progress
		// we should wait for it to finish
		r.bootstrapWg.Wait()

		return r.bootstrap()
	},
		retry.Context(ctx),
		bootstrapAttemptsAtt,
		bootstrapRetryInterval,
		bootstrapDelayType,
		bootstrapErrReportType, retry.OnRetry(func(n uint, err error) {
			r.logger.Warnf("Failed to bootstap reporter: %v. Attempt: %d, Max attempts: %d", err, n+1, bootstrapAttempts)
		})); err != nil {
		if errors.Is(err, context.Canceled) {
			// context was cancelled we do not need to anything more, app is quiting
			return
		}

		// we failed to bootstrap multiple time, we should panic as something unexpected is happening.
		r.logger.Fatalf("Failed to bootstrap reporter: %v after %d attempts", err, bootstrapAttempts)
	}
}

// initBTCCache fetches the blocks since T-k-w in the BTC canonical chain
// where T is the height of the latest block in BBN header chain
func (r *Reporter) initBTCCache() error {
	var (
		err                  error
		bbnLatestBlockHeight uint32
		bbnBaseHeight        uint32
		baseHeight           uint32
		ibs                  []*types.IndexedBlock
	)

	r.btcCache, err = types.NewBTCCache(r.cfg.BTCCacheSize) // TODO: give an option to be unsized
	if err != nil {
		panic(err)
	}

	// get T, i.e., total block count in BBN header chain
	tipRes, err := r.babylonClient.BTCHeaderChainTip()
	if err != nil {
		return err
	}
	bbnLatestBlockHeight = tipRes.Header.Height

	// Find the base height
	baseRes, err := r.babylonClient.BTCBaseHeader()
	if err != nil {
		return err
	}
	bbnBaseHeight = baseRes.Header.Height

	// Fetch block since `baseHeight = T - k - w` from BTC, where
	// - T is total block count in BBN header chain
	// - k is btcConfirmationDepth of BBN
	// - w is checkpointFinalizationTimeout of BBN
	if bbnLatestBlockHeight > bbnBaseHeight+r.btcConfirmationDepth+r.checkpointFinalizationTimeout {
		baseHeight = bbnLatestBlockHeight - r.btcConfirmationDepth - r.checkpointFinalizationTimeout + 1
	} else {
		baseHeight = bbnBaseHeight
	}

	// Get current tip height to determine the range
	tipHeight, err := r.btcClient.GetBestBlock()
	if err != nil {
		panic(err)
	}

	ibs, err = r.btcClient.FindBlocksByHeightRange(baseHeight, tipHeight)
	if err != nil {
		panic(err)
	}

	if err = r.btcCache.Init(ibs); err != nil {
		panic(err)
	}

	return nil
}

// waitUntilBTCSync waits for BTC to synchronize until BTC is no shorter than Babylon's BTC light client.
// It returns BTC last block hash, BTC last block height, and Babylon's base height.
func (r *Reporter) waitUntilBTCSync() error {
	var (
		btcLatestBlockHeight uint32
		bbnLatestBlockHash   *chainhash.Hash
		bbnLatestBlockHeight uint32
		err                  error
	)

	// Retrieve hash/height of the latest block in BTC
	btcLatestBlockHeight, err = r.btcClient.GetBestBlock()
	if err != nil {
		return err
	}
	r.logger.Infof("BTC latest block hash and height: (%d)", btcLatestBlockHeight)

	// TODO: if BTC falls behind BTCLightclient's base header, then the vigilante is incorrectly configured and should panic

	// Retrieve hash/height of the latest block in BBN header chain
	tipRes, err := r.babylonClient.BTCHeaderChainTip()
	if err != nil {
		return err
	}

	hash, err := bbntypes.NewBTCHeaderHashBytesFromHex(tipRes.Header.HashHex)
	if err != nil {
		return err
	}

	bbnLatestBlockHash = hash.ToChainhash()
	bbnLatestBlockHeight = tipRes.Header.Height
	r.logger.Infof("BBN header chain latest block hash and height: (%v, %d)", bbnLatestBlockHash, bbnLatestBlockHeight)

	// If BTC chain is shorter than BBN header chain, pause until BTC catches up
	if btcLatestBlockHeight == 0 || btcLatestBlockHeight < bbnLatestBlockHeight {
		r.logger.Infof("BTC chain (length %d) falls behind BBN header chain (length %d), wait until BTC catches up", btcLatestBlockHeight, bbnLatestBlockHeight)

		// periodically check if BTC catches up with BBN.
		// When BTC catches up, break and continue the bootstrapping process
		ticker := time.NewTicker(5 * time.Second) // TODO: parameterise the polling interval
		for range ticker.C {
			btcLatestBlockHeight, err = r.btcClient.GetBestBlock()
			if err != nil {
				return err
			}
			tipRes, err = r.babylonClient.BTCHeaderChainTip()
			if err != nil {
				return err
			}
			bbnLatestBlockHeight = tipRes.Header.Height
			if btcLatestBlockHeight > 0 && btcLatestBlockHeight >= bbnLatestBlockHeight {
				r.logger.Infof("BTC chain (length %d) now catches up with BBN header chain (length %d), continue bootstrapping", btcLatestBlockHeight, bbnLatestBlockHeight)

				break
			}
			r.logger.Infof("BTC chain (length %d) still falls behind BBN header chain (length %d), keep waiting", btcLatestBlockHeight, bbnLatestBlockHeight)
		}
	}

	return nil
}

func (r *Reporter) checkHeaderConsistency(consistencyCheckHeight uint32) error {
	var err error

	consistencyCheckBlock := r.btcCache.FindBlock(consistencyCheckHeight)
	if consistencyCheckBlock == nil {
		err = fmt.Errorf("cannot find the %d-th block of BBN header chain in BTC cache for initial consistency check", consistencyCheckHeight)
		panic(err)
	}
	consistencyCheckHash := consistencyCheckBlock.BlockHash()

	r.logger.Debugf("block for consistency check: height %d, hash %v", consistencyCheckHeight, consistencyCheckHash)

	// Given that two consecutive BTC headers are chained via hash functions,
	// generating a header that can be in two different positions in two different BTC header chains
	// is as hard as breaking the hash function.
	// So as long as the block exists on Babylon, it has to be at the same position as in Babylon as well.
	res, err := r.babylonClient.ContainsBTCBlock(&consistencyCheckHash) // TODO: this API has error. Find out why
	if err != nil {
		return err
	}
	if !res.Contains {
		err = fmt.Errorf("BTC main chain is inconsistent with BBN header chain: k-deep block in BBN header chain: %v", consistencyCheckHash)
		panic(err)
	}

	return nil
}
