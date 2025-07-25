package btcslasher_test

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/lightningnetwork/lnd/chainntnfs"

	sdkmath "cosmossdk.io/math"
	"github.com/btcsuite/btcd/btcec/v2"

	datagen "github.com/babylonlabs-io/babylon/v3/testutil/datagen"
	bbn "github.com/babylonlabs-io/babylon/v3/types"
	btcctypes "github.com/babylonlabs-io/babylon/v3/x/btccheckpoint/types"
	bstypes "github.com/babylonlabs-io/babylon/v3/x/btcstaking/types"
	ftypes "github.com/babylonlabs-io/babylon/v3/x/finality/types"
	"github.com/babylonlabs-io/vigilante/btcstaking-tracker/btcslasher"
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
				"",
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
				"",
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
