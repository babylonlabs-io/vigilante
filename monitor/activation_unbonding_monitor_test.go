package monitor

import (
	"github.com/babylonlabs-io/babylon/v3/testutil/datagen"
	btcstakingtypes "github.com/babylonlabs-io/babylon/v3/x/btcstaking/types"
	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/testutil/mocks"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/golang/mock/gomock"
	"github.com/lightningnetwork/lnd/chainntnfs"
	"github.com/stretchr/testify/require"
	"math/rand"
	"testing"
	"time"
)

func TestActivationUnbondingMonitor(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(time.Now().Unix()))
	ctl := gomock.NewController(t)
	cfg := config.DefaultBTCStakingTrackerConfig()
	mockClient := NewMockBabylonAdaptorClient(ctl)
	mockBtcClient := mocks.NewMockBTCClient(ctl)

	monitor := NewActivationUnbondingMonitor(mockClient, mockBtcClient, &cfg)

	expectedActivated := 5
	delegations := make([]Delegation, 0, expectedActivated)
	for i := 0; i < expectedActivated; i++ {
		stk := datagen.GenRandomTx(r)
		delegations = append(delegations, Delegation{
			StakingTx:             stk,
			StakingOutputIdx:      0,
			DelegationStartHeight: 0,
			UnbondingOutput:       nil,
			HasProof:              false,
			Status: btcstakingtypes.BTCDelegationStatus_ACTIVE.
				String(),
		})
	}

	mockClient.EXPECT().DelegationsByStatus(gomock.Any(), gomock.Any(),
		gomock.Any()).Return(delegations, nil, nil).AnyTimes()

	mockClient.EXPECT().BTCDelegation(gomock.Any()).Return(&delegations[0], nil).AnyTimes()
	mockBtcClient.EXPECT().TxDetails(gomock.Any(), gomock.Any()).Return(
		&chainntnfs.TxConfirmation{
			BlockHeight: 850000,
			BlockHash:   &chainhash.Hash{},
			TxIndex:     1,
		},
		btcclient.TxInChain,
		nil,
	).AnyTimes()

	mockBtcClient.EXPECT().GetBestBlock().Return(uint32(850006), nil).AnyTimes()
	mockClient.EXPECT().GetConfirmationDepth().Return(uint32(4), nil).AnyTimes()

	actDels, err := monitor.GetDelegationsByStatus(btcstakingtypes.BTCDelegationStatus_ACTIVE)
	require.NoError(t, err)
	require.NotNil(t, actDels)

	hashDels, err := monitor.GetDelegationByHash("test")
	require.NoError(t, err)
	require.NotNil(t, hashDels)

	for _, delegation := range delegations {
		confirmation, err := monitor.CheckKDeepConfirmation(&delegation)
		require.NoError(t, err)
		require.NotNil(t, confirmation)
		require.True(t, confirmation)
	}
}

func TestActivationUnbondingMonitor_Error(t *testing.T) {
	t.Parallel()
	ctl := gomock.NewController(t)
	cfg := config.DefaultBTCStakingTrackerConfig()
	mockClient := NewMockBabylonAdaptorClient(ctl)
	mockBtcClient := mocks.NewMockBTCClient(ctl)

	monitor := NewActivationUnbondingMonitor(mockClient, mockBtcClient, &cfg)

	t.Run("Transaction not in chain", func(t *testing.T) {
		t.Parallel()
		stk := datagen.GenRandomTx(rand.New(rand.NewSource(2)))
		delegation := Delegation{
			StakingTx:        stk,
			StakingOutputIdx: 0,
		}

		mockBtcClient.EXPECT().TxDetails(gomock.Any(), gomock.Any()).
			Return(&chainntnfs.TxConfirmation{}, btcclient.TxInMemPool, nil)

		confirmed, err := monitor.CheckKDeepConfirmation(&delegation)
		require.NoError(t, err)
		require.False(t, confirmed)
	})

	t.Run("Insufficient confirmations", func(t *testing.T) {
		t.Parallel()
		stk := datagen.GenRandomTx(rand.New(rand.NewSource(3)))
		delegation := Delegation{
			StakingTx:        stk,
			StakingOutputIdx: 0,
		}

		mockBtcClient.EXPECT().TxDetails(gomock.Any(), gomock.Any()).
			Return(&chainntnfs.TxConfirmation{BlockHeight: 850000}, btcclient.TxInChain, nil)
		mockBtcClient.EXPECT().GetBestBlock().Return(uint32(850003), nil)
		mockClient.EXPECT().GetConfirmationDepth().Return(uint32(6), nil)

		confirmed, err := monitor.CheckKDeepConfirmation(&delegation)
		require.NoError(t, err)
		require.False(t, confirmed)
	})
}
