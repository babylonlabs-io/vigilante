package reporter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/avast/retry-go/v4"
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

// checkConsistency checks whether the `max(backend_tip_height - confirmation_depth, backend_base_height)` block is same
// between backend header chain and BTC main chain. This makes sure that already confirmed chain is the same from point
// of view of both chains.
func (r *Reporter) checkConsistency() (*consistencyCheckInfo, error) {
	// Get backend tip
	backendTipHeight, _, err := r.backend.GetTip(context.Background())
	if err != nil {
		return nil, err
	}

	// Find the base height
	var backendBaseHeight uint32
	if babylonBackend, ok := r.backend.(*BabylonBackend); ok {
		// For Babylon: query base header (pruning)
		baseRes, err := babylonBackend.client.BTCBaseHeader()
		if err != nil {
			return nil, err
		}
		backendBaseHeight = baseRes.Header.Height
	} else {
		// For Ethereum: the contract doesn't prune, so base height is either:
		// 1. The contract's genesis (for freshly deployed contracts)
		// 2. A calculated value (tip - k - w) for contracts with history
		//
		// We use a simple heuristic: assume the contract has been running long enough
		// to have k+w blocks, otherwise it's freshly deployed and base = tip
		if backendTipHeight > r.btcConfirmationDepth+r.checkpointFinalizationTimeout {
			backendBaseHeight = backendTipHeight - r.btcConfirmationDepth - r.checkpointFinalizationTimeout + 1
		} else {
			// For contracts with very few blocks (< k+w total), the effective base is
			// much closer to the tip. In the extreme case (only genesis), base == tip.
			// Since we can't easily query the contract's genesis height, we use 0 as
			// a conservative estimate.
			backendBaseHeight = 0
		}
	}

	var consistencyCheckHeight uint32
	// For Babylon backends with pruning, we use the standard calculation
	if _, ok := r.backend.(*BabylonBackend); ok {
		if backendTipHeight >= backendBaseHeight+r.btcConfirmationDepth {
			consistencyCheckHeight = backendTipHeight - r.btcConfirmationDepth
		} else {
			consistencyCheckHeight = backendBaseHeight
		}
	} else {
		// For Ethereum backends: find a common ancestor between the contract and BTC node.
		// After a reorg, the contract may be on a different fork than the BTC node.
		// We need to find a block that exists in BOTH chains with the same hash,
		// then start submitting from there + 1.
		commonAncestor, err := r.findEthCommonAncestor(backendTipHeight)
		if err != nil {
			return nil, fmt.Errorf("failed to find common ancestor for ETH backend: %w", err)
		}
		r.logger.Infof("Found common ancestor at height %d for Ethereum backend (contract tip=%d)", commonAncestor, backendTipHeight)

		return &consistencyCheckInfo{
			bbnLatestBlockHeight: backendTipHeight,
			startSyncHeight:      commonAncestor + 1,
		}, nil
	}

	// this checks whether header at already confirmed height is the same in reporter btc cache and in backend
	if err := r.checkHeaderConsistency(consistencyCheckHeight); err != nil {
		return nil, err
	}

	return &consistencyCheckInfo{
		bbnLatestBlockHeight: backendTipHeight,
		// we are starting from the block after already confirmed block
		startSyncHeight: consistencyCheckHeight + 1,
	}, nil
}

// findEthCommonAncestor finds a common ancestor block between the Ethereum contract and BTC node.
// This is needed because after a reorg, the contract may be on a different fork than the BTC node.
// We walk backward from the contract tip until we find a block that exists in both chains with the same hash.
// Returns the height of the common ancestor block.
func (r *Reporter) findEthCommonAncestor(contractTipHeight uint32) (uint32, error) {
	ctx := context.Background()

	// Maximum blocks to search backward (limited by contract's circular buffer of 2000 blocks)
	// and practical reorg limits
	const maxSearchDepth = 1000

	r.logger.Debugf("Searching for common ancestor starting from contract tip %d", contractTipHeight)

	for depth := uint32(0); depth < maxSearchDepth && contractTipHeight >= depth; depth++ {
		height := contractTipHeight - depth

		// First, check if the block exists in the BTC cache
		cachedBlock := r.btcCache.FindBlock(height)
		if cachedBlock == nil {
			// Block not in cache - we need to query the BTC node directly
			btcBlock, _, err := r.btcClient.GetBlockByHeight(height)
			if err != nil {
				r.logger.Debugf("Failed to get BTC block at height %d: %v", height, err)

				continue
			}
			if btcBlock == nil {
				r.logger.Debugf("BTC node doesn't have block at height %d", height)

				continue
			}
			// Use the BTC node's block hash
			btcHash := btcBlock.BlockHash()
			contains, err := r.backend.ContainsBlock(ctx, &btcHash, height)
			if err != nil {
				r.logger.Warnf("Failed to check if contract contains block at height %d: %v", height, err)

				continue
			}
			if contains {
				r.logger.Infof("Found common ancestor at height %d (from BTC node query)", height)

				return height, nil
			}
		} else {
			// Block is in cache - check if it matches the contract
			cachedHash := cachedBlock.BlockHash()
			contains, err := r.backend.ContainsBlock(ctx, &cachedHash, height)
			if err != nil {
				r.logger.Warnf("Failed to check if contract contains block at height %d: %v", height, err)

				continue
			}
			if contains {
				r.logger.Debugf("Found common ancestor at height %d (from cache)", height)

				return height, nil
			}
		}

		r.logger.Debugf("No match at height %d, continuing search...", height)
	}

	return 0, fmt.Errorf("could not find common ancestor within %d blocks of contract tip %d", maxSearchDepth, contractTipHeight)
}

