package relayer

import (
	"errors"
	"github.com/babylonlabs-io/vigilante/submitter/store"
	"github.com/babylonlabs-io/vigilante/testutil/mocks"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_maybeResendFromStore(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                string
		epoch               uint64
		getLatestCheckpoint GetLatestCheckpointFunc
		getRawTransaction   GetRawTransactionFunc
		sendTransaction     SendTransactionFunc
		expectedResult      bool
		expectedError       error
	}{
		{
			name:  "Checkpoint not found",
			epoch: 123,
			getLatestCheckpoint: func() (*store.StoredCheckpoint, bool, error) {
				return nil, false, nil
			},
			getRawTransaction: func(_ *chainhash.Hash) (*btcutil.Tx, error) {
				return nil, nil
			},
			sendTransaction: func(_ *wire.MsgTx) (*chainhash.Hash, error) {
				return nil, nil
			},
			expectedResult: false,
			expectedError:  nil,
		},
		{
			name:  "Error retrieving checkpoint",
			epoch: 123,
			getLatestCheckpoint: func() (*store.StoredCheckpoint, bool, error) {
				return nil, false, errors.New("checkpoint error")
			},
			getRawTransaction: func(_ *chainhash.Hash) (*btcutil.Tx, error) {
				return nil, nil
			},
			sendTransaction: func(_ *wire.MsgTx) (*chainhash.Hash, error) {
				return nil, nil
			},
			expectedResult: false,
			expectedError:  errors.New("checkpoint error"),
		},
		{
			name:  "Epoch mismatch",
			epoch: 123,
			getLatestCheckpoint: func() (*store.StoredCheckpoint, bool, error) {
				return &store.StoredCheckpoint{Epoch: 456, Tx1: &wire.MsgTx{}, Tx2: &wire.MsgTx{}}, true, nil
			},
			getRawTransaction: func(_ *chainhash.Hash) (*btcutil.Tx, error) {
				return nil, nil
			},
			sendTransaction: func(_ *wire.MsgTx) (*chainhash.Hash, error) {
				return nil, nil
			},
			expectedResult: false,
			expectedError:  nil,
		},
		{
			name:  "Successful resends",
			epoch: 123,
			getLatestCheckpoint: func() (*store.StoredCheckpoint, bool, error) {
				return &store.StoredCheckpoint{Epoch: 123, Tx1: &wire.MsgTx{}, Tx2: &wire.MsgTx{}}, true, nil
			},
			getRawTransaction: func(_ *chainhash.Hash) (*btcutil.Tx, error) {
				return nil, &btcjson.RPCError{Code: btcjson.ErrRPCNoTxInfo, Message: "transaction not found"}
			},
			sendTransaction: func(_ *wire.MsgTx) (*chainhash.Hash, error) {
				return &chainhash.Hash{}, nil // Simulate successful send
			},
			expectedResult: true,
			expectedError:  nil,
		},
		{
			name:  "Error resending transaction",
			epoch: 123,
			getLatestCheckpoint: func() (*store.StoredCheckpoint, bool, error) {
				return &store.StoredCheckpoint{Epoch: 123, Tx1: &wire.MsgTx{}, Tx2: &wire.MsgTx{}}, true, nil
			},
			getRawTransaction: func(_ *chainhash.Hash) (*btcutil.Tx, error) {
				return nil, &btcjson.RPCError{Code: btcjson.ErrRPCNoTxInfo, Message: "transaction not found"}
			},
			sendTransaction: func(_ *wire.MsgTx) (*chainhash.Hash, error) {
				return nil, errors.New("send error")
			},
			expectedResult: false,
			expectedError:  errors.New("send error"),
		},
		{
			name:  "Transaction resend returns ErrRPCTxAlreadyInChain",
			epoch: 123,
			getLatestCheckpoint: func() (*store.StoredCheckpoint, bool, error) {
				return &store.StoredCheckpoint{Epoch: 123, Tx1: &wire.MsgTx{}, Tx2: &wire.MsgTx{}}, true, nil
			},
			getRawTransaction: func(_ *chainhash.Hash) (*btcutil.Tx, error) {
				return &btcutil.Tx{}, nil
			},
			sendTransaction: func(_ *wire.MsgTx) (*chainhash.Hash, error) {
				return nil, &btcjson.RPCError{Code: btcjson.ErrRPCTxAlreadyInChain, Message: "tx already in chain"}
			},
			expectedResult: true,
			expectedError:  nil,
		},
		{
			name:  "Network error in getRawTransaction",
			epoch: 123,
			getLatestCheckpoint: func() (*store.StoredCheckpoint, bool, error) {
				return &store.StoredCheckpoint{Epoch: 123, Tx1: &wire.MsgTx{}, Tx2: &wire.MsgTx{}}, true, nil
			},
			getRawTransaction: func(_ *chainhash.Hash) (*btcutil.Tx, error) {
				return nil, errors.New("network error")
			},
			sendTransaction: func(_ *wire.MsgTx) (*chainhash.Hash, error) {
				return nil, errors.New("should not be called")
			},
			expectedResult: false,
			expectedError:  errors.New("network error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := maybeResendFromStore(tt.epoch, tt.getLatestCheckpoint, tt.getRawTransaction, tt.sendTransaction)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestRelayer_VerifyRBFRequirements(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		newFee            btcutil.Amount
		mempoolEntryErr   error
		mempoolEntry      *btcjson.GetMempoolEntryResult
		networkInfoErr    error
		networkInfo       *btcjson.GetNetworkInfoResult
		expectedErrSubstr string
	}{
		{
			name:              "transaction not in mempool",
			newFee:            btcutil.Amount(2000),
			mempoolEntryErr:   errors.New("tx not found"),
			mempoolEntry:      nil,
			expectedErrSubstr: "not found in mempool",
		},
		{
			name:   "descendant count exceeds limit",
			newFee: btcutil.Amount(2000),
			mempoolEntry: &btcjson.GetMempoolEntryResult{
				DescendantCount: 101,
			},
			expectedErrSubstr: "too many descendant transactions",
		},
		{
			name:   "new fee not greater than original fees",
			newFee: btcutil.Amount(2000),
			mempoolEntry: &btcjson.GetMempoolEntryResult{
				DescendantCount: 50,
				DescendantFees:  2000, // Same as newFee
				DescendantSize:  500,
			},
			expectedErrSubstr: "insufficient fee",
		},
		{
			name:   "new feerate not greater than original feerate",
			newFee: btcutil.Amount(2000),
			mempoolEntry: &btcjson.GetMempoolEntryResult{
				DescendantCount: 50,
				DescendantFees:  1000, // Lower than newFee
				DescendantSize:  125,  // But better feerate (8 sat/vB vs newFee's 8 sat/vB)
			},
			expectedErrSubstr: "insufficient feerate",
		},
		{
			name:   "failed to get network info",
			newFee: btcutil.Amount(2000),
			mempoolEntry: &btcjson.GetMempoolEntryResult{
				DescendantCount: 50,
				DescendantFees:  1000,
				DescendantSize:  500, // Lower feerate than newFee (2 sat/vB vs 8 sat/vB)
			},
			networkInfoErr:    errors.New("network info error"),
			expectedErrSubstr: "failed to get network info",
		},
		{
			name:   "fee increment too small",
			newFee: btcutil.Amount(2000),
			mempoolEntry: &btcjson.GetMempoolEntryResult{
				DescendantCount: 50,
				DescendantFees:  1000,
				DescendantSize:  500, // Lower feerate than newFee (2 sat/vB vs 8 sat/vB)
			},
			networkInfo: &btcjson.GetNetworkInfoResult{
				IncrementalFee: 5, // 5 sat/vB * 250 vB = 1250 sats required increment
				// newFee - originalFees = 2000 - 1000 = 1000 < 1250 (required increment)
			},
			expectedErrSubstr: "fee increment too small",
		},
		{
			name:   "all RBF requirements met",
			newFee: btcutil.Amount(2000),
			mempoolEntry: &btcjson.GetMempoolEntryResult{
				DescendantCount: 50,
				DescendantFees:  1000,
				DescendantSize:  500, // Lower feerate than newFee (2 sat/vB vs 8 sat/vB)
			},
			networkInfo: &btcjson.GetNetworkInfoResult{
				IncrementalFee: 2, // 2 sat/vB * 250 vB = 500 sats required increment
				// newFee - originalFees = 2000 - 1000 = 1000 > 500 (required increment)
			},
			expectedErrSubstr: "", // No error expected
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBTCWallet := mocks.NewMockBTCWallet(ctrl)

			relayer := &Relayer{
				BTCWallet: mockBTCWallet,
			}

			txID := "abc123"
			txVirtualSize := int64(250)
			// Set up GetMempoolEntry expectations
			if tc.mempoolEntryErr != nil {
				mockBTCWallet.EXPECT().
					GetMempoolEntry(gomock.Eq(txID)).
					Return(nil, tc.mempoolEntryErr)
			} else {
				mockBTCWallet.EXPECT().
					GetMempoolEntry(gomock.Eq(txID)).
					Return(tc.mempoolEntry, nil)
			}

			// Set up GetNetworkInfo expectations only if we expect to get that far
			if tc.mempoolEntryErr == nil && tc.mempoolEntry.DescendantCount <= 100 &&
				tc.newFee > btcutil.Amount(tc.mempoolEntry.DescendantFees) &&
				float64(tc.newFee)/float64(txVirtualSize) > tc.mempoolEntry.DescendantFees/float64(tc.mempoolEntry.DescendantSize) {

				mockBTCWallet.EXPECT().
					GetNetworkInfo().
					Return(tc.networkInfo, tc.networkInfoErr)
			}

			// Call the function being tested
			err := relayer.verifyRBFRequirements(txID, tc.newFee, txVirtualSize)

			// Assert the results
			if tc.expectedErrSubstr == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrSubstr)
			}
		})
	}
}
