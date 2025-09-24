package monitor

import (
	"github.com/avast/retry-go/v4"
	btcstakingtypes "github.com/babylonlabs-io/babylon/v3/x/btcstaking/types"
	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/lightningnetwork/lnd/chainntnfs"
	"time"
)

type ActivationUnbondingMonitor struct {
	babylonClient BabylonAdaptorClient
	btcClient     btcclient.BTCClient
	cfg           *config.BTCStakingTrackerConfig
}

func NewActivationUnbondingMonitor(babylonClient BabylonAdaptorClient, btcClient btcclient.BTCClient, cfg *config.BTCStakingTrackerConfig) *ActivationUnbondingMonitor {
	return &ActivationUnbondingMonitor{
		babylonClient: babylonClient,
		btcClient:     btcClient,
		cfg:           cfg,
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
