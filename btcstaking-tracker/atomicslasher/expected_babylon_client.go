package atomicslasher

import (
	"context"
	"github.com/babylonlabs-io/babylon/client/babylonclient"

	"cosmossdk.io/errors"
	bstypes "github.com/babylonlabs-io/babylon/x/btcstaking/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquerytypes "github.com/cosmos/cosmos-sdk/types/query"
)

type BabylonClient interface {
	FinalityProvider(fpBtcPkHex string) (*bstypes.QueryFinalityProviderResponse, error)
	BTCDelegations(status bstypes.BTCDelegationStatus, pagination *sdkquerytypes.PageRequest) (*bstypes.QueryBTCDelegationsResponse, error)
	BTCDelegation(stakingTxHashHex string) (*bstypes.QueryBTCDelegationResponse, error)
	BTCStakingParamsByVersion(version uint32) (*bstypes.QueryParamsByVersionResponse, error)
	ReliablySendMsg(ctx context.Context, msg sdk.Msg, expectedErrors []*errors.Error, unrecoverableErrors []*errors.Error) (*babylonclient.RelayerTxResponse, error)
	MustGetAddr() string
}
