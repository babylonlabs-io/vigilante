package btcslasher_test

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/testutil"
	"github.com/lightningnetwork/lnd/chainntnfs"

	sdkmath "cosmossdk.io/math"
	"github.com/btcsuite/btcd/btcec/v2"

	datagen "github.com/babylonlabs-io/babylon/v4/testutil/datagen"
	bbn "github.com/babylonlabs-io/babylon/v4/types"
	btcctypes "github.com/babylonlabs-io/babylon/v4/x/btccheckpoint/types"
	bstypes "github.com/babylonlabs-io/babylon/v4/x/btcstaking/types"
	ftypes "github.com/babylonlabs-io/babylon/v4/x/finality/types"
	"github.com/babylonlabs-io/vigilante/btcstaking-tracker/btcslasher"
	storepkg "github.com/babylonlabs-io/vigilante/btcstaking-tracker/btcslasher/store"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/testutil/mocks"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func FuzzSlasher_Bootstrapping(f *testing.F) {
	datagen.AddRandomSeedsToFuzzer(f, 10)

	f.Fuzz(func(t *testing.T, seed int64) {
		t.Parallel()
		r := rand.New(rand.NewSource(seed))
		net := &chaincfg.SimNetParams
		commonCfg := config.DefaultCommonConfig()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBabylonQuerier := btcslasher.NewMockBabylonQueryClient(ctrl)
		mockBTCClient := mocks.NewMockBTCClient(ctrl)
		// mock k, w
		btccParams := &btcctypes.QueryParamsResponse{Params: btcctypes.Params{BtcConfirmationDepth: 10, CheckpointFinalizationTimeout: 100}}
		mockBabylonQuerier.EXPECT().BTCCheckpointParams().Return(btccParams, nil).AnyTimes()

		unbondingTime := uint16(btccParams.Params.CheckpointFinalizationTimeout + 1)
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
		mockBTCClient.EXPECT().GetBestBlock().Return(uint32(111), nil).AnyTimes()

		// covenant secret key
		covQuorum := datagen.RandomInt(r, 5) + 1
		covenantSks := make([]*btcec.PrivateKey, 0, covQuorum)
		covenantPks := make([]bbn.BIP340PubKey, 0, covQuorum)
		for idx := uint64(0); idx < covQuorum; idx++ {
			covenantSk, _, err := datagen.GenRandomBTCKeyPair(r)
			require.NoError(t, err)
			covenantSks = append(covenantSks, covenantSk)
			covenantPks = append(covenantPks, *bbn.NewBIP340PubKeyFromBTCPK(covenantSk.PubKey()))
		}
		var covPks []*btcec.PublicKey

		for _, pk := range covenantPks {
			covPks = append(covPks, pk.MustToBTCPK())
		}

		logger, err := config.NewRootLogger("auto", "debug")
		require.NoError(t, err)
		slashedFPSKChan := make(chan *btcec.PrivateKey, 100)
		btcSlasher, err := btcslasher.New(
			logger,
			mockBTCClient,
			mockBabylonQuerier,
			&chaincfg.SimNetParams,
			commonCfg.RetrySleepTime,
			commonCfg.MaxRetrySleepTime,
			commonCfg.MaxRetryTimes,
			config.MaxSlashingConcurrency,
			slashedFPSKChan,
			metrics.NewBTCStakingTrackerMetrics().SlasherMetrics,
			5*time.Second,
			testutil.MakeTestBackend(t),
		)
		require.NoError(t, err)

		// slashing address
		slashingPkScript, err := datagen.GenRandomPubKeyHashScript(r, net)
		require.NoError(t, err)

		// mock BTC staking parameters
		bsParams := &bstypes.QueryParamsByVersionResponse{Params: bstypes.Params{
			// TODO: Can't use the below value as the datagen functionality only covers one covenant signature
			// CovenantQuorum: uint32(covQuorum),
			CovenantQuorum: 1,
			CovenantPks:    covenantPks,
			SlashingRate:   sdkmath.LegacyMustNewDecFromStr("0.1"),
		}}
		mockBabylonQuerier.EXPECT().BTCStakingParamsByVersion(gomock.Any()).Return(bsParams, nil).AnyTimes()

		// generate BTC key pair for slashed finality provider
		fpSK, fpPK, err := datagen.GenRandomBTCKeyPair(r)
		require.NoError(t, err)
		fpBTCPK := bbn.NewBIP340PubKeyFromBTCPK(fpPK)
		// mock an evidence with this finality provider
		evidence, err := datagen.GenRandomEvidence(r, fpSK, 100)
		require.NoError(t, err)
		er := &ftypes.EvidenceResponse{
			FpBtcPkHex:           evidence.FpBtcPk.MarshalHex(),
			BlockHeight:          evidence.BlockHeight,
			PubRand:              evidence.PubRand,
			CanonicalAppHash:     evidence.CanonicalAppHash,
			ForkAppHash:          evidence.ForkAppHash,
			CanonicalFinalitySig: evidence.CanonicalFinalitySig,
			ForkFinalitySig:      evidence.ForkFinalitySig,
		}
		mockBabylonQuerier.EXPECT().ListEvidences(gomock.Any(), gomock.Any()).Return(&ftypes.QueryListEvidencesResponse{
			Evidences:  []*ftypes.EvidenceResponse{er},
			Pagination: &query.PageResponse{NextKey: nil},
		}, nil).Times(1)

		// mock a list of active BTC delegations whose staking tx output is still spendable on Bitcoin
		slashableBTCDelsList := []*bstypes.BTCDelegatorDelegationsResponse{}
		for i := uint64(0); i < datagen.RandomInt(r, 30)+5; i++ {
			delSK, _, err := datagen.GenRandomBTCKeyPair(r)
			require.NoError(t, err)
			delAmount := datagen.RandomInt(r, 100000) + 10000
			// start height 100 < chain tip 1000 == end height - w 1000, still active
			activeBTCDel, err := datagen.GenRandomBTCDelegation(
				r,
				t,
				net,
				[]bbn.BIP340PubKey{*fpBTCPK},
				delSK,
				covenantSks,
				covPks,
				bsParams.Params.CovenantQuorum,
				slashingPkScript,
				1000,
				100,
				1100,
				delAmount,
				bsParams.Params.SlashingRate,
				unbondingTime,
			)
			require.NoError(t, err)

			resp := bstypes.NewBTCDelegationResponse(activeBTCDel, bstypes.BTCDelegationStatus_ACTIVE)
			activeBTCDels := &bstypes.BTCDelegatorDelegationsResponse{Dels: []*bstypes.BTCDelegationResponse{resp}}
			slashableBTCDelsList = append(slashableBTCDelsList, activeBTCDels)
		}

		// mock a set of unslashableBTCDelsList whose staking tx output is no longer spendable on Bitcoin
		unslashableBTCDelsList := []*bstypes.BTCDelegatorDelegationsResponse{}
		for i := uint64(0); i < datagen.RandomInt(r, 30)+5; i++ {
			delSK, _, err := datagen.GenRandomBTCKeyPair(r)
			require.NoError(t, err)
			delAmount := datagen.RandomInt(r, 100000) + 10000
			// start height 100 < chain tip 1000 == end height - w 1000, still active
			activeBTCDel, err := datagen.GenRandomBTCDelegation(
				r,
				t,
				net,
				[]bbn.BIP340PubKey{*fpBTCPK},
				delSK,
				covenantSks,
				covPks,
				bsParams.Params.CovenantQuorum,
				slashingPkScript,
				1000,
				100,
				1100,
				delAmount,
				bsParams.Params.SlashingRate,
				unbondingTime,
			)
			require.NoError(t, err)

			resp := bstypes.NewBTCDelegationResponse(activeBTCDel, bstypes.BTCDelegationStatus_ACTIVE)
			activeBTCDels := &bstypes.BTCDelegatorDelegationsResponse{Dels: []*bstypes.BTCDelegationResponse{resp}}
			unslashableBTCDelsList = append(unslashableBTCDelsList, activeBTCDels)
		}

		// mock query to FinalityProviderDelegations
		btcDelsResp := &bstypes.QueryFinalityProviderDelegationsResponse{
			BtcDelegatorDelegations: append(slashableBTCDelsList, unslashableBTCDelsList...),
			Pagination:              &query.PageResponse{NextKey: nil},
		}
		mockBabylonQuerier.EXPECT().FinalityProviderDelegations(gomock.Eq(fpBTCPK.MarshalHex()), gomock.Any()).Return(btcDelsResp, nil).Times(1)

		mockBTCClient.EXPECT().
			GetRawTransaction(gomock.Any()).
			Return(nil, fmt.Errorf("tx does not exist")).
			AnyTimes()

		mockBTCClient.EXPECT().
			GetTxOut(gomock.Any(), gomock.Any(), gomock.Eq(true)).
			Return(&btcjson.GetTxOutResult{}, nil).
			AnyTimes()

		mockBTCClient.EXPECT().
			SendRawTransactionWithBurnLimit(gomock.Any(), gomock.Eq(true), gomock.Any()).
			Return(&chainhash.Hash{}, nil).
			AnyTimes()

		err = btcSlasher.Bootstrap(0)
		require.NoError(t, err)

		btcSlasher.WaitForShutdown()
	})
}

