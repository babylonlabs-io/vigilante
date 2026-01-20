package reporter

import (
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/babylonlabs-io/vigilante/version"

	"github.com/babylonlabs-io/vigilante/retrywrap"
	notifier "github.com/lightningnetwork/lnd/chainntnfs"

	"github.com/avast/retry-go/v4"
	"github.com/babylonlabs-io/babylon/v4/btctxformatter"
	btcctypes "github.com/babylonlabs-io/babylon/v4/x/btccheckpoint/types"
	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/types"
	"go.uber.org/zap"
)

// confirmationTracker tracks an in-flight header batch submission
// waiting for ETH confirmation in a background goroutine.
type confirmationTracker struct {
	blocks    []*types.IndexedBlock
	txHash    string
	startTime time.Time
	done      chan error
}

type Reporter struct {
	cfg    *config.ReporterConfig
	logger *zap.SugaredLogger

	btcClient     btcclient.BTCClient
	backend       Backend       // Abstraction for header submission (Babylon, Ethereum, etc.)
	babylonClient BabylonClient // Still needed for checkpoint operations
	btcNotifier   notifier.ChainNotifier

	// retry attributes
	retrySleepTime    time.Duration
	maxRetrySleepTime time.Duration
	maxRetryTimes     uint

	// Internal states of the reporter
	checkpointCache               *types.CheckpointCache
	btcCache                      *types.BTCCache
	btcConfirmationDepth          uint32
	checkpointFinalizationTimeout uint32
	metrics                       *metrics.ReporterMetrics
	wg                            sync.WaitGroup
	started                       bool
	quit                          chan struct{}
	quitMu                        sync.Mutex

	bootstrapMutex      sync.Mutex
	bootstrapInProgress bool
	// bootstrapWg is used to wait for the bootstrap process to finish
	// bootstrapWg is incremented in bootstrap()
	// bootstrapWg is waited in bootstrapWithRetries()
	bootstrapWg sync.WaitGroup

	// pendingBlocks channel for async header submission (ETH backend only)
	// Blocks are queued here and processed by headerSubmissionWorker
	pendingBlocks chan *types.IndexedBlock
}

func New(
	cfg *config.ReporterConfig,
	parentLogger *zap.Logger,
	btcClient btcclient.BTCClient,
	backend Backend,
	babylonClient BabylonClient, // Can be nil for ETH backend
	btcNotifier notifier.ChainNotifier,
	retrySleepTime,
	maxRetrySleepTime time.Duration,
	maxRetryTimes uint,
	metrics *metrics.ReporterMetrics,
) (*Reporter, error) {
	logger := parentLogger.With(zap.String("module", "reporter")).Sugar()

	var (
		k             uint32
		w             uint32
		checkpointTag []byte
		err           error
	)

	if cfg.BackendType == config.BackendTypeEthereum {
		// Use config values (no Babylon client needed)
		k = cfg.BTCConfirmationDepth
		w = cfg.CheckpointFinalizationTimeout
		checkpointTag, err = hex.DecodeString(cfg.CheckpointTag)
		if err != nil {
			return nil, fmt.Errorf("failed to decode checkpoint tag from config: %w", err)
		}
		logger.Infof("Using config BTCCheckpoint params: (k, w, tag) = (%d, %d, %s)", k, w, cfg.CheckpointTag)
	} else {
		// Query from Babylon chain
		if babylonClient == nil {
			return nil, fmt.Errorf("babylonClient is required for Babylon backend")
		}
		var btccParamsRes *btcctypes.QueryParamsResponse
		err = retrywrap.Do(func() error {
			btccParamsRes, err = babylonClient.BTCCheckpointParams()

			return err
		},
			retry.Delay(retrySleepTime),
			retry.MaxDelay(maxRetrySleepTime),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to get BTC Checkpoint parameters: %w", err)
		}
		k = btccParamsRes.Params.BtcConfirmationDepth
		w = btccParamsRes.Params.CheckpointFinalizationTimeout
		checkpointTag, err = hex.DecodeString(btccParamsRes.Params.CheckpointTag)
		if err != nil {
			return nil, fmt.Errorf("failed to decode checkpoint tag: %w", err)
		}
		logger.Infof("BTCCheckpoint parameters: (k, w, tag) = (%d, %d, %s)", k, w, checkpointTag)
	}

	// Note that BTC cache is initialised only after bootstrapping
	ckptCache := types.NewCheckpointCache(checkpointTag, btctxformatter.CurrentVersion)

	return &Reporter{
		cfg:                           cfg,
		logger:                        logger,
		retrySleepTime:                retrySleepTime,
		maxRetrySleepTime:             maxRetrySleepTime,
		maxRetryTimes:                 maxRetryTimes,
		btcClient:                     btcClient,
		backend:                       backend,
		babylonClient:                 babylonClient,
		btcNotifier:                   btcNotifier,
		checkpointCache:               ckptCache,
		btcConfirmationDepth:          k,
		checkpointFinalizationTimeout: w,
		metrics:                       metrics,
		quit:                          make(chan struct{}),
		pendingBlocks:                 make(chan *types.IndexedBlock, 100), // Buffer for ~16 hours of BTC blocks
	}, nil
}

