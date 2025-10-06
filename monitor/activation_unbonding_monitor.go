package monitor

import (
	"context"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"sync"
	"time"

	"github.com/avast/retry-go/v4"
	btcstakingtypes "github.com/babylonlabs-io/babylon/v3/x/btcstaking/types"
	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/btcstaking-tracker/stakingeventwatcher"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/lightningnetwork/lnd/chainntnfs"
	"go.uber.org/zap"
)

const (
	maxConcurrentRequests = 1000
)

type ActivationTracking struct {
	StakingHash chainhash.Hash
	KDeepAt     time.Time
	LastChecked time.Time
	HasAlerted  bool
}

type UnbondingTracking struct {
	StakingHash      chainhash.Hash
	SpendingTxHash   chainhash.Hash
	SpendingKDeepAt  time.Time
	LastChecked      time.Time
	HasAlerted       bool
}

type ActivationUnbondingMonitor struct {
	babylonClient     BabylonAdaptorClient
	btcClient         btcclient.BTCClient
	cfg               *config.MonitorConfig
	indexer           stakingeventwatcher.SpendChecker
	activationTracker map[chainhash.Hash]*ActivationTracking
	unbondingTracker  map[chainhash.Hash]*UnbondingTracking
	logger            *zap.Logger
	mu                sync.RWMutex
	metrics           *metrics.ActivationUnbondingMonitorMetrics
	quit              chan struct{}
	wg                sync.WaitGroup
}

func NewActivationUnbondingMonitor(babylonClient BabylonAdaptorClient,
	btcClient btcclient.BTCClient,
	indexer stakingeventwatcher.SpendChecker,
	cfg *config.MonitorConfig,
	logger *zap.Logger, monitorMetrics *metrics.
		ActivationUnbondingMonitorMetrics) *ActivationUnbondingMonitor {
	return &ActivationUnbondingMonitor{
		babylonClient:     babylonClient,
		btcClient:         btcClient,
		cfg:               cfg,
		indexer:           indexer,
		activationTracker: make(map[chainhash.Hash]*ActivationTracking),
		unbondingTracker:  make(map[chainhash.Hash]*UnbondingTracking),
		logger:            logger,
		metrics:           monitorMetrics,
		quit:              make(chan struct{}),
	}
}

func (m *ActivationUnbondingMonitor) GetDelegationsByStatus(status btcstakingtypes.BTCDelegationStatus) ([]Delegation, error) {
	cursor := []byte(nil)
	var allDelegations []Delegation
	for {
		del, nextCursor, err := m.babylonClient.DelegationsByStatus(status, cursor, 1000)
		if err != nil {
			return nil, err
		}

		allDelegations = append(allDelegations, del...)

		if nextCursor == nil {
			break
		}
		cursor = nextCursor
	}

	return allDelegations, nil
}

func (m *ActivationUnbondingMonitor) GetDelegationByHash(hash string) (*Delegation, error) {
	return m.babylonClient.BTCDelegation(hash)
}

func (m *ActivationUnbondingMonitor) CheckKDeepConfirmation(delegation *Delegation) (bool, error) {
	outputScript := delegation.StakingTx.TxOut[delegation.StakingOutputIdx].PkScript
	stakingTxHash := delegation.StakingTx.TxHash()

	var details *chainntnfs.TxConfirmation
	var status btcclient.TxStatus

	err := retry.Do(func() error {
		var err error
		details, status, err = m.btcClient.TxDetails(&stakingTxHash, outputScript)

		return err
	},
		retry.Attempts(3),
		retry.Delay(1*time.Second))

	if err != nil {
		return false, err
	}

	if status != btcclient.TxInChain {
		return false, nil
	}

	currentHeight, err := m.btcClient.GetBestBlock()
	if err != nil {
		return false, err
	}

	bbnDepth, err := m.babylonClient.GetConfirmationDepth()
	if err != nil {
		return false, err
	}

	confirmations := currentHeight - details.BlockHeight

	return confirmations >= bbnDepth, nil
}

func (m *ActivationUnbondingMonitor) CheckActivationTiming() error {
	cursor := []byte(nil)
	batchSize := uint64(1000)

	for {
		batch, nextCursor, err := m.babylonClient.DelegationsByStatus(
			btcstakingtypes.BTCDelegationStatus_VERIFIED,
			cursor,
			batchSize,
		)
		if err != nil {
			return err
		}

		err = m.processDelegations(batch)
		if err != nil {
			return err
		}

		if nextCursor == nil {
			break
		}
		cursor = nextCursor
	}

	m.mu.Lock()
	m.cleanupTrackedDelegations()
	m.mu.Unlock()

	return nil
}

