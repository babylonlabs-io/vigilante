package reporter_test

import (
	"math/rand"
	"testing"

	"github.com/babylonlabs-io/babylon/v4/client/babylonclient"
	"github.com/lightningnetwork/lnd/lntest/mock"

	"github.com/babylonlabs-io/babylon/v4/testutil/datagen"
	btcctypes "github.com/babylonlabs-io/babylon/v4/x/btccheckpoint/types"
	btclctypes "github.com/babylonlabs-io/babylon/v4/x/btclightclient/types"
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

	// create backend
	backend := reporter.NewBabylonBackend(mockBabylonClient)

	r, err := reporter.New(
		&cfg.Reporter,
		logger,
		mockBTCClient,
		backend,
		mockBabylonClient,
		&mockNotifier,
		cfg.Common.RetrySleepTime,
		cfg.Common.MaxRetrySleepTime,
		cfg.Common.MaxRetryTimes,
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

		// mock MustGetAddr for header submission
		mockBabylonClient.EXPECT().MustGetAddr().Return("test-address").AnyTimes()

		// mock BTCHeaderChainTip for GetTip check before retry
		// Return height 0 so submissions proceed (tip < endHeight)
		mockBabylonClient.EXPECT().BTCHeaderChainTip().Return(
			&btclctypes.QueryTipResponse{
				Header: &btclctypes.BTCHeaderInfoResponse{Height: 0, HashHex: "0000000000000000000000000000000000000000000000000000000000000000"}},
			nil,
		).AnyTimes()

		// inserting header will always be successful
		mockBabylonClient.EXPECT().ReliablySendMsg(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&babylonclient.RelayerTxResponse{Code: 0}, nil).AnyTimes()

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
		mockBabylonClient.EXPECT().ReliablySendMsg(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&babylonclient.RelayerTxResponse{Code: 0}, nil).AnyTimes()

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

func makeBlocks(heights ...uint32) []*types.IndexedBlock {
	blocks := make([]*types.IndexedBlock, len(heights))
	for i, h := range heights {
		blocks[i] = &types.IndexedBlock{
			Height: h,
		}
	}

	return blocks
}

func TestFilterAlreadyProcessedBlocks(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                   string
		inputBlocks            []*types.IndexedBlock
		inputHeight            uint32
		expectedBlocks         []*types.IndexedBlock
		expectedKeepProcessing bool
	}{
		{
			name:                   "Empty input batch",
			inputBlocks:            []*types.IndexedBlock{},
			inputHeight:            10,
			expectedBlocks:         []*types.IndexedBlock{},
			expectedKeepProcessing: false,
		},
		{
			name:                   "Nil input batch",
			inputBlocks:            nil,
			inputHeight:            10,
			expectedBlocks:         []*types.IndexedBlock{},
			expectedKeepProcessing: false,
		},
		{
			name:                   "Chain tip far ahead of batch",
			inputBlocks:            makeBlocks(10, 11, 12),
			inputHeight:            20,
			expectedBlocks:         nil,
			expectedKeepProcessing: false,
		},
		{
			name:                   "Chain tip equals last block height",
			inputBlocks:            makeBlocks(10, 11, 12),
			inputHeight:            12,
			expectedBlocks:         nil,
			expectedKeepProcessing: false,
		},
		{
			name:                   "Chain tip catches some blocks",
			inputBlocks:            makeBlocks(10, 11, 12, 13, 14),
			inputHeight:            11,
			expectedBlocks:         makeBlocks(12, 13, 14),
			expectedKeepProcessing: true,
		},
		{
			name:                   "Chain tip equals first block height",
			inputBlocks:            makeBlocks(10, 11, 12),
			inputHeight:            10,
			expectedBlocks:         makeBlocks(11, 12),
			expectedKeepProcessing: true,
		},
		{
			name:                   "Chain tip before all blocks",
			inputBlocks:            makeBlocks(10, 11, 12),
			inputHeight:            5,
			expectedBlocks:         makeBlocks(10, 11, 12),
			expectedKeepProcessing: true,
		},
		{
			name:                   "Chain tip catches all but last block",
			inputBlocks:            makeBlocks(10, 11, 12),
			inputHeight:            11,
			expectedBlocks:         makeBlocks(12),
			expectedKeepProcessing: true,
		},
		{
			name:                   "Single block batch - already processed",
			inputBlocks:            makeBlocks(10),
			inputHeight:            10,
			expectedBlocks:         nil,
			expectedKeepProcessing: false,
		},
		{
			name:                   "Single block batch - not processed",
			inputBlocks:            makeBlocks(10),
			inputHeight:            9,
			expectedBlocks:         makeBlocks(10),
			expectedKeepProcessing: true,
		},
		{
			name:                   "All blocks <= height",
			inputBlocks:            makeBlocks(8, 9, 10),
			inputHeight:            10,
			expectedBlocks:         nil,
			expectedKeepProcessing: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotBlocks, gotKeepProcessing := reporter.FilterAlreadyProcessedBlocks(tc.inputBlocks, tc.inputHeight)

			require.Equal(t, tc.expectedKeepProcessing, gotKeepProcessing, "KeepProcessing flag mismatch")
			require.Equal(t, tc.expectedBlocks, gotBlocks, "Blocks slice mismatch")

			if tc.expectedBlocks == nil {
				require.Nil(t, gotBlocks, "Expected nil blocks slice")
			} else {
				require.NotNil(t, gotBlocks, "Expected non-nil blocks slice")
				require.Len(t, gotBlocks, len(tc.expectedBlocks), "Blocks slice length mismatch")
			}
		})
	}
}
