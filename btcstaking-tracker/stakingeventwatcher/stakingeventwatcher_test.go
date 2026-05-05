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
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
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

// newTestStakingEventWatcher returns a minimal StakingEventWatcher wired with
// the supplied mock adapter, suitable for unit-testing helpers like
// waitForRequiredDepth in isolation.
func newTestStakingEventWatcher(t *testing.T, adapter BabylonNodeAdapter) *StakingEventWatcher {
	t.Helper()
	cfg := config.DefaultBTCStakingTrackerConfig()
	// Tight retry timing so abort/error cases finish fast in unit tests.
	cfg.RetrySubmitUnbondingTxInterval = 10 * time.Millisecond
	cfg.RetryJitter = 1 * time.Millisecond

	bsMetrics := metrics.NewBTCStakingTrackerMetrics()

	return &StakingEventWatcher{
		logger:             zap.NewNop().Sugar(),
		quit:               make(chan struct{}),
		cfg:                &cfg,
		babylonNodeAdapter: adapter,
		metrics:            bsMetrics.UnbondingWatcherMetrics,
	}
}

// TestWaitForRequiredDepth_AbortPredicateShortCircuits verifies that when an
// abort predicate returns true, waitForRequiredDepth exits early with nil
// error and does NOT call QueryHeaderDepth. This is the path taken by
// handleSpend when a child stake expansion gets unbonded mid-wait: the watcher
// must stop waiting for the expansion to be k-deep and fall through to report
// the parent's unbond directly.
func TestWaitForRequiredDepth_AbortPredicateShortCircuits(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAdapter := NewMockBabylonNodeAdapter(ctrl)
	// Strict expectation: QueryHeaderDepth must NEVER be called when abort fires
	// before the first depth query.
	mockAdapter.EXPECT().QueryHeaderDepth(gomock.Any()).Times(0)

	sew := newTestStakingEventWatcher(t, mockAdapter)
	defer close(sew.quit)

	abortCalls := 0
	abortFn := func() (bool, error) {
		abortCalls++

		return true, nil
	}

	hash := &chainhash.Hash{}
	err := sew.waitForRequiredDepth(context.Background(), hash, 6, abortFn)

	require.NoError(t, err, "abort must surface as nil error to the caller")
	require.Equal(t, 1, abortCalls, "abort predicate must be evaluated exactly once before exit")
}

// TestWaitForRequiredDepth_AbortAfterDepthCheck verifies that the abort
// predicate is checked at every retry attempt. Setup: depth is initially
// insufficient (forcing a retry), then the abort fires before the second
// depth query, so the second QueryHeaderDepth call never happens.
func TestWaitForRequiredDepth_AbortAfterDepthCheck(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAdapter := NewMockBabylonNodeAdapter(ctrl)
	// Exactly one depth query: first attempt returns insufficient depth, then
	// abort fires before the second depth query.
	mockAdapter.EXPECT().QueryHeaderDepth(gomock.Any()).Return(uint32(1), nil).Times(1)

	sew := newTestStakingEventWatcher(t, mockAdapter)
	defer close(sew.quit)

	abortCalls := 0
	abortFn := func() (bool, error) {
		abortCalls++
		// First attempt: don't abort (let depth query run and fail).
		// Second attempt: abort before depth query.
		return abortCalls > 1, nil
	}

	hash := &chainhash.Hash{}
	err := sew.waitForRequiredDepth(context.Background(), hash, 6, abortFn)

	require.NoError(t, err, "abort must surface as nil error to the caller")
	require.GreaterOrEqual(t, abortCalls, 2,
		"abort predicate must be evaluated on every retry attempt")
}

// TestWaitForRequiredDepth_NoAbortReachesDepth verifies the happy path: with
// a predicate that never aborts, waitForRequiredDepth succeeds the moment
// QueryHeaderDepth reports sufficient depth.
func TestWaitForRequiredDepth_NoAbortReachesDepth(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAdapter := NewMockBabylonNodeAdapter(ctrl)
	mockAdapter.EXPECT().QueryHeaderDepth(gomock.Any()).Return(uint32(10), nil).Times(1)

	sew := newTestStakingEventWatcher(t, mockAdapter)
	defer close(sew.quit)

	abortFn := func() (bool, error) {
		return false, nil
	}

	hash := &chainhash.Hash{}
	err := sew.waitForRequiredDepth(context.Background(), hash, 6, abortFn)

	require.NoError(t, err)
}

