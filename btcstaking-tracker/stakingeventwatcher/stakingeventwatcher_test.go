package stakingeventwatcher

import (
	"context"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/babylonlabs-io/babylon/v4/testutil/datagen"
	btcstakingtypes "github.com/babylonlabs-io/babylon/v4/x/btcstaking/types"
	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/testutil/mocks"
	"github.com/btcsuite/btcd/wire"
	"github.com/golang/mock/gomock"
	"github.com/lightningnetwork/lnd/chainntnfs"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/sync/semaphore"
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
			Status:                btcstakingtypes.BTCDelegationStatus_VERIFIED.String(),
		})
	}

	mockBabylonNodeAdapter.EXPECT().
		DelegationsByStatus(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(delegations, nil, nil).AnyTimes()

	mockBabylonNodeAdapter.EXPECT().
		ActivateDelegation(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	mockBabylonNodeAdapter.EXPECT().QueryHeaderDepth(gomock.Any()).Return(uint32(2), nil).AnyTimes()
	mockBabylonNodeAdapter.EXPECT().IsDelegationVerified(gomock.Any()).Return(true, nil).AnyTimes()
	mockBabylonNodeAdapter.EXPECT().BTCDelegation(gomock.Any()).Return(&Delegation{
		Status: btcstakingtypes.BTCDelegationStatus_VERIFIED.String(),
	}, nil).AnyTimes()

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

	sew.wg.Add(3)
	go func() {
		go sew.fetchDelegations()
		go sew.handlerVerifiedDelegations()
		go sew.handleUnbondedDelegations()
	}()

	require.Eventually(t, func() bool {
		return promtestutil.ToFloat64(sew.metrics.ReportedActivateDelegationsCounter) >= float64(expectedActivated)
	}, 60*time.Second, 100*time.Millisecond)
}

func TestHandlingDelegationsByEvents(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(time.Now().Unix()))
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cfg := config.DefaultBTCStakingTrackerConfig()
	cfg.CheckDelegationsInterval = 1 * time.Second
	cfg.FetchCometBlockInterval = 1 * time.Second

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

	sew.delegationRetrievalInProgress.Store(false)

	mockBabylonNodeAdapter.EXPECT().CometBFTTipHeight(gomock.Any()).DoAndReturn(func(_ context.Context) (int64, error) {
		return sew.currentCometTipHeight.Load() + 1, nil
	}).AnyTimes()

	defer close(sew.quit)

	expectedActivated := 1000
	stakingTxHashes := make([]string, 0, expectedActivated)

	for i := 0; i < expectedActivated; i++ {
		stk := datagen.GenRandomTx(r)
		delegation := Delegation{
			StakingTx:             stk,
			StakingOutputIdx:      0,
			DelegationStartHeight: 0,
			UnbondingOutput:       nil,
			HasProof:              false,
			Status:                btcstakingtypes.BTCDelegationStatus_VERIFIED.String(),
		}
		stkHash := stk.TxHash()
		stakingTxHashes = append(stakingTxHashes, stkHash.String())
		mockBabylonNodeAdapter.EXPECT().
			BTCDelegation(stkHash.String()).
			Return(&delegation, nil).AnyTimes()
	}

	firstCall := mockBabylonNodeAdapter.EXPECT().
		DelegationsModifiedInBlock(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(stakingTxHashes, nil).Times(1)

	mockBabylonNodeAdapter.EXPECT().
		DelegationsModifiedInBlock(gomock.Any(), gomock.Any(), gomock.Any()).
		After(firstCall).
		Return(nil, nil).AnyTimes()

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

	sew.wg.Add(3)
	go func() {
		go sew.fetchCometBftBlockForever()
		go sew.handlerVerifiedDelegations()
		go sew.handleUnbondedDelegations()
	}()

	require.Eventually(t, func() bool {
		return promtestutil.ToFloat64(sew.metrics.ReportedActivateDelegationsCounter) >= float64(expectedActivated)
	}, 90*time.Second, 100*time.Millisecond)
}

// TestTryParseStakerSignatureFromSpentTx_TimelockPath tests that when a spending tx
// uses the timelock path (3 witness elements instead of 4+), the function returns
// ErrSpendPathNotUnbonding error.
func TestTryParseStakerSignatureFromSpentTx_TimelockPath(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(time.Now().Unix()))

	// Create a staking tx
	stakingTx := datagen.GenRandomTx(r)
	stakingOutputIdx := uint32(0)

	// Create unbonding output that matches what the spending tx will have
	unbondingOutput := &wire.TxOut{
		Value:    1000,
		PkScript: []byte{0x00, 0x14, 0x01, 0x02, 0x03}, // dummy p2wpkh script
	}

	// Create tracked delegation
	td := &TrackedDelegation{
		StakingTx:        stakingTx,
		StakingOutputIdx: stakingOutputIdx,
		UnbondingOutput:  unbondingOutput,
	}

	// Create spending tx that references the staking output
	spendingTx := wire.NewMsgTx(2)

	// Add input that spends the staking output
	stakingTxHash := stakingTx.TxHash()
	spendingTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  stakingTxHash,
			Index: stakingOutputIdx,
		},
		// Timelock path witness has only 3 elements: signature, script, control_block
		// (no covenant signature like unbonding path which has 4+)
		Witness: wire.TxWitness{
			[]byte{0x01, 0x02, 0x03}, // staker signature (dummy)
			[]byte{0x04, 0x05, 0x06}, // script (dummy)
			[]byte{0x07, 0x08, 0x09}, // control block (dummy)
		},
	})

	// Add output matching unbonding output
	spendingTx.AddTxOut(unbondingOutput)

	// Call the function - should return ErrSpendPathNotUnbonding
	_, err := tryParseStakerSignatureFromSpentTx(spendingTx, td)

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrSpendPathNotUnbonding),
		"expected ErrSpendPathNotUnbonding, got: %v", err)
}

