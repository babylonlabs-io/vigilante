package btcslasher

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/babylonlabs-io/babylon/v3/testutil/datagen"
	bbn "github.com/babylonlabs-io/babylon/v3/types"
	bstypes "github.com/babylonlabs-io/babylon/v3/x/btcstaking/types"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/testutil/mocks"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestBTCSlasher_slashBTCDelegation_exitUnslashable(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(time.Now().UnixMilli()))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBabylonQuerier := NewMockBabylonQueryClient(ctrl)
	mockBTCClient := mocks.NewMockBTCClient(ctrl)
	// mock btc
	mockBTCClient.EXPECT().GetRawTransaction(gomock.Any()).Return(nil, fmt.Errorf("mock not found")).AnyTimes()
	// always return nil for GetTxOut, we want to simulate that it's not spendable
	mockBTCClient.EXPECT().GetTxOut(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	btcSlasher := &BTCSlasher{
		logger:                 zaptest.NewLogger(t).Named(t.Name()).Sugar(),
		BTCClient:              mockBTCClient,
		BBNQuerier:             mockBabylonQuerier,
		netParams:              nil,
		btcFinalizationTimeout: 0,
		retrySleepTime:         1 * time.Second,
		maxRetrySleepTime:      5 * time.Second,
		maxRetryTimes:          0,
		metrics:                metrics.NewBTCStakingTrackerMetrics().SlasherMetrics,
		slashResultChan:        make(chan *SlashResult, 1),
	}

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

	fpSK, fpPK, err := datagen.GenRandomBTCKeyPair(r)
	require.NoError(t, err)
	delSK, _, err := datagen.GenRandomBTCKeyPair(r)
	require.NoError(t, err)
	fpBTCPK := bbn.NewBIP340PubKeyFromBTCPK(fpPK)
	activeBTCDel, err := datagen.GenRandomBTCDelegation(
		r,
		t,
		&chaincfg.SimNetParams,
		[]bbn.BIP340PubKey{*fpBTCPK},
		delSK,
		covenantSks,
		covPks,
		1,
		[]byte("test"),
		1000,
		100,
		1100,
		100000,
		sdkmath.LegacyMustNewDecFromStr("0.1"),
		10,
	)
	require.NoError(t, err)

	del := bstypes.NewBTCDelegationResponse(activeBTCDel, bstypes.BTCDelegationStatus_ACTIVE)

	btcSlasher.slashBTCDelegation(fpBTCPK, fpSK, del)

	// check if the slashing result is correct
	slashedFP := <-btcSlasher.slashResultChan
	require.NoError(t, slashedFP.Err, "slashing should not fail")
}
