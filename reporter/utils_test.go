package reporter_test

import (
	"github.com/babylonlabs-io/babylon/client/babylonclient"
	"github.com/lightningnetwork/lnd/lntest/mock"
	"math/rand"
	"testing"

	"github.com/babylonlabs-io/babylon/testutil/datagen"
	btcctypes "github.com/babylonlabs-io/babylon/x/btccheckpoint/types"
	btclctypes "github.com/babylonlabs-io/babylon/x/btclightclient/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/reporter"
	vdatagen "github.com/babylonlabs-io/vigilante/testutil/datagen"
	"github.com/babylonlabs-io/vigilante/testutil/mocks"
	"github.com/babylonlabs-io/vigilante/types"
)

func newMockReporter(t *testing.T, ctrl *gomock.Controller) (
	*reporter.MockBabylonClient, *reporter.Reporter) {
	cfg := config.DefaultConfig()
	logger, err := cfg.CreateLogger()
	require.NoError(t, err)

	mockBTCClient := mocks.NewMockBTCClient(ctrl)
	mockBabylonClient := reporter.NewMockBabylonClient(ctrl)
	btccParams := btcctypes.DefaultParams()
	mockBabylonClient.EXPECT().GetConfig().Return(&cfg.Babylon).AnyTimes()
	mockBabylonClient.EXPECT().BTCCheckpointParams().Return(
		&btcctypes.QueryParamsResponse{Params: btccParams}, nil).AnyTimes()
	mockNotifier := mock.ChainNotifier{}

	r, err := reporter.New(
		&cfg.Reporter,
		logger,
		mockBTCClient,
		mockBabylonClient,
		&mockNotifier,
		cfg.Common.RetrySleepTime,
		cfg.Common.MaxRetrySleepTime,
		metrics.NewReporterMetrics(),
	)
	require.NoError(t, err)

	return mockBabylonClient, r
}

// FuzzProcessHeaders fuzz tests ProcessHeaders()
// - Data: a number of random blocks, with or without Babylon txs
// - Tested property: for any BTC block, if its header is not duplicated, then it will submit this header
func FuzzProcessHeaders(f *testing.F) {
	datagen.AddRandomSeedsToFuzzer(f, 10)

	f.Fuzz(func(t *testing.T, seed int64) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		r := rand.New(rand.NewSource(seed))

		// generate a random number of blocks
		numBlocks := datagen.RandomInt(r, 1000) + 100 // more than 1 pages of MsgInsertHeader messages to submit
		blocks, _, _ := vdatagen.GenRandomBlockchainWithBabylonTx(r, numBlocks, 0, 0)
		ibs := []*types.IndexedBlock{}
		for _, block := range blocks {
			ibs = append(ibs, types.NewIndexedBlockFromMsgBlock(r.Uint32(), block))
		}

		mockBabylonClient, mockReporter := newMockReporter(t, ctrl)

		// a random number of blocks exists on chain
		numBlocksOnChain := r.Intn(int(numBlocks))
		mockBabylonClient.EXPECT().ContainsBTCBlock(gomock.Any()).Return(
			&btclctypes.QueryContainsBytesResponse{Contains: true}, nil).Times(numBlocksOnChain)
		mockBabylonClient.EXPECT().ContainsBTCBlock(gomock.Any()).Return(
			&btclctypes.QueryContainsBytesResponse{Contains: false}, nil).AnyTimes()

		// inserting header will always be successful
		mockBabylonClient.EXPECT().InsertHeaders(gomock.Any(), gomock.Any()).Return(&babylonclient.RelayerTxResponse{Code: 0}, nil).AnyTimes()

		// if Babylon client contains this block, numSubmitted has to be 0, otherwise 1
		numSubmitted, err := mockReporter.ProcessHeaders("", ibs)
		require.Equal(t, int(numBlocks)-numBlocksOnChain, numSubmitted)
		require.NoError(t, err)
	})
}

// FuzzProcessCheckpoints fuzz tests ProcessCheckpoints()
// - Data: a number of random blocks, with or without Babylon txs
// - Tested property: for any BTC block, if it contains Babylon data, then it will extract checkpoint segments, do a match, and report matched checkpoints
func FuzzProcessCheckpoints(f *testing.F) {
	datagen.AddRandomSeedsToFuzzer(f, 100)

	f.Fuzz(func(t *testing.T, seed int64) {
		t.Parallel()
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		r := rand.New(rand.NewSource(seed))

		mockBabylonClient, mockReporter := newMockReporter(t, ctrl)
		// inserting SPV proofs is always successful
		mockBabylonClient.EXPECT().InsertBTCSpvProof(gomock.Any(), gomock.Any()).Return(&babylonclient.RelayerTxResponse{Code: 0}, nil).AnyTimes()

		// generate a random number of blocks, with or without Babylon txs
		numBlocks := datagen.RandomInt(r, 100)
		blocks, numCkptSegsExpected, rawCkpts := vdatagen.GenRandomBlockchainWithBabylonTx(r, numBlocks, 0.3, 0.4)
		ibs := []*types.IndexedBlock{}
		numMatchedCkptsExpected := 0
		for i, block := range blocks {
			ibs = append(ibs, types.NewIndexedBlockFromMsgBlock(r.Uint32(), block))
			if rawCkpts[i] != nil {
				numMatchedCkptsExpected++
			}
		}

		numCkptSegs, numMatchedCkpts := mockReporter.ProcessCheckpoints("", ibs)
		require.Equal(t, numCkptSegsExpected, numCkptSegs)
		require.Equal(t, numMatchedCkptsExpected, numMatchedCkpts)
	})
}
