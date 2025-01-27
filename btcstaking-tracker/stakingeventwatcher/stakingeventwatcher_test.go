package stakingeventwatcher

import (
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/testutil/mocks"
	"github.com/golang/mock/gomock"
	"github.com/lightningnetwork/lnd/chainntnfs"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/sync/semaphore"
	"math/rand"
	"testing"
	"time"
)

func TestHandlingDelegations(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(time.Now().Unix()))
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cfg := config.DefaultBTCStakingTrackerConfig()
	cfg.CheckDelegationsInterval = 1 * time.Second

	mockBTCClient := mocks.NewMockBTCClient(ctrl)
	mockBabylonNodeAdapter := NewMockBabylonNodeAdapter(ctrl)

	mockBabylonNodeAdapter.EXPECT().BtcClientTipHeight().Return(uint32(0), nil).AnyTimes()
	bsMetrics := metrics.NewBTCStakingTrackerMetrics()

	sew := StakingEventWatcher{
		logger:                          zap.NewNop().Sugar(),
		quit:                            make(chan struct{}),
		cfg:                             &cfg,
		babylonNodeAdapter:              mockBabylonNodeAdapter,
		btcClient:                       mockBTCClient,
		unbondingTracker:                NewTrackedDelegations(),
		pendingTracker:                  NewTrackedDelegations(),
		verifiedInsufficientConfTracker: NewTrackedDelegations(),
		verifiedNotInChainTracker:       NewTrackedDelegations(),
		verifiedSufficientConfTracker:   NewTrackedDelegations(),
		unbondingDelegationChan:         make(chan *newDelegation),
		unbondingRemovalChan:            make(chan *delegationInactive),
		activationLimiter:               semaphore.NewWeighted(30),
		metrics:                         bsMetrics.UnbondingWatcherMetrics,
	}

	defer close(sew.quit)

	expectedActivated := 1000
	delegations := make([]Delegation, 0, expectedActivated)
	for i := 0; i < expectedActivated; i++ {
		stk := datagen.GenRandomTx(r)
		delegations = append(delegations, Delegation{
			StakingTx:             stk,
			StakingOutputIdx:      0,
			DelegationStartHeight: 0,
			UnbondingOutput:       nil,
			HasProof:              false,
		})
	}

	mockBabylonNodeAdapter.EXPECT().
		DelegationsByStatus(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(delegations, nil).AnyTimes()

	mockBabylonNodeAdapter.EXPECT().
		ActivateDelegation(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	mockBabylonNodeAdapter.EXPECT().QueryHeaderDepth(gomock.Any()).Return(uint32(2), nil).AnyTimes()
	mockBabylonNodeAdapter.EXPECT().IsDelegationVerified(gomock.Any()).Return(true, nil).AnyTimes()

	params := BabylonParams{ConfirmationTimeBlocks: 1}
	mockBabylonNodeAdapter.EXPECT().Params().Return(&params, nil).AnyTimes()

	block, _ := datagen.GenRandomBtcdBlock(r, 10, nil)
	bh := block.BlockHash()
	details := &chainntnfs.TxConfirmation{
		BlockHash:   &bh,
		BlockHeight: 100,
		TxIndex:     1,
		Tx:          nil,
		Block:       block,
	}
	mockBTCClient.EXPECT().TxDetails(gomock.Any(), gomock.Any()).Return(details, btcclient.TxInChain, nil).AnyTimes()

	sew.wg.Add(2)
	go func() {
		go sew.fetchDelegations()
		go sew.handlerVerifiedDelegations()
	}()

	require.Eventually(t, func() bool {
		return promtestutil.ToFloat64(sew.metrics.ReportedActivateDelegationsCounter) >= float64(expectedActivated)
	}, 60*time.Second, 100*time.Millisecond)
}