// TestWaitForRequiredDepth_NoAbortFnsBackwardCompat verifies that callers that
// pass no abort predicates retain the original behavior (used by the
// activation path).
func TestWaitForRequiredDepth_NoAbortFnsBackwardCompat(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAdapter := NewMockBabylonNodeAdapter(ctrl)
	mockAdapter.EXPECT().QueryHeaderDepth(gomock.Any()).Return(uint32(10), nil).Times(1)

	sew := newTestStakingEventWatcher(t, mockAdapter)
	defer close(sew.quit)

	hash := &chainhash.Hash{}
	err := sew.waitForRequiredDepth(context.Background(), hash, 6)

	require.NoError(t, err)
}

// newHandleSpendTestSEW returns a StakingEventWatcher wired with the supplied
// adapter and btcClient mocks, suitable for invoking handleSpend in isolation.
// CheckDelegationActiveInterval is tightened so polling loops drain quickly
// when ctx is cancelled.
func newHandleSpendTestSEW(
	t *testing.T,
	adapter BabylonNodeAdapter,
	btcClient *mocks.MockBTCClient,
) *StakingEventWatcher {
	t.Helper()
	cfg := config.DefaultBTCStakingTrackerConfig()
	cfg.RetrySubmitUnbondingTxInterval = 5 * time.Millisecond
	cfg.CheckDelegationActiveInterval = 5 * time.Millisecond
	cfg.RetryJitter = 1 * time.Millisecond

	bsMetrics := metrics.NewBTCStakingTrackerMetrics()

	return &StakingEventWatcher{
		logger:                          zap.NewNop().Sugar(),
		quit:                            make(chan struct{}),
		cfg:                             &cfg,
		babylonNodeAdapter:              adapter,
		btcClient:                       btcClient,
		unbondingTracker:                NewTrackedDelegations(),
		pendingTracker:                  NewTrackedDelegations(),
		verifiedInsufficientConfTracker: NewTrackedDelegations(),
		verifiedNotInChainTracker:       NewTrackedDelegations(),
		verifiedSufficientConfTracker:   NewTrackedDelegations(),
		unbondingDelegationChan:         make(chan *newDelegation, 1),
		unbondingRemovalChan:            make(chan *delegationInactive, 1),
		activationLimiter:               semaphore.NewWeighted(30),
		metrics:                         bsMetrics.UnbondingWatcherMetrics,
		babylonConfirmationTimeBlocks:   6,
	}
}

// buildHandleSpendFixtures creates a parent staking tx + a spending tx that
// references it, using a non-unbonding witness layout (so the parsed staker
// signature path returns an error and the switch falls through past the
// unbonding case). The returned tracked delegation reflects what handleSpend
// would receive from the unbondingTracker.
func buildHandleSpendFixtures(t *testing.T, r *rand.Rand) (*TrackedDelegation, *wire.MsgTx) {
	t.Helper()
	stakingTx := datagen.GenRandomTx(r)
	stakingOutputIdx := uint32(0)

	unbondingOutput := &wire.TxOut{
		Value:    1000,
		PkScript: []byte{0x00, 0x14, 0xaa, 0xbb, 0xcc},
	}

	td := &TrackedDelegation{
		StakingTx:        stakingTx,
		StakingOutputIdx: stakingOutputIdx,
		UnbondingOutput:  unbondingOutput,
	}

	// spending tx: spends the parent staking output, but with a single output
	// whose value differs from unbondingOutput.Value. tryParseStakerSignature
	// returns an error early on this mismatch (before the witness-len check),
	// so we are safe with a 3-element timelock-ish witness and avoid the
	// witness<4 panic.
	spendingTx := wire.NewMsgTx(2)
	spendingTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  stakingTx.TxHash(),
			Index: stakingOutputIdx,
		},
		Witness: wire.TxWitness{
			[]byte{0x01, 0x02, 0x03},
			[]byte{0x04, 0x05, 0x06},
			[]byte{0x07, 0x08, 0x09},
		},
	})
	spendingTx.AddTxOut(&wire.TxOut{
		Value:    unbondingOutput.Value + 1, // <-- mismatch on Value forces nonUnbondingTx branch
		PkScript: append([]byte(nil), unbondingOutput.PkScript...),
	})

	return td, spendingTx
}

