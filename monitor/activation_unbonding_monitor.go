package monitor

import (
	btcstakingtypes "github.com/babylonlabs-io/babylon/v3/x/btcstaking/types"
	"github.com/cosmos/cosmos-sdk/types/query"
)

type ActivationUnbondingMonitor struct {
	babylonClient BabylonBTCStakingClient
}

func NewActivationUnbondingMonitor(client BabylonBTCStakingClient) *ActivationUnbondingMonitor {
	return &ActivationUnbondingMonitor{
		babylonClient: client,
	}
}

func (m *ActivationUnbondingMonitor) GetVerifiedDelegations() (*btcstakingtypes.QueryBTCDelegationsResponse, error) {
	return m.babylonClient.BTCDelegations(
		btcstakingtypes.BTCDelegationStatus_VERIFIED, &query.PageRequest{Limit: 100},
	)
}

func (m *ActivationUnbondingMonitor) GetActiveDelegations() (*btcstakingtypes.QueryBTCDelegationsResponse, error) {
	return m.babylonClient.BTCDelegations(
		btcstakingtypes.BTCDelegationStatus_ACTIVE, &query.PageRequest{Limit: 100})
}

func (m *ActivationUnbondingMonitor) GetDelegationByHash(hash string) (*btcstakingtypes.QueryBTCDelegationResponse, error) {
	return m.babylonClient.BTCDelegation(hash)
}
