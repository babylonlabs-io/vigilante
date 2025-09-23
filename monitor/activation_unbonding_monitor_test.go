package monitor

import (
	btcstakingtypes "github.com/babylonlabs-io/babylon/v3/x/btcstaking/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestActivationUnbondingMonitor(t *testing.T) {
	t.Parallel()
	ctl := gomock.NewController(t)
	mockClient := NewMockBabylonBTCStakingClient(ctl)
	mockClient.EXPECT().BTCDelegations(gomock.Any(),
		gomock.Any()).Return(&btcstakingtypes.QueryBTCDelegationsResponse{},
		nil).AnyTimes()

	mockClient.EXPECT().BTCDelegation(gomock.Any()).Return(&btcstakingtypes.
	QueryBTCDelegationResponse{},
		nil).AnyTimes()

	monitor := NewActivationUnbondingMonitor(mockClient)

	actDels, err := monitor.GetActiveDelegations()
	require.NoError(t, err)
	require.NotNil(t, actDels)

	verDels, err := monitor.GetVerifiedDelegations()
	require.NoError(t, err)
	require.NotNil(t, verDels)

	hashDels, err := monitor.GetDelegationByHash("test")
	require.NoError(t, err)
	require.NotNil(t, hashDels)
}
