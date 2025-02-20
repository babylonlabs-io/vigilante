//go:build e2e
// +build e2e

package e2etest

import (
	"github.com/babylonlabs-io/vigilante/testutil"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"math/rand"
	"testing"
	"time"

	"github.com/babylonlabs-io/babylon/testutil/datagen"
	btcctypes "github.com/babylonlabs-io/babylon/x/btccheckpoint/types"
	checkpointingtypes "github.com/babylonlabs-io/babylon/x/checkpointing/types"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/babylonlabs-io/vigilante/metrics"
	"github.com/babylonlabs-io/vigilante/submitter"
)

func TestSubmitterSubmission(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(time.Now().Unix()))
	numMatureOutputs := uint32(300)

	tm := StartManager(t, numMatureOutputs, defaultEpochInterval)
	defer tm.Stop(t)

	randomCheckpoint := datagen.GenRandomRawCheckpointWithMeta(r)
	randomCheckpoint.Status = checkpointingtypes.Sealed
	randomCheckpoint.Ckpt.EpochNum = 1

	ctl := gomock.NewController(t)
	mockBabylonClient := submitter.NewMockBabylonQueryClient(ctl)
	subAddr, _ := sdk.AccAddressFromBech32(submitterAddrStr)

	mockBabylonClient.EXPECT().BTCCheckpointParams().Return(
		&btcctypes.QueryParamsResponse{
			Params: btcctypes.Params{
				CheckpointTag:                 babylonTagHex,
				BtcConfirmationDepth:          2,
				CheckpointFinalizationTimeout: 4,
			},
		}, nil)
	mockBabylonClient.EXPECT().RawCheckpointList(gomock.Any(), gomock.Any()).Return(
		&checkpointingtypes.QueryRawCheckpointListResponse{
			RawCheckpoints: []*checkpointingtypes.RawCheckpointWithMetaResponse{
				randomCheckpoint.ToResponse(),
			},
		}, nil).AnyTimes()

	tm.Config.Submitter.PollingIntervalSeconds = 2

	// create submitter
	vigilantSubmitter, _ := submitter.New(
		&tm.Config.Submitter,
		logger,
		tm.BTCClient,
		mockBabylonClient,
		subAddr,
		tm.Config.Common.RetrySleepTime,
		tm.Config.Common.MaxRetrySleepTime,
		tm.Config.Common.MaxRetryTimes,
		metrics.NewSubmitterMetrics(),
		testutil.MakeTestBackend(t),
		tm.Config.BTC.WalletName,
	)

	vigilantSubmitter.Start()

	defer func() {
		vigilantSubmitter.Stop()
		vigilantSubmitter.WaitForShutdown()
	}()

	// wait for our 2 op_returns with epoch 1 checkpoint to hit the mempool
	var mempoolTxs []*chainhash.Hash
	require.Eventually(t, func() bool {
		var err error
		mempoolTxs, err = tm.BTCClient.GetRawMempool()
		require.NoError(t, err)
		return len(mempoolTxs) > 1
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	require.NotNil(t, mempoolTxs)

	require.Eventually(t, func() bool {
		return len(tm.RetrieveTransactionFromMempool(t, mempoolTxs)) == 2
	}, eventuallyWaitTimeOut, eventuallyPollTime)

	// mine a block with those transactions
	blockWithOpReturnTransactions := tm.mineBlock(t)
	// block should have 3 transactions, 2 from submitter and 1 coinbase
	require.Equal(t, len(blockWithOpReturnTransactions.Transactions), 3)
}

func TestSubmitterSubmissionReplace(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(time.Now().Unix()))
	numMatureOutputs := uint32(300)

	tm := StartManager(t, numMatureOutputs, defaultEpochInterval)
	defer tm.Stop(t)

	randomCheckpoint := datagen.GenRandomRawCheckpointWithMeta(r)
	randomCheckpoint.Status = checkpointingtypes.Sealed
	randomCheckpoint.Ckpt.EpochNum = 1

	ctl := gomock.NewController(t)
	mockBabylonClient := submitter.NewMockBabylonQueryClient(ctl)
	subAddr, _ := sdk.AccAddressFromBech32(submitterAddrStr)

	mockBabylonClient.EXPECT().BTCCheckpointParams().Return(
		&btcctypes.QueryParamsResponse{
			Params: btcctypes.Params{
				CheckpointTag:                 babylonTagHex,
				BtcConfirmationDepth:          2,
				CheckpointFinalizationTimeout: 4,
			},
		}, nil)
	mockBabylonClient.EXPECT().RawCheckpointList(gomock.Any(), gomock.Any()).Return(
		&checkpointingtypes.QueryRawCheckpointListResponse{
			RawCheckpoints: []*checkpointingtypes.RawCheckpointWithMetaResponse{
				randomCheckpoint.ToResponse(),
			},
		}, nil).AnyTimes()

	tm.Config.Submitter.PollingIntervalSeconds = 2
	tm.Config.Submitter.ResendIntervalSeconds = 2
	tm.Config.Submitter.ResubmitFeeMultiplier = 2.1
	// create submitter
	vigilantSubmitter, _ := submitter.New(
		&tm.Config.Submitter,
		logger,
		tm.BTCClient,
		mockBabylonClient,
		subAddr,
		tm.Config.Common.RetrySleepTime,
		tm.Config.Common.MaxRetrySleepTime,
		tm.Config.Common.MaxRetryTimes,
		metrics.NewSubmitterMetrics(),
		testutil.MakeTestBackend(t),
		tm.Config.BTC.WalletName,
	)

	vigilantSubmitter.Start()

	defer func() {
		vigilantSubmitter.Stop()
		vigilantSubmitter.WaitForShutdown()
	}()

	// wait for our 2 op_returns with epoch 1 checkpoint to hit the mempool and then
	// retrieve them from there
	txsMap := make(map[string]struct{})
	var sendTransactions []*btcutil.Tx

	var mempoolTxs []*chainhash.Hash
	require.Eventually(t, func() bool {
		var err error
		mempoolTxs, err = tm.BTCClient.GetRawMempool()
		require.NoError(t, err)
		for _, hash := range mempoolTxs {
			hashStr := hash.String()
			if _, exists := txsMap[hashStr]; !exists {
				tx, err := tm.BTCClient.GetRawTransaction(hash)
				require.NoError(t, err)
				txsMap[hashStr] = struct{}{}
				sendTransactions = append(sendTransactions, tx)
			}
		}
		return len(txsMap) == 3
	}, eventuallyWaitTimeOut, 50*time.Millisecond)

	resendTx2 := sendTransactions[2]

	// Here check that sendTransactions1 are replacements for sendTransactions, i.e they should have:
	// 1. same
	// 2. outputs with different values
	// 3. different signatures
	require.Equal(t, sendTransactions[1].MsgTx().TxIn[0].PreviousOutPoint, resendTx2.MsgTx().TxIn[0].PreviousOutPoint)
	require.Less(t, resendTx2.MsgTx().TxOut[1].Value, sendTransactions[1].MsgTx().TxOut[1].Value)
	require.NotEqual(t, sendTransactions[1].MsgTx().TxIn[0].Witness[0], resendTx2.MsgTx().TxIn[0].Witness[0])

	// mine a block with those replacement transactions just to be sure they execute correctly
	blockWithOpReturnTransactions := tm.mineBlock(t)
	// block should have 2 transactions, 1 from submitter and 1 coinbase
	require.Equal(t, len(blockWithOpReturnTransactions.Transactions), 3)
	require.True(t, promtestutil.ToFloat64(vigilantSubmitter.Metrics().ResentCheckpointsCounter) == 1)
}