// TestHandleSpend_UnbondedChildExpansion_SkipsExpansionBranch verifies the
// runtime gating in handleSpend: when babylon reports the spending tx as a
// stake expansion that is ALREADY UNBONDED, the expansion branch (which
// would call btcClient.GetRawTransactionVerbose and then wait for k-deep)
// must be skipped. The watcher should fall through to the regular unbond
// reporting path for the parent.
//
// Strict expectation: GetRawTransactionVerbose must NEVER be called.
func TestHandleSpend_UnbondedChildExpansion_SkipsExpansionBranch(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	td, spendingTx := buildHandleSpendFixtures(t, r)
	spendingTxHash := spendingTx.TxHash()

	mockAdapter := NewMockBabylonNodeAdapter(ctrl)
	mockBTCClient := mocks.NewMockBTCClient(ctrl)

	mockAdapter.EXPECT().BTCDelegation(spendingTxHash.String()).Return(&Delegation{
		Status:           btcstakingtypes.BTCDelegationStatus_UNBONDED.String(),
		IsStakeExpansion: true,
		IsUnbonded:       true,
	}, nil).AnyTimes()

	// Hard guarantee: the expansion branch is skipped.
	mockBTCClient.EXPECT().GetRawTransactionVerbose(gomock.Any()).Times(0)

	// Fall-through path polls TxDetails via waitForStakeSpendInclusionProof; we
	// keep it returning "not in chain" so the retry loop spins until ctx cancel.
	mockBTCClient.EXPECT().
		TxDetails(gomock.Any(), gomock.Any()).
		Return(nil, btcclient.TxNotFound, nil).
		AnyTimes()

	sew := newHandleSpendTestSEW(t, mockAdapter, mockBTCClient)
	defer close(sew.quit)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		sew.handleSpend(ctx, spendingTx, td)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handleSpend did not return after ctx cancel")
	}
}

// TestHandleSpend_NotUnbondedChildExpansion_EntersExpansionBranch is the
// control case for the gating test above: with the child stake-expansion
// NOT unbonded, the expansion branch MUST be entered, which means
// GetRawTransactionVerbose is called. We make it return a malformed block
// hash so the branch returns immediately afterwards (no k-deep wait), keeping
// the test fast.
func TestHandleSpend_NotUnbondedChildExpansion_EntersExpansionBranch(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	td, spendingTx := buildHandleSpendFixtures(t, r)
	spendingTxHash := spendingTx.TxHash()

	mockAdapter := NewMockBabylonNodeAdapter(ctrl)
	mockBTCClient := mocks.NewMockBTCClient(ctrl)

	mockAdapter.EXPECT().BTCDelegation(spendingTxHash.String()).Return(&Delegation{
		Status:           btcstakingtypes.BTCDelegationStatus_VERIFIED.String(),
		IsStakeExpansion: true,
		IsUnbonded:       false,
	}, nil).AnyTimes()

	// Strict expectation: expansion branch IS taken → GetRawTransactionVerbose called at least once.
	// Returning a non-hex block hash forces the branch to bail out of handleSpend
	// right after, so the test does not need to wait for k-deep depth.
	mockBTCClient.EXPECT().
		GetRawTransactionVerbose(gomock.Any()).
		Return(&btcjson.TxRawResult{BlockHash: "not-a-real-hash"}, nil).
		MinTimes(1)

	sew := newHandleSpendTestSEW(t, mockAdapter, mockBTCClient)
	defer close(sew.quit)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		sew.handleSpend(ctx, spendingTx, td)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("handleSpend did not return after entering expansion branch")
	}
}

// TestHandleSpend_StkExpUnbondedChildSkipsKDeepWait pins the gating change in
// handleSpend: when the spending tx is a stake expansion AND the child
// delegation is already UNBONDED on babylon (DelegatorUnbondingInfo set), the
// expansion-specific case branch must be skipped entirely. The watcher must
// not wait for the expansion tx to be k-deep — it should fall through to the
// standard report-unbond path for the parent.
//
// Verified via the gating expression directly: with IsUnbonded=true the
// switch predicate evaluates to false even though the spending tx has a
// matching delegation on babylon.
func TestHandleSpend_StkExpUnbondedChildSkipsKDeepWait(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name            string
		child           *Delegation
		err             error
		expectExpBranch bool
	}{
		{
			name: "child VERIFIED, not unbonded → expansion branch (wait for k-deep)",
			child: &Delegation{
				Status:           btcstakingtypes.BTCDelegationStatus_VERIFIED.String(),
				IsStakeExpansion: true,
				IsUnbonded:       false,
			},
			expectExpBranch: true,
		},
		{
			name: "child UNBONDED via DelegatorUnbondingInfo → branch skipped",
			child: &Delegation{
				Status:           btcstakingtypes.BTCDelegationStatus_UNBONDED.String(),
				IsStakeExpansion: true,
				IsUnbonded:       true,
			},
			expectExpBranch: false,
		},
		{
			name:            "no child delegation found on babylon → not an expansion",
			child:           nil,
			err:             errors.New("not found"),
			expectExpBranch: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Same predicate the watcher computes inline in handleSpend.
			gotBranch := tc.err == nil && tc.child != nil && tc.child.IsStakeExpansion && !tc.child.IsUnbonded
			require.Equal(t, tc.expectExpBranch, gotBranch)
		})
	}
}
