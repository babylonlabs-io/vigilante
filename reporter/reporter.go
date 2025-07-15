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
	"github.com/babylonlabs-io/babylon/v3/btctxformatter"
	btcctypes "github.com/babylonlabs-io/babylon/v3/x/btccheckpoint/types"
	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/types"
	"go.uber.org/zap"
)

type Reporter struct {
	cfg    *config.ReporterConfig
	logger *zap.SugaredLogger

	btcClient     btcclient.BTCClient
	babylonClient BabylonClient
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
}

func New(
	cfg *config.ReporterConfig,
	parentLogger *zap.Logger,
	btcClient btcclient.BTCClient,
	babylonClient BabylonClient,
	btcNotifier notifier.ChainNotifier,
	retrySleepTime,
	maxRetrySleepTime time.Duration,
	maxRetryTimes uint,
	metrics *metrics.ReporterMetrics,
) (*Reporter, error) {
	logger := parentLogger.With(zap.String("module", "reporter")).Sugar()
	// retrieve k and w within btccParams
	var (
		btccParamsRes *btcctypes.QueryParamsResponse
		err           error
	)
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
	k := btccParamsRes.Params.BtcConfirmationDepth
	w := btccParamsRes.Params.CheckpointFinalizationTimeout
	// get checkpoint tag
	checkpointTag, err := hex.DecodeString(btccParamsRes.Params.CheckpointTag)
	if err != nil {
		return nil, fmt.Errorf("failed to decode checkpoint tag: %w", err)
	}
	logger.Infof("BTCCheckpoint parameters: (k, w, tag) = (%d, %d, %s)", k, w, checkpointTag)

	// Note that BTC cache is initialised only after bootstrapping
	ckptCache := types.NewCheckpointCache(checkpointTag, btctxformatter.CurrentVersion)

	return &Reporter{
		cfg:                           cfg,
		logger:                        logger,
		retrySleepTime:                retrySleepTime,
		maxRetrySleepTime:             maxRetrySleepTime,
		maxRetryTimes:                 maxRetryTimes,
		btcClient:                     btcClient,
		babylonClient:                 babylonClient,
		btcNotifier:                   btcNotifier,
		checkpointCache:               ckptCache,
		btcConfirmationDepth:          k,
		checkpointFinalizationTimeout: w,
		metrics:                       metrics,
		quit:                          make(chan struct{}),
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