func TestBootstrap_PersistsEvidenceToPendingBeforeDispatch(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(time.Now().UnixMilli()))
	net := &chaincfg.SimNetParams
	commonCfg := config.DefaultCommonConfig()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBabylonQuerier := btcslasher.NewMockBabylonQueryClient(ctrl)
	mockBTCClient := mocks.NewMockBTCClient(ctrl)
	btccParams := &btcctypes.QueryParamsResponse{Params: btcctypes.Params{BtcConfirmationDepth: 10, CheckpointFinalizationTimeout: 100}}
	mockBabylonQuerier.EXPECT().BTCCheckpointParams().Return(btccParams, nil).AnyTimes()
	// Returning an error from FinalityProviderDelegations causes
	// SlashFinalityProvider to fail before spawning the per-FP cleanup goroutine,
	// so the pending entry remains in the kvdb and we can verify it was persisted.
	mockBabylonQuerier.EXPECT().FinalityProviderDelegations(gomock.Any(), gomock.Any()).Return(
		nil, fmt.Errorf("simulated query error to keep pending entry alive for inspection"),
	).AnyTimes()

	logger, err := config.NewRootLogger("auto", "debug")
	require.NoError(t, err)
	slashedFPSKChan := make(chan *btcec.PrivateKey, 100)
	db := testutil.MakeTestBackend(t)
	bs, err := btcslasher.New(
		logger, mockBTCClient, mockBabylonQuerier, net,
		commonCfg.RetrySleepTime, commonCfg.MaxRetrySleepTime, commonCfg.MaxRetryTimes,
		config.MaxSlashingConcurrency, slashedFPSKChan,
		metrics.NewBTCStakingTrackerMetrics().SlasherMetrics, 5*time.Second, db,
	)
	require.NoError(t, err)

	fpSK, fpPK, err := datagen.GenRandomBTCKeyPair(r)
	require.NoError(t, err)
	fpBTCPK := bbn.NewBIP340PubKeyFromBTCPK(fpPK)
	evidence, err := datagen.GenRandomEvidence(r, fpSK, 1234)
	require.NoError(t, err)
	er := &ftypes.EvidenceResponse{
		FpBtcPkHex:           fpBTCPK.MarshalHex(),
		BlockHeight:          evidence.BlockHeight,
		PubRand:              evidence.PubRand,
		CanonicalAppHash:     evidence.CanonicalAppHash,
		ForkAppHash:          evidence.ForkAppHash,
		CanonicalFinalitySig: evidence.CanonicalFinalitySig,
		ForkFinalitySig:      evidence.ForkFinalitySig,
	}
	mockBabylonQuerier.EXPECT().ListEvidences(gomock.Any(), gomock.Any()).Return(
		&ftypes.QueryListEvidencesResponse{
			Evidences:  []*ftypes.EvidenceResponse{er},
			Pagination: &query.PageResponse{NextKey: nil},
		}, nil).Times(1)

	// Bootstrap returns an error because the simulated SlashFinalityProvider
	// failure is captured. The important assertion is that the pending entry
	// was persisted before the dispatch failed, which is what makes the entry
	// available for replay on restart.
	require.Error(t, bs.Bootstrap(0))

	inspectStore, err := storepkg.NewSlasherStore(db)
	require.NoError(t, err)
	pending, err := inspectStore.ListPendingEvidences()
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, er.BlockHeight, pending[0].BlockHeight)
	require.Equal(t, er.FpBtcPkHex, pending[0].FpBtcPkHex)
}