func (m *ActivationUnbondingMonitor) handleKDeepDel(stakingTxHash chainhash.Hash) {
	if tracker, exists := m.activationTracker[stakingTxHash]; exists {
		// need to now start the timing
		timeSinceKDeep := time.Since(tracker.KDeepAt)
		activationTimeout := time.Duration(m.cfg.ActivationTimeoutSeconds) * time.
			Second

		if timeSinceKDeep > activationTimeout && !tracker.HasAlerted {
			m.logger.Error("Delegation activation timeout detected",
				zap.String("delegationID", stakingTxHash.String()),
				zap.Duration("timeSinceKDeep", timeSinceKDeep),
				zap.Duration("timeout", activationTimeout))
			m.metrics.ActivationTimeoutsCounter.WithLabelValues(stakingTxHash.String()).Inc()
			tracker.HasAlerted = true
		}

		tracker.LastChecked = time.Now()
	} else {
		m.activationTracker[stakingTxHash] = &ActivationTracking{
			StakingHash: stakingTxHash,
			KDeepAt:     time.Now(),
			LastChecked: time.Now(),
			HasAlerted:  false,
		}
		m.metrics.TrackedActivationGauge.Inc()
	}
}

func (m *ActivationUnbondingMonitor) processDelegations(verifiedDels []Delegation) error {
	var wg errgroup.Group
	sem := semaphore.NewWeighted(maxConcurrentRequests)

	for _, verifiedDel := range verifiedDels {
		del := verifiedDel
		wg.Go(func() error {
			if err := sem.Acquire(context.Background(), 1); err != nil {
				return err
			}
			defer sem.Release(1)

			stakingTxHash := del.StakingTx.TxHash()
			kDeep, err := m.CheckKDeepConfirmation(&del)
			if err != nil {
				m.logger.Warn("Error checking K-deep", zap.String("stakingTxHash", stakingTxHash.String()), zap.Error(err))

				return err
			}

			m.mu.Lock()
			if kDeep {
				m.handleKDeepDel(stakingTxHash)
			} else {
				delete(m.activationTracker, stakingTxHash)
			}
			m.mu.Unlock()

			return nil
		})
	}

	return wg.Wait()
}

func (m *ActivationUnbondingMonitor) cleanupTrackedDelegations() {
	for hash, tracker := range m.activationTracker {
		del, err := m.GetDelegationByHash(hash.String())
		if err != nil {
			m.logger.Warn("Error getting delegation", zap.String("hash", hash.String()), zap.Error(err))

			continue
		}

		if del.Status == btcstakingtypes.BTCDelegationStatus_ACTIVE.String() {
			activationDelay := time.Since(tracker.KDeepAt)
			m.metrics.ActivationDelayHistogram.Observe(activationDelay.Seconds())
			m.logger.Info("Delegation activated", zap.String("hash", hash.String()), zap.Duration("activationDelay", activationDelay))

			delete(m.activationTracker, hash)
			m.metrics.TrackedActivationGauge.Add(-1)
		} else if time.Since(tracker.LastChecked) > 24*time.Hour {
			delete(m.activationTracker, hash)
			m.metrics.TrackedActivationGauge.Add(-1)
		}
	}
}

func (m *ActivationUnbondingMonitor) CheckUnbondingTiming() error {
	cursor := []byte(nil)
	batchSize := uint64(1000)

	for {
		delegations, nextCursor, err := m.babylonClient.DelegationsByStatus(
			btcstakingtypes.BTCDelegationStatus_ACTIVE,
			cursor,
			batchSize,
		)
		if err != nil {
			return err
		}

		if err := m.processUnbondingDels(delegations); err != nil {
			return err
		}

		if nextCursor == nil {
			break
		}
		cursor = nextCursor
	}

	m.mu.Lock()
	m.cleanupUnbondingTracker()
	m.mu.Unlock()

	return nil
}

func (m *ActivationUnbondingMonitor) processUnbondingDels(delegations []Delegation) error {
	var wg errgroup.Group
	sem := semaphore.NewWeighted(maxConcurrentRequests)

	for _, delegation := range delegations {
		del := delegation
		wg.Go(func() error {
			if err := sem.Acquire(context.Background(), 1); err != nil {
				return err
			}
			defer sem.Release(1)

			stakingHash := del.StakingTx.TxHash()

			isSpentKDeep, spendingTxHash, err := m.checkIfSpent(&del)
			if err != nil {
				m.logger.Warn("error whilst checking spent",
					zap.String("stakingHash", stakingHash.String()),
					zap.Error(err))

				return nil
			}

			m.mu.Lock()
			if isSpentKDeep && spendingTxHash != nil {
				m.handleSpentDelegation(stakingHash, *spendingTxHash)
			} else {
				delete(m.unbondingTracker, stakingHash)
			}
			m.mu.Unlock()

			return nil
		})
	}

	return wg.Wait()
}

