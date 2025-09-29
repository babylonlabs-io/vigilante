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
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/lightningnetwork/lnd/chainntnfs"
	"go.uber.org/zap"
)

type ActivationTracking struct {
	StakingHash chainhash.Hash
	KDeepAt     time.Time
	LastChecked time.Time
	HasAlerted  bool
}

type ActivationUnbondingMonitor struct {
	babylonClient     BabylonAdaptorClient
	btcClient         btcclient.BTCClient
	cfg               *config.MonitorConfig
	activationTracker map[chainhash.Hash]*ActivationTracking
	logger            *zap.Logger
	mu                sync.RWMutex
	metrics           *metrics.ActivationUnbondingMonitorMetrics
}

func NewActivationUnbondingMonitor(babylonClient BabylonAdaptorClient,
	btcClient btcclient.BTCClient, cfg *config.MonitorConfig,
	logger *zap.Logger, monitorMetrics *metrics.
ActivationUnbondingMonitorMetrics) *ActivationUnbondingMonitor {
	return &ActivationUnbondingMonitor{
		babylonClient:     babylonClient,
		btcClient:         btcClient,
		cfg:               cfg,
		activationTracker: make(map[chainhash.Hash]*ActivationTracking),
		logger:            logger,
		metrics:           monitorMetrics,
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
		activationTimeout := time.Duration(m.cfg.ActivationTimeoutMinutes) * time.Minute

		if timeSinceKDeep > activationTimeout && !tracker.HasAlerted {
			//to do: trigger alert
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
	}
}

func (m *ActivationUnbondingMonitor) processDelegations(verifiedDels []Delegation) error {
	var wg errgroup.Group
	sem := semaphore.NewWeighted(1000)

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