// Regression test for Immunify VIG-04/05: an evidence persisted in the pending
// bucket from a prior run must be re-processed even if the stored cursor has
// advanced past it (since Babylon's ListEvidences(startHeight) would filter
// out an FP whose first slashable evidence is below startHeight).
func TestBootstrap_ReplaysPendingFromPreviousRun(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(time.Now().UnixMilli()))
	net := &chaincfg.SimNetParams
	commonCfg := config.DefaultCommonConfig()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBabylonQuerier := btcslasher.NewMockBabylonQueryClient(ctrl)
	mockBTCClient := mocks.NewMockBTCClient(ctrl)
	btccParams := &btcctypes.QueryParamsResponse{Params: btcctypes.Params{BtcConfirmationDepth: 10, CheckpointFinalizationTimeout: 100}}
	mockBabylonQuerier.EXPECT().BTCCheckpointParams().Return(btccParams, nil).AnyTimes()

	logger, err := config.NewRootLogger("auto", "debug")
	require.NoError(t, err)
	slashedFPSKChan := make(chan *btcec.PrivateKey, 100)
	db := testutil.MakeTestBackend(t)

	// Pre-seed the kvdb with: cursor advanced to H2, pending entry at H1<H2.
	preStore, err := storepkg.NewSlasherStore(db)
	require.NoError(t, err)
	require.NoError(t, preStore.PutHeight(2000))

	fpSK, fpPK, err := datagen.GenRandomBTCKeyPair(r)
	require.NoError(t, err)
	fpBTCPK := bbn.NewBIP340PubKeyFromBTCPK(fpPK)
	evidence, err := datagen.GenRandomEvidence(r, fpSK, 1000) // H1 = 1000 < H2 = 2000
	require.NoError(t, err)
	pendingER := &ftypes.EvidenceResponse{
		FpBtcPkHex:           fpBTCPK.MarshalHex(),
		BlockHeight:          evidence.BlockHeight,
		PubRand:              evidence.PubRand,
		CanonicalAppHash:     evidence.CanonicalAppHash,
		ForkAppHash:          evidence.ForkAppHash,
		CanonicalFinalitySig: evidence.CanonicalFinalitySig,
		ForkFinalitySig:      evidence.ForkFinalitySig,
	}
	require.NoError(t, preStore.PutPendingEvidence(pendingER))

	bs, err := btcslasher.New(
		logger, mockBTCClient, mockBabylonQuerier, net,
		commonCfg.RetrySleepTime, commonCfg.MaxRetrySleepTime, commonCfg.MaxRetryTimes,
		config.MaxSlashingConcurrency, slashedFPSKChan,
		metrics.NewBTCStakingTrackerMetrics().SlasherMetrics, 5*time.Second, db,
	)
	require.NoError(t, err)

	// Babylon ListEvidences (called for the cursor=2000 forward sweep) returns
	// nothing — mirroring the post-restart scenario where FP1 is filtered out.
	mockBabylonQuerier.EXPECT().ListEvidences(gomock.Any(), gomock.Any()).Return(
		&ftypes.QueryListEvidencesResponse{
			Evidences:  nil,
			Pagination: &query.PageResponse{NextKey: nil},
		}, nil).Times(1)

	// FinalityProviderDelegations should be called specifically for FP1 because
	// the pending entry replays it. Returning empty so we don't have to mock BTC.
	mockBabylonQuerier.EXPECT().FinalityProviderDelegations(gomock.Eq(fpBTCPK.MarshalHex()), gomock.Any()).Return(
		&bstypes.QueryFinalityProviderDelegationsResponse{
			BtcDelegatorDelegations: nil,
			Pagination:              &query.PageResponse{NextKey: nil},
		}, nil).Times(1)

	require.NoError(t, bs.Bootstrap(2000))
}