// Start starts the goroutines necessary to manage a vigilante.
func (r *Reporter) Start() {
	commit, _ := version.CommitInfo()
	r.logger.Infof("version: %s, commit: %s", version.Version(), commit)

	r.quitMu.Lock()
	select {
	case <-r.quit:
		// Restart the vigilante goroutines after shutdown finishes.
		r.WaitForShutdown()
		r.quit = make(chan struct{})
	default:
		// Ignore when the vigilante is still running.
		if r.started {
			r.quitMu.Unlock()

			return
		}
		r.started = true
	}
	r.quitMu.Unlock()

	r.bootstrapWithRetries()

	if err := r.btcNotifier.Start(); err != nil {
		r.logger.Errorf("Failed starting notifier")

		return
	}

	blockNotifier, err := r.btcNotifier.RegisterBlockEpochNtfn(nil)
	if err != nil {
		r.logger.Errorf("Failed registering block epoch notifier")

		return
	}

	r.wg.Add(1)
	go r.blockEventHandler(blockNotifier)

	// Start header submission worker for ETH backend (async batching)
	if _, ok := r.backend.(*EthereumBackend); ok {
		r.wg.Add(1)
		go r.headerSubmissionWorker()
	}

	go r.checkpointCache.StartCleanupRoutine(r.quit, time.Hour, 24*time.Hour)

	// start record time-related metrics
	r.metrics.RecordMetrics()

	r.logger.Infof("Successfully started the vigilant reporter")
}

// quitChan atomically reads the quit channel.
func (r *Reporter) quitChan() <-chan struct{} {
	r.quitMu.Lock()
	c := r.quit
	r.quitMu.Unlock()

	return c
}

// Stop signals all vigilante goroutines to shutdown.
func (r *Reporter) Stop() {
	r.quitMu.Lock()
	quit := r.quit
	r.quitMu.Unlock()

	select {
	case <-quit:
	default:
		// closing the `quit` channel will trigger all select case `<-quit`,
		// and thus making all handler routines to break the for loop.
		close(quit)
	}
}

// ShuttingDown returns whether the vigilante is currently in the process of shutting down or not.
func (r *Reporter) ShuttingDown() bool {
	select {
	case <-r.quitChan():
		return true
	default:
		return false
	}
}

// WaitForShutdown blocks until all vigilante goroutines have finished executing.
func (r *Reporter) WaitForShutdown() {
	// TODO: let Babylon client WaitForShutDown
	r.wg.Wait()
}