func (r *Reporter) bootstrap() error {
	var (
		btcLatestBlockHeight uint64
		ibs                  []*types.IndexedBlock
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

	// flag to indicate if we should clean up, err happened
	success := false
	defer func() {
		// cleanup func in case we error, prevents deadlocks
		if !success {
			r.bootstrapMutex.Lock()
			r.bootstrapInProgress = false
			r.bootstrapMutex.Unlock()
			r.bootstrapWg.Done()
		}
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

	// Get signer address (only needed for Babylon backend checkpoint processing)
	var signer string
	if r.babylonClient != nil {
		signer = r.babylonClient.MustGetAddr()
	}

	r.logger.Infof("BTC height: %d. Backend height: %d. Start syncing from height %d.",
		btcLatestBlockHeight, consistencyInfo.bbnLatestBlockHeight, consistencyInfo.startSyncHeight)

	// Check if there are any new blocks to sync
	// If backend is already at BTC tip, there's nothing to sync
	btcCacheTip := r.btcCache.Tip()
	if btcCacheTip != nil && consistencyInfo.startSyncHeight > btcCacheTip.Height {
		r.logger.Infof("Backend is already in sync with BTC (both at height %d), no new headers to submit",
			btcCacheTip.Height)
	} else {
		// Get blocks from cache starting from the sync height
		ibs, err = r.btcCache.GetLastBlocks(consistencyInfo.startSyncHeight)
		if err != nil {
			panic(err)
		}

		// extracts and submits headers for each block in ibs
		// Note: As we are retrieving blocks from btc cache from block just after confirmed block which
		// we already checked for consistency, we can be sure that even if rest of the block headers is different than in Babylon
		// due to reorg, our fork will be better than the one in Babylon.
		if _, err = r.ProcessHeaders(signer, ibs); err != nil {
			// this can happen when there are two contentious vigilantes or if our btc node is behind.
			r.logger.Errorf("Failed to submit headers: %v", err)
			// returning error as it is up to the caller to decide what do next
			return err
		}
	}

	// trim cache to the latest k+w blocks on BTC (which are same as in BBN)
	maxEntries := r.btcConfirmationDepth + r.checkpointFinalizationTimeout
	if err = r.btcCache.Resize(maxEntries); err != nil {
		r.logger.Errorf("Failed to resize BTC cache: %v", err)
		panic(err)
	}
	r.btcCache.Trim()

	r.logger.Infof("Size of the BTC cache: %d", r.btcCache.Size())

	// fetch k+w blocks from cache and submit checkpoints
	ibs = r.btcCache.GetAllBlocks()
	go func() {
		defer func() {
			r.bootstrapMutex.Lock()
			r.bootstrapInProgress = false
			r.bootstrapMutex.Unlock()
			r.bootstrapWg.Done()
		}()
		r.logger.Infof("Async processing checkpoints started")
		_, _ = r.ProcessCheckpoints(signer, ibs)
	}()

	r.logger.Info("Successfully finished bootstrapping")

	success = true

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
// where T is the height of the latest block in the backend's BTC header chain
func (r *Reporter) initBTCCache() error {
	var (
		err                      error
		backendLatestBlockHeight uint32
		backendBaseHeight        uint32
		baseHeight               uint32
		ibs                      []*types.IndexedBlock
	)

	r.btcCache, err = types.NewBTCCache(r.cfg.BTCCacheSize) // TODO: give an option to be unsized
	if err != nil {
		panic(err)
	}

	// Get T, i.e., total block count in backend's BTC header chain
	backendLatestBlockHeight, _, err = r.backend.GetTip(context.Background())
	if err != nil {
		return err
	}

	// Find the base height
	// For BabylonBackend: query BTCBaseHeader since Babylon prunes old blocks
	// For EthereumBackend: use calculated value since contract stores all blocks from genesis
	if babylonBackend, ok := r.backend.(*BabylonBackend); ok {
		baseRes, err := babylonBackend.client.BTCBaseHeader()
		if err != nil {
			return err
		}
		backendBaseHeight = baseRes.Header.Height
	} else {
		// For non-Babylon backends (e.g., Ethereum), calculate base height
		// These backends don't prune, so we can go back k+w blocks or to genesis
		if backendLatestBlockHeight > r.btcConfirmationDepth+r.checkpointFinalizationTimeout {
			backendBaseHeight = backendLatestBlockHeight - r.btcConfirmationDepth - r.checkpointFinalizationTimeout + 1
		} else {
			backendBaseHeight = 0 // Go back to genesis if we haven't reached k+w blocks yet
		}
	}

	// Fetch block since `baseHeight = T - k - w` from BTC, where
	// - T is total block count in backend's BTC header chain
	// - k is btcConfirmationDepth
	// - w is checkpointFinalizationTimeout
	if backendLatestBlockHeight > backendBaseHeight+r.btcConfirmationDepth+r.checkpointFinalizationTimeout {
		baseHeight = backendLatestBlockHeight - r.btcConfirmationDepth - r.checkpointFinalizationTimeout + 1
	} else {
		baseHeight = backendBaseHeight
	}

	ibs, err = r.btcClient.FindTailBlocksByHeight(baseHeight)
	if err != nil {
		panic(err)
	}

	if err = r.btcCache.Init(ibs); err != nil {
		panic(err)
	}

	return nil
}

// waitUntilBTCSync waits for BTC to synchronize until BTC is no shorter than the backend's BTC light client.
// It returns BTC last block hash, BTC last block height, and backend's base height.
func (r *Reporter) waitUntilBTCSync() error {
	var (
		btcLatestBlockHeight     uint32
		backendLatestBlockHash   *chainhash.Hash
		backendLatestBlockHeight uint32
		err                      error
	)

	// Retrieve hash/height of the latest block in BTC
	btcLatestBlockHeight, err = r.btcClient.GetBestBlock()
	if err != nil {
		return err
	}
	r.logger.Infof("BTC latest block hash and height: (%d)", btcLatestBlockHeight)

	// TODO: if BTC falls behind backend's base header, then the vigilante is incorrectly configured and should panic

	// Retrieve hash/height of the latest block in backend's header chain
	backendLatestBlockHeight, backendLatestBlockHash, err = r.backend.GetTip(context.Background())
	if err != nil {
		return err
	}
	r.logger.Infof("Backend header chain latest block hash and height: (%v, %d)", backendLatestBlockHash, backendLatestBlockHeight)

	// If BTC chain is shorter than backend's header chain, pause until BTC catches up
	if btcLatestBlockHeight == 0 || btcLatestBlockHeight < backendLatestBlockHeight {
		r.logger.Infof("BTC chain (length %d) falls behind backend header chain (length %d), wait until BTC catches up", btcLatestBlockHeight, backendLatestBlockHeight)

		// periodically check if BTC catches up with backend.
		// When BTC catches up, break and continue the bootstrapping process
		ticker := time.NewTicker(5 * time.Second) // TODO: parameterise the polling interval
		for range ticker.C {
			btcLatestBlockHeight, err = r.btcClient.GetBestBlock()
			if err != nil {
				return err
			}
			backendLatestBlockHeight, _, err = r.backend.GetTip(context.Background())
			if err != nil {
				return err
			}
			if btcLatestBlockHeight > 0 && btcLatestBlockHeight >= backendLatestBlockHeight {
				r.logger.Infof("BTC chain (length %d) now catches up with backend header chain (length %d), continue bootstrapping", btcLatestBlockHeight, backendLatestBlockHeight)

				break
			}
			r.logger.Infof("BTC chain (length %d) still falls behind backend header chain (length %d), keep waiting", btcLatestBlockHeight, backendLatestBlockHeight)
		}
	}

	return nil
}

func (r *Reporter) checkHeaderConsistency(consistencyCheckHeight uint32) error {
	var err error

	consistencyCheckBlock := r.btcCache.FindBlock(consistencyCheckHeight)
	if consistencyCheckBlock == nil {
		err = fmt.Errorf("cannot find the %d-th block of backend header chain in BTC cache for initial consistency check", consistencyCheckHeight)
		panic(err)
	}
	consistencyCheckHash := consistencyCheckBlock.BlockHash()

	r.logger.Debugf("block for consistency check: height %d, hash %v", consistencyCheckHeight, consistencyCheckHash)

	// Given that two consecutive BTC headers are chained via hash functions,
	// generating a header that can be in two different positions in two different BTC header chains
	// is as hard as breaking the hash function.
	// So as long as the block exists in the backend, it has to be at the same position in the backend as well.
	contains, err := r.backend.ContainsBlock(context.Background(), &consistencyCheckHash, consistencyCheckHeight)
	if err != nil {
		return err
	}
	if !contains {
		err = fmt.Errorf("BTC main chain is inconsistent with backend header chain: k-deep block in backend header chain: %v", consistencyCheckHash)
		panic(err)
	}

	return nil
}