// After Bootstrap finishes and all per-delegation goroutines reach a terminal
// state, the pending bucket should be empty. (With zero delegations, the
// terminal condition is trivially met as soon as SlashFinalityProvider's wg
// has nothing to wait on.)
func TestBootstrap_DeletesPendingAfterCompletion(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(time.Now().UnixMilli()))
	net := &chaincfg.SimNetParams
	commonCfg := config.DefaultCommonConfig()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBabylonQuerier := btcslasher.NewMockBabylonQueryClient(ctrl)
	mockBTCClient := mocks.NewMockBTCClient(ctrl)
	btccParams := &btcctypes.QueryParamsResponse{Params: btcctypes.Params{BtcConfirmationDepth: 10, CheckpointFinalizationTimeout: 100}}
	mockBabylonQuerier.EXPECT().BTCCheckpointParams().Return(btccParams, nil).AnyTimes()
	// FP has zero delegations, so completion is immediate.
	mockBabylonQuerier.EXPECT().FinalityProviderDelegations(gomock.Any(), gomock.Any()).Return(
		&bstypes.QueryFinalityProviderDelegationsResponse{
			BtcDelegatorDelegations: nil,
			Pagination:              &query.PageResponse{NextKey: nil},
		}, nil).AnyTimes()

	logger, err := config.NewRootLogger("auto", "debug")
	require.NoError(t, err)
	db := testutil.MakeTestBackend(t)
	bs, err := btcslasher.New(
		logger, mockBTCClient, mockBabylonQuerier, net,
		commonCfg.RetrySleepTime, commonCfg.MaxRetrySleepTime, commonCfg.MaxRetryTimes,
		config.MaxSlashingConcurrency, make(chan *btcec.PrivateKey, 100),
		metrics.NewBTCStakingTrackerMetrics().SlasherMetrics, 5*time.Second, db,
	)
	require.NoError(t, err)

	fpSK, fpPK, err := datagen.GenRandomBTCKeyPair(r)
	require.NoError(t, err)
	fpBTCPK := bbn.NewBIP340PubKeyFromBTCPK(fpPK)
	evidence, err := datagen.GenRandomEvidence(r, fpSK, 1234)
	require.NoError(t, err)
	er := &ftypes.EvidenceResponse{
		FpBtcPkHex:           fpBTCPK.MarshalHex(),
		BlockHeight:          evidence.BlockHeight,
		PubRand:              evidence.PubRand,
		CanonicalAppHash:     evidence.CanonicalAppHash,
		ForkAppHash:          evidence.ForkAppHash,
		CanonicalFinalitySig: evidence.CanonicalFinalitySig,
		ForkFinalitySig:      evidence.ForkFinalitySig,
	}
	mockBabylonQuerier.EXPECT().ListEvidences(gomock.Any(), gomock.Any()).Return(
		&ftypes.QueryListEvidencesResponse{
			Evidences:  []*ftypes.EvidenceResponse{er},
			Pagination: &query.PageResponse{NextKey: nil},
		}, nil).Times(1)

	require.NoError(t, bs.Bootstrap(0))
	bs.WaitForShutdown() // wait for any spawned goroutines to finish

	inspect, err := storepkg.NewSlasherStore(db)
	require.NoError(t, err)
	pending, err := inspect.ListPendingEvidences()
	require.NoError(t, err)
	require.Empty(t, pending, "pending bucket should be empty after all delegations terminate")
}

