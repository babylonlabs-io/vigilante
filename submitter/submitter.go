package submitter

import (
	"encoding/hex"
	"fmt"
	"github.com/babylonlabs-io/vigilante/retrywrap"
	"github.com/lightningnetwork/lnd/kvdb"
	"sync"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/babylonlabs-io/babylon/btctxformatter"
	btcctypes "github.com/babylonlabs-io/babylon/x/btccheckpoint/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"go.uber.org/zap"

	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/submitter/poller"
	"github.com/babylonlabs-io/vigilante/submitter/relayer"
)

type Submitter struct {
	Cfg    *config.SubmitterConfig
	logger *zap.SugaredLogger

	relayer *relayer.Relayer
	poller  *poller.Poller

	metrics *metrics.SubmitterMetrics

	wg      sync.WaitGroup
	started bool
	quit    chan struct{}
	quitMu  sync.Mutex
}

func New(
	cfg *config.SubmitterConfig,
	parentLogger *zap.Logger,
	btcWallet btcclient.BTCWallet,
	queryClient BabylonQueryClient,
	submitterAddr sdk.AccAddress,
	retrySleepTime, maxRetrySleepTime time.Duration, maxRetryTimes uint,
	submitterMetrics *metrics.SubmitterMetrics,
	db kvdb.Backend,
	walletName string,
) (*Submitter, error) {
	logger := parentLogger.With(zap.String("module", "submitter"))
	var (
		btccheckpointParams *btcctypes.QueryParamsResponse
		err                 error
	)
	err = retrywrap.Do(func() error {
		btccheckpointParams, err = queryClient.BTCCheckpointParams()

		return err
	},
		retry.Delay(retrySleepTime),
		retry.MaxDelay(maxRetrySleepTime),
		retry.Attempts(maxRetryTimes),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get checkpoint params: %w", err)
	}

	// get checkpoint tag
	checkpointTag, err := hex.DecodeString(btccheckpointParams.Params.CheckpointTag)
	if err != nil {
		return nil, fmt.Errorf("failed to decode checkpoint tag: %w", err)
	}

	p := poller.New(queryClient, cfg.BufferSize)

	btcCfg := btcWallet.GetBTCConfig()
	est, err := relayer.NewFeeEstimator(btcCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create fee estimator: %w", err)
	}
	logger.Sugar().Infof("Successfully started fee estimator for bitcoind")

	r := relayer.New(
		btcWallet,
		walletName,
		checkpointTag,
		btctxformatter.CurrentVersion,
		submitterAddr,
		submitterMetrics.RelayerMetrics,
		est,
		cfg,
		logger,
		db,
	)

	return &Submitter{
		Cfg:     cfg,
		logger:  logger.Sugar(),
		poller:  p,
		relayer: r,
		metrics: submitterMetrics,
		quit:    make(chan struct{}),
	}, nil
}

// Start starts the goroutines necessary to manage a vigilante.
func (s *Submitter) Start() {
	s.quitMu.Lock()
	select {
	case <-s.quit:
		// Restart the vigilante goroutines after shutdown finishes.
		s.WaitForShutdown()
		s.quit = make(chan struct{})
	default:
		// Ignore when the vigilante is still running.
		if s.started {
			s.quitMu.Unlock()

			return
		}
		s.started = true
	}
	s.quitMu.Unlock()

	s.wg.Add(1)
	// TODO: implement subscriber to the raw checkpoints
	// TODO: when bootstrapping,
	// - start subscribing raw checkpoints
	// - query/forward sealed raw checkpoints to BTC
	// - keep subscribing new raw checkpoints
	go s.pollCheckpoints()
	s.wg.Add(1)
	go s.processCheckpoints()

	// start to record time-related metrics
	s.metrics.RecordMetrics()

	s.logger.Infof("Successfully created the vigilant submitter")
}

// quitChan atomically reads the quit channel.
func (s *Submitter) quitChan() <-chan struct{} {
	s.quitMu.Lock()
	c := s.quit
	s.quitMu.Unlock()

	return c
}

// Stop signals all vigilante goroutines to shutdown.
func (s *Submitter) Stop() {
	s.quitMu.Lock()
	quit := s.quit
	s.quitMu.Unlock()

	select {
	case <-quit:
	default:
		close(quit)
	}
}

// ShuttingDown returns whether the vigilante is currently in the process of
// shutting down or not.
func (s *Submitter) ShuttingDown() bool {
	select {
	case <-s.quitChan():
		return true
	default:
		return false
	}
}

// WaitForShutdown blocks until all vigilante goroutines have finished executing.
func (s *Submitter) WaitForShutdown() {
	s.wg.Wait()
}

func (s *Submitter) pollCheckpoints() {
	defer s.wg.Done()
	quit := s.quitChan()

	ticker := time.NewTicker(time.Duration(s.Cfg.PollingIntervalSeconds) * time.Second)

	for {
		select {
		case <-ticker.C:
			s.logger.Info("Polling sealed raw checkpoints...")
			err := s.poller.PollSealedCheckpoints()
			if err != nil {
				s.logger.Errorf("failed to query raw checkpoints: %v", err)

				continue
			}
			s.logger.Debugf("Next polling happens in %v seconds", s.Cfg.PollingIntervalSeconds)
		case <-quit:
			// We have been asked to stop
			return
		}
	}
}

func (s *Submitter) processCheckpoints() {
	defer s.wg.Done()
	quit := s.quitChan()

	for {
		select {
		case ckpt := <-s.poller.GetSealedCheckpointChan():
			s.logger.Infof("A sealed raw checkpoint for epoch %v is found", ckpt.Ckpt.EpochNum)
			if err := s.relayer.SendCheckpointToBTC(ckpt); err != nil {
				s.logger.Errorf("Failed to submit the raw checkpoint for %v: %v", ckpt.Ckpt.EpochNum, err)
				s.metrics.FailedCheckpointsCounter.Inc()

				continue
			}
			if err := s.relayer.MaybeResubmitSecondCheckpointTx(ckpt); err != nil {
				s.logger.Errorf("Failed to resubmit the raw checkpoint for %v: %v", ckpt.Ckpt.EpochNum, err)
				s.metrics.FailedCheckpointsCounter.Inc()
			}
			s.metrics.SecondsSinceLastCheckpointGauge.Set(0)
		case <-quit:
			// We have been asked to stop
			return
		}
	}
}

func (s *Submitter) Metrics() *metrics.SubmitterMetrics {
	return s.metrics
}
