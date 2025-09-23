package monitor

import (
	btcstakingtypes "github.com/babylonlabs-io/babylon/v3/x/btcstaking/types"
	"github.com/cosmos/cosmos-sdk/types/query"
)

type BabylonBTCStakingClient interface {
	BTCDelegations(status btcstakingtypes.BTCDelegationStatus, pageReq *query.PageRequest) (*btcstakingtypes.QueryBTCDelegationsResponse, error)
	BTCDelegation(stakingTxHash string) (*btcstakingtypes.QueryBTCDelegationResponse, error)
}