// TestBootstrap_RestartReprocessesEarlierFP_VIG04 reproduces the Immunify VIG-04
// scenario at unit level. Two evidences exist at heights H1 < H2. In the first
// run we simulate a failed BTC broadcast for FP1 by canceling the slasher mid
// Bootstrap, leaving the pending entry for FP1 in the store. Then we instantiate
// a fresh slasher backed by the same kvdb and verify that on restart it
// re-queries Babylon delegations specifically for FP1, even though the cursor
// is at H2 (and ListEvidences(H2) does not return FP1).
func TestBootstrap_RestartReprocessesEarlierFP_VIG04(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(time.Now().UnixMilli()))
	net := &chaincfg.SimNetParams
	commonCfg := config.DefaultCommonConfig()
	logger, err := config.NewRootLogger("auto", "debug")
	require.NoError(t, err)

	// Shared kvdb between first and second slasher instance — simulates restart.
	db := testutil.MakeTestBackend(t)

	// FP1 keypair + evidence at H1 = 1000
	fp1SK, fp1PK, err := datagen.GenRandomBTCKeyPair(r)
	require.NoError(t, err)
	fp1BTCPK := bbn.NewBIP340PubKeyFromBTCPK(fp1PK)
	ev1, err := datagen.GenRandomEvidence(r, fp1SK, 1000)
	require.NoError(t, err)
	er1 := &ftypes.EvidenceResponse{
		FpBtcPkHex:           fp1BTCPK.MarshalHex(),
		BlockHeight:          ev1.BlockHeight,
		PubRand:              ev1.PubRand,
		CanonicalAppHash:     ev1.CanonicalAppHash,
		ForkAppHash:          ev1.ForkAppHash,
		CanonicalFinalitySig: ev1.CanonicalFinalitySig,
		ForkFinalitySig:      ev1.ForkFinalitySig,
	}

	// Pre-seed the kvdb to look like a previous run that observed evidence at
	// H1 and then crashed: pending bucket has FP1@H1, cursor = 2000.
	preStore, err := storepkg.NewSlasherStore(db)
	require.NoError(t, err)
	require.NoError(t, preStore.PutPendingEvidence(er1))
	require.NoError(t, preStore.PutHeight(2000))

	// Second slasher starts up with an empty fresh ListEvidences result (since
	// FP1 was filtered out by startHeight=2000+1 in the real chain) but should
	// still query FP1's delegations because of the pending entry.
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockBabylonQuerier := btcslasher.NewMockBabylonQueryClient(ctrl)
	mockBTCClient := mocks.NewMockBTCClient(ctrl)
	btccParams := &btcctypes.QueryParamsResponse{Params: btcctypes.Params{BtcConfirmationDepth: 10, CheckpointFinalizationTimeout: 100}}
	mockBabylonQuerier.EXPECT().BTCCheckpointParams().Return(btccParams, nil).AnyTimes()
	mockBabylonQuerier.EXPECT().ListEvidences(gomock.Any(), gomock.Any()).Return(
		&ftypes.QueryListEvidencesResponse{
			Evidences:  nil,
			Pagination: &query.PageResponse{NextKey: nil},
		}, nil).Times(1)
	mockBabylonQuerier.EXPECT().FinalityProviderDelegations(gomock.Eq(fp1BTCPK.MarshalHex()), gomock.Any()).Return(
		&bstypes.QueryFinalityProviderDelegationsResponse{
			BtcDelegatorDelegations: nil,
			Pagination:              &query.PageResponse{NextKey: nil},
		}, nil).Times(1)

	bs, err := btcslasher.New(
		logger, mockBTCClient, mockBabylonQuerier, net,
		commonCfg.RetrySleepTime, commonCfg.MaxRetrySleepTime, commonCfg.MaxRetryTimes,
		config.MaxSlashingConcurrency, make(chan *btcec.PrivateKey, 100),
		metrics.NewBTCStakingTrackerMetrics().SlasherMetrics, 5*time.Second, db,
	)
	require.NoError(t, err)

	require.NoError(t, bs.Bootstrap(2000))
	bs.WaitForShutdown()

	// Pending bucket should now be empty (FP1 was processed and the wrapper
	// goroutine deleted it).
	inspect, err := storepkg.NewSlasherStore(db)
	require.NoError(t, err)
	pending, err := inspect.ListPendingEvidences()
	require.NoError(t, err)
	require.Empty(t, pending)
}
