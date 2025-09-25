package monitor

import (
	"github.com/avast/retry-go/v4"
	btcstakingtypes "github.com/babylonlabs-io/babylon/v3/x/btcstaking/types"
	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"go.uber.org/zap"
	"sync"
	"time"
)

type ActivationTracking struct {
	StakingHash    chainhash.Hash
	FirstSeenKDeep time.Time
	LastChecked    time.Time
	HasAlerted     bool
}

type ActivationUnbondingMonitor struct {
	babylonClient     BabylonBTCStakingClient
	btcClient         btcclient.BTCClient
	cfg               *config.BTCStakingTrackerConfig
	activationTracker map[chainhash.Hash]*ActivationTracking
	logger            *zap.SugaredLogger
	mu                sync.RWMutex
}

func NewActivationUnbondingMonitor(babylonClient BabylonBTCStakingClient, btcClient btcclient.BTCClient, cfg *config.BTCStakingTrackerConfig, logger *zap.SugaredLogger) *ActivationUnbondingMonitor {
	return &ActivationUnbondingMonitor{
		babylonClient:     babylonClient,
		btcClient:         btcClient,
		cfg:               cfg,
		activationTracker: make(map[chainhash.Hash]*ActivationTracking),
		logger:            logger,
	}
}

func (m *ActivationUnbondingMonitor) GetDelegationsByStatus(status btcstakingtypes.BTCDelegationStatus) ([]Delegation, error) {
	cursor := []byte(nil)
	var allDelegations []Delegation
	for {
		del, nextCursor, err := m.babylonClient.DelegationsByStatus(status, cursor, 100)
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

func (m *ActivationUnbondingMonitor) GetActiveDelegations() ([]Delegation, error) {
	cursor := []byte(nil)
	var allDelegations []Delegation
	for {
		del, nextCursor, err := m.babylonClient.DelegationsByStatus(
			btcstakingtypes.BTCDelegationStatus_ACTIVE,
			cursor, 100,
		)
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
	verifiedDels, err := m.GetVerifiedDelegations()
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, verifiedDel := range verifiedDels {
		stakingTxHash := verifiedDel.StakingTx.TxHash()
		kDeep, err := m.CheckKDeepConfirmation(&verifiedDel)
		if err != nil {
			m.logger.Warnf("Error checking K-deep for %s: %v", stakingTxHash, err)
			continue
		}

		if kDeep {
			if tracker, exists := m.activationTracker[stakingTxHash]; exists {
				// need to now start the timing
				timeSinceKDeep := time.Now().Sub(tracker.FirstSeenKDeep)

				if timeSinceKDeep > 30*time.Minute && !tracker.HasAlerted {
					//to do trigger alert
					tracker.HasAlerted = true
				}

				tracker.LastChecked = time.Now()
			} else {
				m.activationTracker[stakingTxHash] = &ActivationTracking{
					FirstSeenKDeep: time.Now(),
					LastChecked:    time.Now(),
					HasAlerted:     false}
			}
		} else {
			if _, exists := m.activationTracker[stakingTxHash]; exists {
				delete(m.activationTracker, stakingTxHash)
			}
		}
	}

	for hash, tracker := range m.activationTracker {
		del, err := m.GetDelegationByHash(hash.String())
		if err != nil {
			m.logger.Warnf("Error getting delegation %s: %v", hash, err)
			continue
		}

		if del.Status == btcstakingtypes.BTCDelegationStatus_ACTIVE.String() {
			delete(m.activationTracker, hash)
		} else if time.Since(tracker.LastChecked) > 24*time.Hour {
			delete(m.activationTracker, hash)
		}
	}
	return nil
}