// headerSubmissionWorker processes pending blocks in batches for ETH backend.
// This decouples block notification from ETH submission to avoid blocking on slow confirmations.
func (r *Reporter) headerSubmissionWorker() {
	defer r.wg.Done()
	quit := r.quitChan()

	// Get config from ETH backend
	ethBackend, ok := r.backend.(*EthereumBackend)
	if !ok {
		r.logger.Error("headerSubmissionWorker called with non-Ethereum backend")

		return
	}
	ethCfg := ethBackend.GetConfig()
	batchSize := int(ethCfg.HeaderBatchSize)
	batchTimeout := ethCfg.HeaderBatchTimeout

	r.logger.Infof("Header submission worker started (batch_size=%d, batch_timeout=%v)",
		batchSize, batchTimeout)

	batchTimer := time.NewTimer(batchTimeout)
	defer batchTimer.Stop()

	var batch []*types.IndexedBlock
	var pendingConfirm *confirmationTracker

	for {
		select {
		case block := <-r.pendingBlocks:
			batch = append(batch, block)
			// Only submit if batch is large enough AND no pending confirmation
			if len(batch) >= batchSize && pendingConfirm == nil {
				pendingConfirm = r.submitHeaderBatchAsync(batch)
				batch = nil
				batchTimer.Reset(batchTimeout)
			}

		case <-batchTimer.C:
			// Submit whatever we have after timeout, only if no pending confirmation
			if len(batch) > 0 && pendingConfirm == nil {
				pendingConfirm = r.submitHeaderBatchAsync(batch)
				batch = nil
			}
			batchTimer.Reset(batchTimeout)

		case err := <-r.confirmationChan(pendingConfirm):
			// Confirmation result received from background goroutine
			if err != nil {
				r.logger.Warnf("Header batch failed (heights %d to %d): %v, triggering bootstrap",
					pendingConfirm.blocks[0].Height, pendingConfirm.blocks[len(pendingConfirm.blocks)-1].Height, err)
				r.bootstrapWithRetries()
				// Clear any pending batch since bootstrap will resync
				batch = nil
			} else {
				r.logger.Infof("Header batch confirmed (heights %d to %d) in %v",
					pendingConfirm.blocks[0].Height, pendingConfirm.blocks[len(pendingConfirm.blocks)-1].Height,
					time.Since(pendingConfirm.startTime))
			}
			pendingConfirm = nil

		case <-quit:
			r.drainAndShutdown(batch, pendingConfirm)
			return
		}
	}
}

// submitHeaderBatch submits a batch of blocks to the ETH backend synchronously.
// Used during shutdown to ensure remaining blocks are submitted.
func (r *Reporter) submitHeaderBatch(blocks []*types.IndexedBlock) {
	if len(blocks) == 0 {
		return
	}

	r.logger.Infof("Submitting header batch (sync): %d blocks (heights %d to %d)",
		len(blocks), blocks[0].Height, blocks[len(blocks)-1].Height)

	// ProcessHeaders ignores signer param for ETH backend
	if _, err := r.ProcessHeaders("", blocks); err != nil {
		r.logger.Warnf("Failed to submit header batch: %v, triggering bootstrap", err)
		r.bootstrapWithRetries()
	}
}

// submitHeaderBatchAsync submits a batch of blocks asynchronously, returning a tracker
// that can be used to wait for the confirmation result.
func (r *Reporter) submitHeaderBatchAsync(blocks []*types.IndexedBlock) *confirmationTracker {
	if len(blocks) == 0 {
		return nil
	}

	tracker := &confirmationTracker{
		blocks:    blocks,
		startTime: time.Now(),
		done:      make(chan error, 1),
	}

	r.logger.Infof("Submitting header batch (async): %d blocks (heights %d to %d)",
		len(blocks), blocks[0].Height, blocks[len(blocks)-1].Height)

	go func() {
		// ProcessHeaders ignores signer param for ETH backend
		_, err := r.ProcessHeaders("", blocks)
		tracker.done <- err
	}()

	return tracker
}

// confirmationChan returns the done channel from the tracker, or nil if tracker is nil.
// A nil channel blocks forever in select, effectively disabling that case.
func (r *Reporter) confirmationChan(tracker *confirmationTracker) <-chan error {
	if tracker == nil {
		return nil
	}
	return tracker.done
}

// drainAndShutdown handles graceful shutdown of the header submission worker.
// It waits for any pending confirmation and submits remaining batched blocks.
func (r *Reporter) drainAndShutdown(batch []*types.IndexedBlock, pending *confirmationTracker) {
	if pending != nil {
		r.logger.Info("Waiting for pending header confirmation before shutdown...")
		select {
		case err := <-pending.done:
			if err != nil {
				r.logger.Warnf("Pending header batch failed during shutdown: %v", err)
			} else {
				r.logger.Infof("Pending header batch confirmed (heights %d to %d)",
					pending.blocks[0].Height, pending.blocks[len(pending.blocks)-1].Height)
			}
		case <-time.After(30 * time.Second):
			r.logger.Warn("Timeout waiting for pending header batch confirmation")
		}
	}

	if len(batch) > 0 {
		r.logger.Infof("Submitting remaining %d blocks before shutdown", len(batch))
		r.submitHeaderBatch(batch)
	}

	r.logger.Info("Header submission worker stopped")
}