// TestTryParseStakerSignatureFromSpentTx_UnbondingPath tests that when a spending tx
// uses the unbonding path (4+ witness elements), the function attempts to parse
// the staker signature (may fail due to invalid signature format in test, but
// should NOT return ErrSpendPathNotUnbonding).
func TestTryParseStakerSignatureFromSpentTx_UnbondingPath(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(time.Now().Unix()))

	// Create a staking tx
	stakingTx := datagen.GenRandomTx(r)
	stakingOutputIdx := uint32(0)

	// Create unbonding output
	unbondingOutput := &wire.TxOut{
		Value:    1000,
		PkScript: []byte{0x00, 0x14, 0x01, 0x02, 0x03},
	}

	td := &TrackedDelegation{
		StakingTx:        stakingTx,
		StakingOutputIdx: stakingOutputIdx,
		UnbondingOutput:  unbondingOutput,
	}

	spendingTx := wire.NewMsgTx(2)
	stakingTxHash := stakingTx.TxHash()
	spendingTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  stakingTxHash,
			Index: stakingOutputIdx,
		},
		// Unbonding path witness has 4+ elements: covenant_sig(s), staker_sig, script, control_block
		Witness: wire.TxWitness{
			[]byte{0x01, 0x02, 0x03}, // covenant signature (dummy)
			[]byte{0x04, 0x05, 0x06}, // staker signature (dummy - invalid format)
			[]byte{0x07, 0x08, 0x09}, // script (dummy)
			[]byte{0x0a, 0x0b, 0x0c}, // control block (dummy)
		},
	})
	spendingTx.AddTxOut(unbondingOutput)

	// Call the function - should NOT return ErrSpendPathNotUnbonding
	// It may return a different error (invalid signature format) but not the timelock error
	_, err := tryParseStakerSignatureFromSpentTx(spendingTx, td)

	// The function should fail (invalid signature bytes), but NOT with ErrSpendPathNotUnbonding
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrSpendPathNotUnbonding),
		"should not return ErrSpendPathNotUnbonding for unbonding path, got: %v", err)
}