func (m *ActivationUnbondingMonitor) checkIfSpent(del *Delegation) (bool, *chainhash.Hash, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	response, err := m.indexer.GetOutspend(ctx, del.StakingTx.TxHash().String(), del.StakingOutputIdx)
	if err != nil {
		return false, nil, err
	}

	if !response.Spent {
		return false, nil, nil
	}

	spendingTxHash, err := chainhash.NewHashFromStr(response.TxID)
	if err != nil {
		return false, nil, err
	}

	if !response.Status.Confirmed {
		return false, nil, nil
	}

	currentHeight, err := m.btcClient.GetBestBlock()
	if err != nil {
		return false, nil, err
	}

	bbnDepth, err := m.babylonClient.GetConfirmationDepth()
	if err != nil {
		return false, nil, err
	}

	if response.Status.BlockHeight < 0 {
		return false, nil, nil
	}

	if response.Status.BlockHeight > int(currentHeight) {
		return false, nil, nil
	}

	confirmations := int(currentHeight) - response.Status.BlockHeight
	if confirmations < int(bbnDepth) {
		return false, nil, nil
	}

	return true, spendingTxHash, nil
}

func (m *ActivationUnbondingMonitor) handleSpentDelegation(stakingHash chainhash.Hash, spendingTxHash chainhash.Hash) {
	if tracker, exists := m.unbondingTracker[stakingHash]; exists {
		timeSinceSpendingKDeep := time.Since(tracker.SpendingKDeepAt)
		unbondingTimeout := time.Duration(m.cfg.UnbondingTimeoutMinutes) * time.Minute

		if timeSinceSpendingKDeep > unbondingTimeout && !tracker.HasAlerted {
			m.logger.Warn("Delegation unbonding timeout detected",
				zap.String("stakingHash", stakingHash.String()),
				zap.String("spendingTxHash", spendingTxHash.String()),
				zap.Duration("timeSinceSpendingKDeep", timeSinceSpendingKDeep),
				zap.Duration("timeout", unbondingTimeout))
			m.metrics.UnbondingTimeoutsCounter.WithLabelValues(stakingHash.String()).Inc()
			tracker.HasAlerted = true
		}
		tracker.LastChecked = time.Now()
	} else {
		m.unbondingTracker[stakingHash] = &UnbondingTracking{
			StakingHash:     stakingHash,
			SpendingTxHash:  spendingTxHash,
			SpendingKDeepAt: time.Now(),
			LastChecked:     time.Now(),
			HasAlerted:      false,
		}
		m.logger.Info("Started tracking unbonding",
			zap.String("stakingHash", stakingHash.String()),
			zap.String("spendingTxHash", spendingTxHash.String()))
		m.metrics.TrackedUnbondingGauge.Inc()
	}
}

func (m *ActivationUnbondingMonitor) cleanupUnbondingTracker() {
	for hash, tracker := range m.unbondingTracker {
		del, err := m.GetDelegationByHash(hash.String())
		if err != nil {
			m.logger.Warn("Error getting delegation",
				zap.String("hash", hash.String()),
				zap.Error(err))

			continue
		}

		if del.Status == btcstakingtypes.BTCDelegationStatus_UNBONDED.String() {
			unbondingDelay := time.Since(tracker.SpendingKDeepAt)
			m.metrics.UnbondingDelayHistogram.Observe(unbondingDelay.Seconds())
			m.logger.Info("Unbonding completed",
				zap.String("hash", hash.String()),
				zap.Duration("unbondingDelay", unbondingDelay))

			delete(m.unbondingTracker, hash)
			m.metrics.TrackedUnbondingGauge.Add(-1)
		} else if time.Since(tracker.LastChecked) > 24*time.Hour {
			delete(m.unbondingTracker, hash)
			m.metrics.TrackedUnbondingGauge.Add(-1)
		}
	}
}

func (m *ActivationUnbondingMonitor) Start() error {
	m.logger.Info("Starting activation/unbonding monitor")

	m.wg.Add(1)
	go m.runMonitorLoop()

	return nil
}

func (m *ActivationUnbondingMonitor) Stop() error {
	m.logger.Info("Stopping activation/unbonding monitor")
	close(m.quit)
	m.wg.Wait()

	return nil
}

func (m *ActivationUnbondingMonitor) runMonitorLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(time.Duration(m.cfg.TimingCheckIntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			var checkWg sync.WaitGroup

			checkWg.Add(1)
			go func() {
				defer checkWg.Done()
				if err := m.CheckActivationTiming(); err != nil {
					m.logger.Error("Error checking activation timing", zap.Error(err))
					m.metrics.CheckErrorsTotal.Inc()
				}
			}()

			checkWg.Add(1)
			go func() {
				defer checkWg.Done()
				if err := m.CheckUnbondingTiming(); err != nil {
					m.logger.Error("Error checking unbonding timing", zap.Error(err))
					m.metrics.CheckErrorsTotal.Inc()
				}
			}()

			checkWg.Wait()

		case <-m.quit:
			return
		}
	}
}
