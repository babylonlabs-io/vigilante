package relayer

import (
	"errors"
	"fmt"
	"github.com/babylonlabs-io/babylon/testutil/datagen"
	"github.com/babylonlabs-io/vigilante/btcclient"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/submitter/store"
	"github.com/babylonlabs-io/vigilante/testutil/mocks"
	"github.com/babylonlabs-io/vigilante/types"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/golang/mock/gomock"
	"github.com/lightningnetwork/lnd/lnwallet/chainfee"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"math/rand"
	"testing"
	"time"
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
		{
			name:  "Tx already in chain",
			epoch: 123,
			getLatestCheckpoint: func() (*store.StoredCheckpoint, bool, error) {
				return &store.StoredCheckpoint{Epoch: 123, Tx1: &wire.MsgTx{}, Tx2: &wire.MsgTx{}}, true, nil
			},
			getRawTransaction: func(_ *chainhash.Hash) (*btcutil.Tx, error) {
				return nil, btcjson.NewRPCError(btcjson.ErrRPCNoTxInfo, "transaction not found")
			},
			sendTransaction: func(_ *wire.MsgTx) (*chainhash.Hash, error) {
				return nil, btcjson.NewRPCError(btcjson.ErrRPCTxAlreadyInChain, "tx already in chain")
			},
			expectedResult: true,
			expectedError:  nil,
		},
		{
			name:  "err raw tx string",
			epoch: 123,
			getLatestCheckpoint: func() (*store.StoredCheckpoint, bool, error) {
				return &store.StoredCheckpoint{Epoch: 123, Tx1: &wire.MsgTx{}, Tx2: &wire.MsgTx{}}, true, nil
			},
			getRawTransaction: func(_ *chainhash.Hash) (*btcutil.Tx, error) {
				return nil, btcjson.NewRPCError(btcjson.ErrRPCNoTxInfo, "transaction not found")
			},
			sendTransaction: func(_ *wire.MsgTx) (*chainhash.Hash, error) {
				return nil, btcjson.NewRPCError(btcjson.ErrRPCRawTxString, "err raw tx string")
			},
			expectedResult: false,
			expectedError: fmt.Errorf("fatal error: invalid transaction format: %w",
				btcjson.NewRPCError(btcjson.ErrRPCRawTxString, "err raw tx string")),
		},
		{
			name:  "err rpc tx",
			epoch: 123,
			getLatestCheckpoint: func() (*store.StoredCheckpoint, bool, error) {
				return &store.StoredCheckpoint{Epoch: 123, Tx1: &wire.MsgTx{}, Tx2: &wire.MsgTx{}}, true, nil
			},
			getRawTransaction: func(_ *chainhash.Hash) (*btcutil.Tx, error) {
				return nil, btcjson.NewRPCError(btcjson.ErrRPCNoTxInfo, "transaction not found")
			},
			sendTransaction: func(_ *wire.MsgTx) (*chainhash.Hash, error) {
				return nil, btcjson.NewRPCError(btcjson.ErrRPCTxError, "err rpc tx")
			},
			expectedResult: false,
			expectedError: fmt.Errorf("transaction error: %w",
				btcjson.NewRPCError(btcjson.ErrRPCTxError, "err rpc tx")),
		},
		{
			name:  "err tx rejected",
			epoch: 123,
			getLatestCheckpoint: func() (*store.StoredCheckpoint, bool, error) {
				return &store.StoredCheckpoint{Epoch: 123, Tx1: &wire.MsgTx{}, Tx2: &wire.MsgTx{}}, true, nil
			},
			getRawTransaction: func(_ *chainhash.Hash) (*btcutil.Tx, error) {
				return nil, btcjson.NewRPCError(btcjson.ErrRPCNoTxInfo, "transaction not found")
			},
			sendTransaction: func(_ *wire.MsgTx) (*chainhash.Hash, error) {
				return nil, btcjson.NewRPCError(btcjson.ErrRPCTxRejected, "err rpc tx rejected")
			},
			expectedResult: false,
			expectedError: fmt.Errorf("transaction rejected: %w",
				btcjson.NewRPCError(btcjson.ErrRPCTxRejected, "err rpc tx rejected")),
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

// TestCalculateBumpedFee tests the calculateBumpedFee function
// nolint:paralleltest
func TestCalculateBumpedFee(t *testing.T) {
	ctrl := gomock.NewController(t)

	t.Cleanup(func() {
		ctrl.Finish()
	})

	mockBTCWallet := mocks.NewMockBTCWallet(ctrl)

	logger, _ := zap.NewDevelopment()
	sugaredLogger := logger.Sugar()

	// Create sample checkpoint info
	createCheckpointInfo := func() *types.CheckpointInfo {
		return &types.CheckpointInfo{
			Epoch: 123,
			TS:    time.Now(),
			Tx2: &types.BtcTxInfo{
				Size: 250,
				Fee:  btcutil.Amount(1000),
				TxID: &chainhash.Hash{1, 2, 3}, // Sample hash
			},
		}
	}

	createRelayer := func(config *config.SubmitterConfig, feeRate chainfee.SatPerKVByte, estimatorErr error) *Relayer {
		mockEstimator := &MockEstimator{
			estimateFeePerKWFn: func(_ uint32) (chainfee.SatPerKWeight, error) {
				if estimatorErr != nil {
					return 0, estimatorErr
				}
				// Convert SatPerKVByte to SatPerKWeight
				return chainfee.SatPerKWeight(feeRate / 4), nil
			},
		}

		btcConfig := &MockBTCConfig{
			targetBlockNum: 6,
			defaultFee:     chainfee.SatPerKVByte(10000),
			txFeeMin:       chainfee.SatPerKVByte(1000),
			txFeeMax:       chainfee.SatPerKVByte(100000),
		}

		mockBTCWallet.EXPECT().GetBTCConfig().Return(btcConfig.GetBTCConfig()).AnyTimes()

		return &Relayer{
			Estimator: mockEstimator,
			BTCWallet: mockBTCWallet,
			config:    config,
			logger:    sugaredLogger,
		}
	}
	// nolint:paralleltest
	t.Run("Basic fee calculation without previous failure", func(t *testing.T) {
		ckptInfo := createCheckpointInfo()
		submitterConfig := &config.SubmitterConfig{
			ResubmitFeeMultiplier: 1.5,
		}
		// 10000 sat/kvB = 10 sat/B
		rl := createRelayer(submitterConfig, 10000, nil)

		bumpedFee, err := rl.calculateBumpedFee(ckptInfo, nil)
		require.NoError(t, err)

		expectedRequiredFee := btcutil.Amount(2500) // 10 * 250
		require.Equal(t, expectedRequiredFee, bumpedFee)
	})
	// nolint:paralleltest
	t.Run("Basic fee calculation with 'other' error type", func(t *testing.T) {
		ckptInfo := createCheckpointInfo()
		submitterConfig := &config.SubmitterConfig{
			ResubmitFeeMultiplier: 1.2,
		}
		// 5000 sat/kvB = 5 sat/B
		rl := createRelayer(submitterConfig, 5000, nil)

		someError := errors.New("other")
		bumpedFee, err := rl.calculateBumpedFee(ckptInfo, someError)
		require.NoError(t, err)

		expectedRequiredFee := btcutil.Amount(1250) // 5 * 250
		require.Equal(t, expectedRequiredFee, bumpedFee)
	})
	// nolint:paralleltest
	t.Run("Insufficient fee error", func(t *testing.T) {
		ckptInfo := createCheckpointInfo()
		submitterConfig := &config.SubmitterConfig{
			ResubmitFeeMultiplier: 1.2,
			InsufficientFeeMargin: 0.25,
		}
		// 5000 sat/kvB = 5 sat/B
		rl := createRelayer(submitterConfig, 5000, nil)

		mempoolEntry := &btcjson.GetMempoolEntryResult{
			DescendantFees: 2000,
		}
		mockBTCWallet.EXPECT().GetMempoolEntry(ckptInfo.Tx2.TxID.String()).Return(mempoolEntry, nil).AnyTimes()

		bumpedFee, err := rl.calculateBumpedFee(ckptInfo, ErrInsufficientFee)
		require.NoError(t, err)

		expectedAdjustedFee := btcutil.Amount(2500) // 2000 * (1 + 0.25)
		require.Equal(t, expectedAdjustedFee, bumpedFee)
	})
	// nolint:paralleltest
	t.Run("Insufficient feerate error", func(t *testing.T) {
		ckptInfo := createCheckpointInfo()
		submitterConfig := &config.SubmitterConfig{
			ResubmitFeeMultiplier:     1.2,
			InsufficientFeerateMargin: 0.3,
		}
		// 8000 sat/kvB = 8 sat/B
		rl := createRelayer(submitterConfig, 8000, nil)

		mempoolEntry := &btcjson.GetMempoolEntryResult{
			DescendantFees: 1500,
			DescendantSize: 300,
			Fee:            0.00001,
		}
		mockBTCWallet.EXPECT().GetMempoolEntry(ckptInfo.Tx2.TxID.String()).Return(mempoolEntry, nil).AnyTimes()

		bumpedFee, err := rl.calculateBumpedFee(ckptInfo, ErrInsufficientFeerate)
		require.NoError(t, err)

		expectedRequiredFee := btcutil.Amount(2000) // 8 * 250
		require.Equal(t, expectedRequiredFee, bumpedFee)
	})
	// nolint:paralleltest
	t.Run("Fee increment too small error", func(t *testing.T) {
		ckptInfo := createCheckpointInfo()
		submitterConfig := &config.SubmitterConfig{
			ResubmitFeeMultiplier: 1.2,
			FeeIncrementMargin:    0.2,
		}
		// 6000 sat/kvB = 6 sat/B
		rl := createRelayer(submitterConfig, 6000, nil)

		mempoolEntry := &btcjson.GetMempoolEntryResult{
			Fee: 0.000012, // 1200 satoshis
		}
		mockBTCWallet.EXPECT().GetMempoolEntry(ckptInfo.Tx2.TxID.String()).Return(mempoolEntry, nil).AnyTimes()

		networkInfo := &btcjson.GetNetworkInfoResult{
			IncrementalFee: 1.0, // 1 sat/byte
		}
		mockBTCWallet.EXPECT().GetNetworkInfo().Return(networkInfo, nil)

		bumpedFee, err := rl.calculateBumpedFee(ckptInfo, ErrFeeIncrementTooSmall)
		require.NoError(t, err)

		expectedRequiredFee := btcutil.Amount(1500) // 6 * 250
		assert.Equal(t, expectedRequiredFee, bumpedFee)
	})
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
			expectedErrSubstr: "feerate insufficient",
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

// MockEstimator implements the chainfee.Estimator interface for testing
type MockEstimator struct {
	estimateFeePerKWFn func(uint32) (chainfee.SatPerKWeight, error)
	relayFeePerKWFn    func() chainfee.SatPerKWeight
}

func (m *MockEstimator) Start() error {
	return nil
}

func (m *MockEstimator) Stop() error {
	return nil
}

func (m *MockEstimator) RelayFeePerKW() chainfee.SatPerKWeight {
	return chainfee.SatPerKWeight(1000)
}

func (m *MockEstimator) EstimateFeePerKW(confirmationTarget uint32) (chainfee.SatPerKWeight, error) {
	return m.estimateFeePerKWFn(confirmationTarget)
}

// MockBTCConfig is a mock implementation of BTCConfig for testing
type MockBTCConfig struct {
	targetBlockNum int64
	defaultFee     chainfee.SatPerKVByte
	txFeeMin       chainfee.SatPerKVByte
	txFeeMax       chainfee.SatPerKVByte
}

func (m *MockBTCConfig) GetBTCConfig() *config.BTCConfig {
	return &config.BTCConfig{
		TargetBlockNum: m.targetBlockNum,
		DefaultFee:     m.defaultFee,
		TxFeeMin:       m.txFeeMin,
		TxFeeMax:       m.txFeeMax,
	}
}

// nolint:maintidx
func TestRelayer_BuildDataTx(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                string
		data                []byte
		setupMocks          func(*mocks.MockBTCWallet)
		expectedErrContains string
	}{
		{
			name: "successful transaction with change",
			data: []byte("test data"),
			setupMocks: func(m *mocks.MockBTCWallet) {
				tx := wire.NewMsgTx(wire.TxVersion)
				// Add OP_RETURN output
				builder := txscript.NewScriptBuilder()
				dataScript, err := builder.AddOp(txscript.OP_RETURN).AddData([]byte("test data")).Script()
				require.NoError(t, err)
				tx.AddTxOut(wire.NewTxOut(0, dataScript))

				// Create a funded tx with change output
				fundedTx := wire.NewMsgTx(wire.TxVersion)
				fundedTx.AddTxOut(wire.NewTxOut(0, dataScript)) // OP_RETURN output

				// Add an input
				hash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
				fundedTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(hash, 0), nil, nil))
				r := rand.New(rand.NewSource(time.Now().UnixMilli()))

				address, err := datagen.GenRandomBTCAddress(r, &chaincfg.RegressionNetParams)
				require.NoError(t, err)
				pkScript, err := txscript.PayToAddrScript(address)
				require.NoError(t, err)

				fundedTx.AddTxOut(wire.NewTxOut(10000, pkScript)) // Change output

				btcConfig := &MockBTCConfig{
					targetBlockNum: 6,
					defaultFee:     chainfee.SatPerKVByte(5000),
					txFeeMin:       chainfee.SatPerKVByte(1000),
					txFeeMax:       chainfee.SatPerKVByte(100000),
				}
				m.EXPECT().GetBTCConfig().Return(btcConfig.GetBTCConfig()).AnyTimes()

				m.EXPECT().
					FundRawTransaction(gomock.Any(), gomock.Any(), gomock.Nil()).
					Return(&btcjson.FundRawTransactionResult{
						Transaction:    fundedTx,
						Fee:            1000,
						ChangePosition: 1,
					}, nil)
			},
		},
		{
			name: "successful transaction without change, add change manually",
			data: []byte("test data"),
			setupMocks: func(m *mocks.MockBTCWallet) {
				r := rand.New(rand.NewSource(time.Now().UnixMilli()))

				address, err := datagen.GenRandomBTCAddress(r, &chaincfg.RegressionNetParams)
				require.NoError(t, err)
				builder := txscript.NewScriptBuilder()
				dataScript, err := builder.AddOp(txscript.OP_RETURN).AddData([]byte("test data")).Script()
				require.NoError(t, err)
				hash, err := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
				require.NoError(t, err)

				// ensure calls happen in sequence
				gomock.InOrder(
					// First call to FundRawTransaction - returns tx without change
					m.EXPECT().
						FundRawTransaction(gomock.Any(), gomock.Any(), gomock.Nil()).
						DoAndReturn(func(_ *wire.MsgTx, _ btcjson.FundRawTransactionOpts, _ interface{}) (*btcjson.FundRawTransactionResult, error) {
							// Return a transaction without change
							fundedTx := wire.NewMsgTx(wire.TxVersion)
							fundedTx.AddTxOut(wire.NewTxOut(0, dataScript)) // OP_RETURN output
							fundedTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(hash, 0), nil, nil))

							return &btcjson.FundRawTransactionResult{
								Transaction:    fundedTx,
								Fee:            1000,
								ChangePosition: -1, // No change
							}, nil
						}).AnyTimes(),

					m.EXPECT().
						GetNewAddress("").
						Return(address, nil).AnyTimes(),

					// Second call to FundRawTransaction (after adding change manually)
					m.EXPECT().
						FundRawTransaction(gomock.Any(), gomock.Any(), gomock.Nil()).
						DoAndReturn(func(tx *wire.MsgTx, _ btcjson.FundRawTransactionOpts, _ interface{}) (*btcjson.FundRawTransactionResult, error) {
							// Make sure a change output has been added
							if len(tx.TxOut) < 2 {
								t.Errorf("Expected transaction to have a change output added, but it only has %d outputs", len(tx.TxOut))
							}

							// Return a properly funded transaction with change
							finalTx := wire.NewMsgTx(wire.TxVersion)
							finalTx.AddTxOut(wire.NewTxOut(0, dataScript)) // OP_RETURN output
							finalTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(hash, 0), nil, nil))

							// Add change output
							pkScript, _ := txscript.PayToAddrScript(address)
							finalTx.AddTxOut(wire.NewTxOut(5000, pkScript))

							return &btcjson.FundRawTransactionResult{
								Transaction:    finalTx,
								Fee:            1000,
								ChangePosition: 1,
							}, nil
						}).AnyTimes(),
				)
			},
		},
		{
			name: "failed to fund raw transaction",
			data: []byte("test data"),
			setupMocks: func(m *mocks.MockBTCWallet) {
				m.EXPECT().
					FundRawTransaction(gomock.Any(), gomock.Any(), gomock.Nil()).
					Return(nil, errors.New("insufficient funds"))
			},
			expectedErrContains: "failed to fund raw tx in buildDataTx",
		},
		{
			name: "failed to get new address",
			data: []byte("test data"),
			setupMocks: func(m *mocks.MockBTCWallet) {
				tx := wire.NewMsgTx(wire.TxVersion)
				// Add OP_RETURN output
				builder := txscript.NewScriptBuilder()
				dataScript, _ := builder.AddOp(txscript.OP_RETURN).AddData([]byte("test data")).Script()
				tx.AddTxOut(wire.NewTxOut(0, dataScript))

				hash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
				tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(hash, 0), nil, nil))

				m.EXPECT().
					FundRawTransaction(gomock.Any(), gomock.Any(), nil).
					Return(&btcjson.FundRawTransactionResult{
						Transaction:    tx,
						Fee:            1000,
						ChangePosition: -1,
					}, nil)

				m.EXPECT().
					GetNewAddress("").
					Return(nil, errors.New("wallet locked"))
			},
			expectedErrContains: "err getting raw change address",
		},
		{
			name: "failed to fund transaction after adding change",
			data: []byte("test data"),
			setupMocks: func(m *mocks.MockBTCWallet) {
				tx := wire.NewMsgTx(wire.TxVersion)
				builder := txscript.NewScriptBuilder()
				dataScript, err := builder.AddOp(txscript.OP_RETURN).AddData([]byte("test data")).Script()
				require.NoError(t, err)
				tx.AddTxOut(wire.NewTxOut(0, dataScript))

				// Add an input
				hash, err := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
				require.NoError(t, err)
				tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(hash, 0), nil, nil))

				m.EXPECT().
					FundRawTransaction(gomock.Any(), gomock.Any(), nil).
					Return(&btcjson.FundRawTransactionResult{
						Transaction:    tx,
						Fee:            1000,
						ChangePosition: -1, // No change
					}, nil)

				r := rand.New(rand.NewSource(time.Now().UnixMilli()))
				address, err := datagen.GenRandomBTCAddress(r, &chaincfg.RegressionNetParams)
				require.NoError(t, err)
				m.EXPECT().
					GetNewAddress("").
					Return(address, nil)

				// Second call to FundRawTransaction fails
				m.EXPECT().
					FundRawTransaction(gomock.Any(), gomock.Any(), gomock.Nil()).
					Return(nil, errors.New("insufficient funds"))
			},
			expectedErrContains: "failed to fund raw tx after nochange",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBTCWallet := mocks.NewMockBTCWallet(ctrl)
			logger := zaptest.NewLogger(t).Sugar()

			mockEstimator := &MockEstimator{
				estimateFeePerKWFn: func(_ uint32) (chainfee.SatPerKWeight, error) {
					return chainfee.SatPerKWeight(5000), nil
				},
			}
			relayer := &Relayer{
				BTCWallet: mockBTCWallet,
				Estimator: mockEstimator,
				logger:    logger,
			}

			relayer.finalizeTxFunc = func(tx *wire.MsgTx) (*types.BtcTxInfo, error) {
				hash := tx.TxHash()

				return &types.BtcTxInfo{
					Tx:   tx,
					TxID: &hash,
					Size: int64(tx.SerializeSize()),
					Fee:  btcutil.Amount(1000), // Dummy fee value for testing
				}, nil
			}
			btcConfig := &MockBTCConfig{
				targetBlockNum: 6,
				defaultFee:     chainfee.SatPerKVByte(5000),
				txFeeMin:       chainfee.SatPerKVByte(1000),
				txFeeMax:       chainfee.SatPerKVByte(100000),
			}

			mockBTCWallet.EXPECT().GetBTCConfig().Return(btcConfig.GetBTCConfig()).AnyTimes()
			tc.setupMocks(mockBTCWallet)

			btcTxInfo, err := relayer.buildDataTx(tc.data)

			if tc.expectedErrContains != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedErrContains)
				require.Nil(t, btcTxInfo)
			} else {
				require.NoError(t, err)
				require.NotNil(t, btcTxInfo)
				require.NotNil(t, btcTxInfo.Tx)

				// Verify OP_RETURN data is in the transaction
				foundData := false
				for _, txOut := range btcTxInfo.Tx.TxOut {
					if txOut.Value == 0 && len(txOut.PkScript) > 0 {
						// Check if it's an OP_RETURN output
						if txOut.PkScript[0] == txscript.OP_RETURN {
							// If we could parse out the original data that would be ideal
							// but we'll simplify for this test
							foundData = true

							break
						}
					}
				}
				require.True(t, foundData, "OP_RETURN data not found in transaction")

				// Verify there's at least one input
				require.Greater(t, len(btcTxInfo.Tx.TxIn), 0, "Transaction should have at least one input")

				// Verify there's at least two outputs (OP_RETURN + change)
				require.GreaterOrEqual(t, len(btcTxInfo.Tx.TxOut), 2, "Transaction should have at least two outputs")

				// Verify TxID and Size are set
				require.NotNil(t, btcTxInfo.TxID, "TxID should not be nil")
				require.Greater(t, btcTxInfo.Size, int64(0), "Size should be greater than 0")
			}
		})
	}
}

// nolint:maintidx
func TestRelayer_FinalizeTransaction(t *testing.T) {
	t.Parallel()
	mockEstimator := &MockEstimator{
		relayFeePerKWFn: func() chainfee.SatPerKWeight {
			return chainfee.SatPerKWeight(1000)
		},
		estimateFeePerKWFn: func(_ uint32) (chainfee.SatPerKWeight, error) {
			return chainfee.SatPerKWeight(5000), nil
		},
	}

	createTestTx := func(hasChange bool) *wire.MsgTx {
		tx := wire.NewMsgTx(wire.TxVersion)

		hash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
		tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(hash, 0), nil, nil))

		builder := txscript.NewScriptBuilder()
		dataScript, _ := builder.AddOp(txscript.OP_RETURN).AddData([]byte("test data")).Script()
		tx.AddTxOut(wire.NewTxOut(0, dataScript))

		if hasChange {
			address, _ := btcutil.DecodeAddress("1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa", &chaincfg.MainNetParams)
			pkScript, _ := txscript.PayToAddrScript(address)
			tx.AddTxOut(wire.NewTxOut(10000, pkScript))
		}

		return tx
	}

	tests := []struct {
		name              string
		txSetup           func() *wire.MsgTx
		mockSetup         func(*mocks.MockBTCWallet, *wire.MsgTx)
		expectedErrSubstr string
		validateResult    func(*testing.T, *types.BtcTxInfo, error)
	}{
		{
			name: "successful transaction with change",
			txSetup: func() *wire.MsgTx {
				return createTestTx(true)
			},
			mockSetup: func(m *mocks.MockBTCWallet, tx *wire.MsgTx) {
				m.EXPECT().
					GetNetParams().
					Return(&chaincfg.RegressionNetParams)

				m.EXPECT().
					WalletPassphrase(gomock.Eq("testpassword"), gomock.Eq(int64(300))).
					Return(nil)

				signedTx := wire.NewMsgTx(wire.TxVersion)
				*signedTx = *tx

				m.EXPECT().
					SignRawTransactionWithWallet(gomock.Any()).
					Return(signedTx, true, nil)

				m.EXPECT().
					GetWalletPass().
					Return("testpassword").
					AnyTimes()

				m.EXPECT().
					GetWalletLockTime().
					Return(int64(300)).
					AnyTimes()
			},
			validateResult: func(t *testing.T, result *types.BtcTxInfo, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.NotNil(t, result.Tx)

				assert.Equal(t, 1, len(result.Tx.TxIn))
				assert.Equal(t, 2, len(result.Tx.TxOut))
				assert.Equal(t, int64(10000), result.Tx.TxOut[changePosition].Value)
			},
		},
		{
			name: "transaction without change",
			txSetup: func() *wire.MsgTx {
				return createTestTx(false)
			},
			mockSetup: func(m *mocks.MockBTCWallet, tx *wire.MsgTx) {
				// Mock WalletPassphrase
				m.EXPECT().
					WalletPassphrase(gomock.Eq("testpassword"), gomock.Eq(int64(300))).
					Return(nil)

				signedTx := wire.NewMsgTx(wire.TxVersion)
				*signedTx = *tx

				m.EXPECT().
					SignRawTransactionWithWallet(gomock.Any()).
					Return(signedTx, true, nil)

				m.EXPECT().
					GetWalletPass().
					Return("testpassword").
					AnyTimes()

				m.EXPECT().
					GetWalletLockTime().
					Return(int64(300)).
					AnyTimes()
			},
			validateResult: func(t *testing.T, result *types.BtcTxInfo, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, int64(71), result.Size)
				assert.Equal(t, btcutil.Amount(284), result.Fee)
				assert.NotNil(t, result.Tx)

				assert.Equal(t, 1, len(result.Tx.TxIn))
				assert.Equal(t, 1, len(result.Tx.TxOut))
			},
		},
		{
			name: "no change address found",
			txSetup: func() *wire.MsgTx {
				tx := createTestTx(true)
				tx.TxOut[changePosition].PkScript = []byte{txscript.OP_RETURN}

				return tx
			},
			mockSetup: func(m *mocks.MockBTCWallet, _ *wire.MsgTx) {
				m.EXPECT().
					GetNetParams().
					Return(&chaincfg.MainNetParams)

				m.EXPECT().
					GetWalletPass().
					Return("testpassword").
					AnyTimes()

				m.EXPECT().
					GetWalletLockTime().
					Return(int64(300)).
					AnyTimes()
			},
			expectedErrSubstr: "no change address found",
		},
		{
			name: "wallet unlock fails",
			txSetup: func() *wire.MsgTx {
				return createTestTx(true)
			},
			mockSetup: func(m *mocks.MockBTCWallet, _ *wire.MsgTx) {
				m.EXPECT().
					GetNetParams().
					Return(&chaincfg.MainNetParams)

				m.EXPECT().
					WalletPassphrase(gomock.Eq("testpassword"), gomock.Eq(int64(300))).
					Return(errors.New("wallet unlock failed"))

				m.EXPECT().
					GetWalletPass().
					Return("testpassword").
					AnyTimes()

				m.EXPECT().
					GetWalletLockTime().
					Return(int64(300)).
					AnyTimes()
			},
			expectedErrSubstr: "failed to sign tx",
		},
		{
			name: "signing transaction fails",
			txSetup: func() *wire.MsgTx {
				return createTestTx(true)
			},
			mockSetup: func(m *mocks.MockBTCWallet, _ *wire.MsgTx) {
				m.EXPECT().
					GetNetParams().
					Return(&chaincfg.MainNetParams)

				m.EXPECT().
					WalletPassphrase(gomock.Eq("testpassword"), gomock.Eq(int64(300))).
					Return(nil)

				m.EXPECT().
					SignRawTransactionWithWallet(gomock.Any()).
					Return(nil, false, errors.New("signing failed"))

				m.EXPECT().
					GetWalletPass().
					Return("testpassword").
					AnyTimes()

				m.EXPECT().
					GetWalletLockTime().
					Return(int64(300)).
					AnyTimes()
			},
			expectedErrSubstr: "failed to sign tx",
		},
		{
			name: "incomplete signature",
			txSetup: func() *wire.MsgTx {
				return createTestTx(true)
			},
			mockSetup: func(m *mocks.MockBTCWallet, tx *wire.MsgTx) {
				m.EXPECT().
					GetNetParams().
					Return(&chaincfg.MainNetParams)

				m.EXPECT().
					WalletPassphrase(gomock.Eq("testpassword"), gomock.Eq(int64(300))).
					Return(nil)

				signedTx := wire.NewMsgTx(wire.TxVersion)
				*signedTx = *tx // Copy the original tx

				m.EXPECT().
					SignRawTransactionWithWallet(gomock.Any()).
					Return(signedTx, false, nil) // allSigned is false

				m.EXPECT().
					GetWalletPass().
					Return("testpassword").
					AnyTimes()

				m.EXPECT().
					GetWalletLockTime().
					Return(int64(300)).
					AnyTimes()
			},
			expectedErrSubstr: "partially signed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBTCWallet := mocks.NewMockBTCWallet(ctrl)
			logger := zaptest.NewLogger(t).Sugar()

			relayer := &Relayer{
				BTCWallet: mockBTCWallet,
				Estimator: mockEstimator,
				logger:    logger,
			}

			tx := tc.txSetup()
			tc.mockSetup(mockBTCWallet, tx)

			result, err := relayer.finalizeTransaction(tx)
			if tc.expectedErrSubstr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrSubstr)
				assert.Nil(t, result)
			} else if tc.validateResult != nil {
				tc.validateResult(t, result, err)
			}
		})
	}
}

// nolint:maintidx
func TestRelayer_MaybeResendSecondTxOfCheckpointToBTC(t *testing.T) {
	t.Parallel()
	// Create a mock estimator
	mockEstimator := &MockEstimator{
		relayFeePerKWFn: func() chainfee.SatPerKWeight {
			return chainfee.SatPerKWeight(1000)
		},
		estimateFeePerKWFn: func(_ uint32) (chainfee.SatPerKWeight, error) {
			return chainfee.SatPerKWeight(5000), nil
		},
	}

	// Helper function to create a basic test transaction
	createTestTx := func(changeValue int64) *types.BtcTxInfo {
		tx := wire.NewMsgTx(wire.TxVersion)

		// Add a dummy input
		hash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
		tx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(hash, 0), nil, nil))

		// Add OP_RETURN output (data output)
		builder := txscript.NewScriptBuilder()
		dataScript, _ := builder.AddOp(txscript.OP_RETURN).AddData([]byte("test data")).Script()
		tx.AddTxOut(wire.NewTxOut(0, dataScript))

		// Add change output
		r := rand.New(rand.NewSource(time.Now().UnixMilli()))
		address, err := datagen.GenRandomBTCAddress(r, &chaincfg.RegressionNetParams)
		require.NoError(t, err)
		pkScript, _ := txscript.PayToAddrScript(address)
		tx.AddTxOut(wire.NewTxOut(changeValue, pkScript))

		txID := tx.TxHash()

		return &types.BtcTxInfo{
			TxID: &txID,
			Tx:   tx,
			Size: 200,                  // Dummy size
			Fee:  btcutil.Amount(1000), // Original fee
		}
	}

	tests := []struct {
		name              string
		tx2Setup          func() *types.BtcTxInfo
		bumpedFee         btcutil.Amount
		mockSetup         func(*mocks.MockBTCWallet, *types.BtcTxInfo, btcutil.Amount)
		expectedErrSubstr string
		validateResult    func(*testing.T, *types.BtcTxInfo, error)
	}{
		{
			name: "transaction already confirmed",
			tx2Setup: func() *types.BtcTxInfo {
				return createTestTx(10000)
			},
			bumpedFee: btcutil.Amount(2000),
			mockSetup: func(m *mocks.MockBTCWallet, _ *types.BtcTxInfo, _ btcutil.Amount) {
				// Mock TxDetails to return confirmed status
				m.EXPECT().
					TxDetails(gomock.Any(), gomock.Any()).
					Return(nil, btcclient.TxInChain, nil)
			},
			validateResult: func(t *testing.T, result *types.BtcTxInfo, err error) {
				assert.NoError(t, err)
				assert.Nil(t, result) // No transaction returned when already confirmed
			},
		},
		{
			name: "failed to get transaction details",
			tx2Setup: func() *types.BtcTxInfo {
				return createTestTx(10000)
			},
			bumpedFee: btcutil.Amount(2000),
			mockSetup: func(m *mocks.MockBTCWallet, _ *types.BtcTxInfo, _ btcutil.Amount) {
				// Mock TxDetails to return an error
				m.EXPECT().
					TxDetails(gomock.Any(), gomock.Any()).
					Return(nil, btcclient.TxNotFound, errors.New("failed to get transaction details"))
			},
			expectedErrSubstr: "failed to get transaction details",
		},
		{
			name: "sufficient balance for fee bump",
			tx2Setup: func() *types.BtcTxInfo {
				return createTestTx(10000) // 10,000 satoshis is enough to cover fee
			},
			bumpedFee: btcutil.Amount(2000),
			mockSetup: func(m *mocks.MockBTCWallet, tx *types.BtcTxInfo, bumpedFee btcutil.Amount) {
				// Mock TxDetails to return pending status
				m.EXPECT().
					TxDetails(gomock.Any(), gomock.Any()).
					Return(nil, btcclient.TxInMemPool, nil).AnyTimes()

				// Mock wallet operations for signing
				m.EXPECT().
					WalletPassphrase(gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				m.EXPECT().
					GetWalletPass().
					Return("testpassword").
					AnyTimes()

				m.EXPECT().
					GetWalletLockTime().
					Return(int64(300)).
					AnyTimes()

				// Mock signing
				signedTx := wire.NewMsgTx(wire.TxVersion)
				*signedTx = *tx.Tx // Copy the original tx
				// Adjust the change output with the new fee
				signedTx.TxOut[changePosition].Value = int64(10000 - bumpedFee)

				m.EXPECT().
					SignRawTransactionWithWallet(gomock.Any()).
					Return(signedTx, true, nil).AnyTimes()

				m.EXPECT().
					GetMempoolEntry(gomock.Any()).
					Return(&btcjson.GetMempoolEntryResult{
						DescendantCount: 50,
						DescendantFees:  1000, // Higher than new fee, will cause RBF to fail
						DescendantSize:  500,
					}, nil).AnyTimes()

				m.EXPECT().
					GetNetworkInfo().
					Return(&btcjson.GetNetworkInfoResult{
						IncrementalFee: 2,
					}, nil)

				// Mock sending the transaction
				hash, _ := chainhash.NewHashFromStr("000000000000000000000000000000000000000000000000000000000000abcd")
				m.EXPECT().
					SendRawTransaction(gomock.Any(), gomock.Eq(true)).
					Return(hash, nil).AnyTimes()
			},
			validateResult: func(t *testing.T, result *types.BtcTxInfo, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, btcutil.Amount(2000), result.Fee) // Fee should be updated
				assert.Equal(t, "000000000000000000000000000000000000000000000000000000000000abcd", result.TxID.String())
				// Verify change output value is reduced by new fee
				assert.Equal(t, int64(6000), result.Tx.TxOut[changePosition].Value) // 10000 - 2000
			},
		},
		{
			name: "insufficient balance for fee bump - need to create new inputs",
			tx2Setup: func() *types.BtcTxInfo {
				return createTestTx(600) // Just above dust threshold
			},
			bumpedFee: btcutil.Amount(1000),
			mockSetup: func(m *mocks.MockBTCWallet, tx *types.BtcTxInfo, _ btcutil.Amount) {
				// Mock TxDetails to return pending status
				m.EXPECT().
					TxDetails(gomock.Any(), gomock.Any()).
					Return(nil, btcclient.TxInMemPool, nil)

				// Mock FundRawTransaction
				fundedTx := wire.NewMsgTx(wire.TxVersion)

				// Add a copy of the original data output (OP_RETURN)
				fundedTx.AddTxOut(wire.NewTxOut(0, tx.Tx.TxOut[0].PkScript))

				// Add a new input to cover the higher fee
				hash2, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000002")
				fundedTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(hash2, 0), nil, nil))

				// Also copy the original input
				hash1, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
				fundedTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(hash1, 0), nil, nil))

				// Add change output with new value
				fundedTx.TxOut = append(fundedTx.TxOut, wire.NewTxOut(5000, tx.Tx.TxOut[changePosition].PkScript))

				m.EXPECT().
					GetMempoolEntry(gomock.Any()).
					Return(&btcjson.GetMempoolEntryResult{
						DescendantCount: 50,
						DescendantFees:  400,
						DescendantSize:  500,
					}, nil).AnyTimes()

				m.EXPECT().
					GetNetworkInfo().
					Return(&btcjson.GetNetworkInfoResult{
						IncrementalFee: 2,
					}, nil).AnyTimes()

				m.EXPECT().
					FundRawTransaction(gomock.Any(), gomock.Any(), gomock.Nil()).
					Return(&btcjson.FundRawTransactionResult{
						Transaction:    fundedTx,
						Fee:            1000,
						ChangePosition: 1,
					}, nil).AnyTimes()

				m.EXPECT().
					WalletPassphrase(gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				m.EXPECT().
					GetWalletPass().
					Return("testpassword").
					AnyTimes().AnyTimes()

				m.EXPECT().
					GetWalletLockTime().
					Return(int64(300)).
					AnyTimes().AnyTimes()

				m.EXPECT().
					SignRawTransactionWithWallet(gomock.Any()).
					Return(fundedTx, true, nil).AnyTimes()

				// Mock sending the transaction
				hash, _ := chainhash.NewHashFromStr("000000000000000000000000000000000000000000000000000000000000dcba")
				m.EXPECT().
					SendRawTransaction(gomock.Any(), gomock.Eq(true)).
					Return(hash, nil).AnyTimes()
			},
			validateResult: func(t *testing.T, result *types.BtcTxInfo, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, btcutil.Amount(1000), result.Fee) // Fee should be updated
				assert.Equal(t, "000000000000000000000000000000000000000000000000000000000000dcba", result.TxID.String())
				assert.Equal(t, 2, len(result.Tx.TxIn))                             // Should have an additional input
				assert.Equal(t, int64(5000), result.Tx.TxOut[changePosition].Value) // New change value
			},
		},
		{
			name: "fund transaction fails",
			tx2Setup: func() *types.BtcTxInfo {
				return createTestTx(600) // Just above dust threshold
			},
			bumpedFee: btcutil.Amount(1000),
			mockSetup: func(m *mocks.MockBTCWallet, _ *types.BtcTxInfo, _ btcutil.Amount) {
				// Mock TxDetails to return pending status
				m.EXPECT().
					TxDetails(gomock.Any(), gomock.Any()).
					Return(nil, btcclient.TxInMemPool, nil)

				// Mock FundRawTransaction with error
				m.EXPECT().
					FundRawTransaction(gomock.Any(), gomock.Any(), gomock.Nil()).
					Return(nil, errors.New("insufficient funds"))
			},
			expectedErrSubstr: "failed to fund transaction",
		},
		{
			name: "RBF requirements not met",
			tx2Setup: func() *types.BtcTxInfo {
				return createTestTx(10000)
			},
			bumpedFee: btcutil.Amount(2000),
			mockSetup: func(m *mocks.MockBTCWallet, tx *types.BtcTxInfo, _ btcutil.Amount) {
				m.EXPECT().
					GetNetworkInfo().
					Return(&btcjson.GetNetworkInfoResult{
						IncrementalFee: 2,
					}, nil).AnyTimes()
				m.EXPECT().
					TxDetails(gomock.Any(), gomock.Any()).
					Return(nil, btcclient.TxInMemPool, nil)

				// Mock verifyRBFRequirements to fail
				txID := tx.Tx.TxHash().String()

				// Mock GetMempoolEntry
				m.EXPECT().
					GetMempoolEntry(gomock.Eq(txID)).
					Return(&btcjson.GetMempoolEntryResult{
						DescendantCount: 50,
						DescendantFees:  1900, // Higher than new fee, will cause RBF to fail
						DescendantSize:  500,
					}, nil)
			},
			expectedErrSubstr: "RBF requirements not met",
		},
		{
			name: "signing fails",
			tx2Setup: func() *types.BtcTxInfo {
				return createTestTx(10000)
			},
			bumpedFee: btcutil.Amount(2000),
			mockSetup: func(m *mocks.MockBTCWallet, _ *types.BtcTxInfo, _ btcutil.Amount) {
				// Mock TxDetails to return pending status
				m.EXPECT().
					TxDetails(gomock.Any(), gomock.Any()).
					Return(nil, btcclient.TxInMemPool, nil)

				m.EXPECT().
					GetMempoolEntry(gomock.Any()).
					Return(&btcjson.GetMempoolEntryResult{
						DescendantCount: 50,
						DescendantFees:  400,
						DescendantSize:  500,
					}, nil).AnyTimes()

				m.EXPECT().
					GetNetworkInfo().
					Return(&btcjson.GetNetworkInfoResult{
						IncrementalFee: 2,
					}, nil).AnyTimes()

				// Mock wallet passphrase
				m.EXPECT().
					WalletPassphrase(gomock.Any(), gomock.Any()).
					Return(errors.New("wallet unlock failed")).
					AnyTimes()

				m.EXPECT().
					GetWalletPass().
					Return("testpassword").
					AnyTimes()

				m.EXPECT().
					GetWalletLockTime().
					Return(int64(300)).
					AnyTimes()
			},
			expectedErrSubstr: "wallet unlock failed",
		},
		{
			name: "send transaction fails",
			tx2Setup: func() *types.BtcTxInfo {
				return createTestTx(10000)
			},
			bumpedFee: btcutil.Amount(2000),
			mockSetup: func(m *mocks.MockBTCWallet, tx *types.BtcTxInfo, bumpedFee btcutil.Amount) {
				m.EXPECT().
					TxDetails(gomock.Any(), gomock.Any()).
					Return(nil, btcclient.TxInMemPool, nil)

				m.EXPECT().
					GetMempoolEntry(gomock.Any()).
					Return(&btcjson.GetMempoolEntryResult{
						DescendantCount: 50,
						DescendantFees:  400,
						DescendantSize:  500,
					}, nil).AnyTimes()

				m.EXPECT().
					GetNetworkInfo().
					Return(&btcjson.GetNetworkInfoResult{
						IncrementalFee: 2,
					}, nil).AnyTimes()

				m.EXPECT().
					WalletPassphrase(gomock.Any(), gomock.Any()).
					Return(nil).AnyTimes()

				m.EXPECT().
					GetWalletPass().
					Return("testpassword").
					AnyTimes()

				m.EXPECT().
					GetWalletLockTime().
					Return(int64(300)).
					AnyTimes()

				// Mock signing
				signedTx := wire.NewMsgTx(wire.TxVersion)
				*signedTx = *tx.Tx // Copy the original tx
				// Adjust the change output with the new fee
				signedTx.TxOut[changePosition].Value = int64(10000 - bumpedFee)

				m.EXPECT().
					SignRawTransactionWithWallet(gomock.Any()).
					Return(signedTx, true, nil).AnyTimes()

				// Mock sending the transaction with error
				m.EXPECT().
					SendRawTransaction(gomock.Any(), gomock.Eq(true)).
					Return(nil, errors.New("transaction rejected")).AnyTimes()
			},
			expectedErrSubstr: "transaction rejected",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBTCWallet := mocks.NewMockBTCWallet(ctrl)
			logger := zaptest.NewLogger(t).Sugar()

			relayer := &Relayer{
				BTCWallet: mockBTCWallet,
				Estimator: mockEstimator,
				logger:    logger,
			}

			tx2 := tc.tx2Setup()
			tc.mockSetup(mockBTCWallet, tx2, tc.bumpedFee)
			result, err := relayer.maybeResendSecondTxOfCheckpointToBTC(tx2, tc.bumpedFee)
			if tc.expectedErrSubstr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrSubstr)
			} else if tc.validateResult != nil {
				tc.validateResult(t, result, err)
			}
		})
	}
}

func TestRelayer_BuildChainedDataTx(t *testing.T) {
	t.Parallel()
	// Create a mock estimator
	mockEstimator := &MockEstimator{
		relayFeePerKWFn: func() chainfee.SatPerKWeight {
			return chainfee.SatPerKWeight(1000)
		},
		estimateFeePerKWFn: func(_ uint32) (chainfee.SatPerKWeight, error) {
			return chainfee.SatPerKWeight(5000), nil
		},
	}

	// Helper function to create a previous transaction
	createPrevTx := func() *wire.MsgTx {
		prevTx := wire.NewMsgTx(wire.TxVersion)

		// Add a dummy input
		hash, _ := chainhash.NewHashFromStr("0000000000000000000000000000000000000000000000000000000000000001")
		prevTx.AddTxIn(wire.NewTxIn(wire.NewOutPoint(hash, 0), nil, nil))

		// Add OP_RETURN output
		builder := txscript.NewScriptBuilder()
		dataScript, _ := builder.AddOp(txscript.OP_RETURN).AddData([]byte("previous tx data")).Script()
		prevTx.AddTxOut(wire.NewTxOut(0, dataScript))

		// Add change output (this will be used as input for the chained tx)
		r := rand.New(rand.NewSource(time.Now().UnixMilli()))
		address, err := datagen.GenRandomBTCAddress(r, &chaincfg.RegressionNetParams)
		require.NoError(t, err)
		pkScript, _ := txscript.PayToAddrScript(address)
		prevTx.AddTxOut(wire.NewTxOut(10000, pkScript))

		return prevTx
	}

	tests := []struct {
		name              string
		data              []byte
		prevTx            *wire.MsgTx
		setupMocks        func(*mocks.MockBTCWallet)
		expectedErrSubstr string
		validateResult    func(*testing.T, *types.BtcTxInfo, error)
	}{
		{
			name:   "successful chained transaction",
			data:   []byte("test data for chained tx"),
			prevTx: createPrevTx(),
			setupMocks: func(m *mocks.MockBTCWallet) {
				// Mock FundRawTransaction
				m.EXPECT().
					FundRawTransaction(gomock.Any(), gomock.Any(), gomock.Nil()).
					DoAndReturn(func(tx *wire.MsgTx, _ btcjson.FundRawTransactionOpts, _ interface{}) (*btcjson.FundRawTransactionResult, error) {
						// Verify the input points to the change output of the previous tx
						assert.Equal(t, 1, len(tx.TxIn), "Transaction should have exactly one input")
						assert.Equal(t, uint32(1), tx.TxIn[0].PreviousOutPoint.Index, "Input should reference change output (index 1)")

						// Create a properly funded transaction
						fundedTx := wire.NewMsgTx(wire.TxVersion)

						// Copy the original input
						for _, txIn := range tx.TxIn {
							fundedTx.AddTxIn(wire.NewTxIn(&txIn.PreviousOutPoint, nil, nil))
							// Preserve the sequence number for RBF
							fundedTx.TxIn[len(fundedTx.TxIn)-1].Sequence = txIn.Sequence
						}

						// Copy the OP_RETURN output
						fundedTx.AddTxOut(wire.NewTxOut(0, tx.TxOut[0].PkScript))

						// Add a change output
						r := rand.New(rand.NewSource(time.Now().UnixMilli()))
						address, err := datagen.GenRandomBTCAddress(r, &chaincfg.RegressionNetParams)
						require.NoError(t, err)
						pkScript, _ := txscript.PayToAddrScript(address)
						fundedTx.AddTxOut(wire.NewTxOut(8000, pkScript)) // 10000 - 2000 fee

						return &btcjson.FundRawTransactionResult{
							Transaction:    fundedTx,
							Fee:            2000,
							ChangePosition: 1,
						}, nil
					})

				btcConfig := &MockBTCConfig{
					targetBlockNum: 6,
					defaultFee:     chainfee.SatPerKVByte(5000),
					txFeeMin:       chainfee.SatPerKVByte(1000),
					txFeeMax:       chainfee.SatPerKVByte(100000),
				}
				m.EXPECT().GetBTCConfig().Return(btcConfig.GetBTCConfig()).AnyTimes()
			},
			validateResult: func(t *testing.T, result *types.BtcTxInfo, err error) {
				assert.NoError(t, err)
				assert.NotNil(t, result)

				// Verify transaction structure
				assert.Equal(t, 1, len(result.Tx.TxIn), "Transaction should have one input")
				assert.Equal(t, 2, len(result.Tx.TxOut), "Transaction should have two outputs (data and change)")

				// Verify the input is correctly pointing to previous tx's change output
				assert.Equal(t, uint32(1), result.Tx.TxIn[0].PreviousOutPoint.Index, "Input should point to previous tx's change output")

				// Verify the input sequence is set for RBF
				assert.Equal(t, uint32(0xFFFFFFFD), result.Tx.TxIn[0].Sequence, "Input sequence should be set to enable RBF")

				// Verify the data output
				assert.Equal(t, int64(0), result.Tx.TxOut[0].Value, "OP_RETURN output should have zero value")

				// Verify the change output
				assert.Equal(t, int64(8000), result.Tx.TxOut[1].Value, "Change output should have correct value")

				// Verify the size and fee values from our mock implementation
				assert.Equal(t, int64(250), result.Size, "Transaction size should match our mock")
				assert.Equal(t, btcutil.Amount(2000), result.Fee, "Transaction fee should match our mock")
			},
		},
		{
			name:   "fund raw transaction fails",
			data:   []byte("test data for chained tx"),
			prevTx: createPrevTx(),
			setupMocks: func(m *mocks.MockBTCWallet) {
				btcConfig := &MockBTCConfig{
					targetBlockNum: 6,
					defaultFee:     chainfee.SatPerKVByte(5000),
					txFeeMin:       chainfee.SatPerKVByte(1000),
					txFeeMax:       chainfee.SatPerKVByte(100000),
				}
				m.EXPECT().GetBTCConfig().Return(btcConfig.GetBTCConfig()).AnyTimes()
				m.EXPECT().
					FundRawTransaction(gomock.Any(), gomock.Any(), gomock.Nil()).
					Return(nil, errors.New("insufficient funds"))
			},
			expectedErrSubstr: "failed to fund raw tx in buildChainedDataTx",
		},
		{
			name:   "finalize transaction fails",
			data:   []byte("test data for chained tx"),
			prevTx: createPrevTx(),
			setupMocks: func(m *mocks.MockBTCWallet) {
				m.EXPECT().
					FundRawTransaction(gomock.Any(), gomock.Any(), gomock.Nil()).
					DoAndReturn(func(tx *wire.MsgTx, _ btcjson.FundRawTransactionOpts, _ interface{}) (*btcjson.FundRawTransactionResult, error) {
						// Create a properly funded transaction
						fundedTx := wire.NewMsgTx(wire.TxVersion)

						// Copy the original input
						for _, txIn := range tx.TxIn {
							fundedTx.AddTxIn(wire.NewTxIn(&txIn.PreviousOutPoint, nil, nil))
							fundedTx.TxIn[len(fundedTx.TxIn)-1].Sequence = txIn.Sequence
						}

						// Copy the OP_RETURN output
						fundedTx.AddTxOut(wire.NewTxOut(0, tx.TxOut[0].PkScript))

						// Add a change output
						r := rand.New(rand.NewSource(time.Now().UnixMilli()))
						address, err := datagen.GenRandomBTCAddress(r, &chaincfg.RegressionNetParams)
						require.NoError(t, err)
						pkScript, _ := txscript.PayToAddrScript(address)
						fundedTx.AddTxOut(wire.NewTxOut(8000, pkScript))

						return &btcjson.FundRawTransactionResult{
							Transaction:    fundedTx,
							Fee:            2000,
							ChangePosition: 1,
						}, nil
					})
				btcConfig := &MockBTCConfig{
					targetBlockNum: 6,
					defaultFee:     chainfee.SatPerKVByte(5000),
					txFeeMin:       chainfee.SatPerKVByte(1000),
					txFeeMax:       chainfee.SatPerKVByte(100000),
				}
				m.EXPECT().GetBTCConfig().Return(btcConfig.GetBTCConfig()).AnyTimes()
			},
			expectedErrSubstr: "failed to sign tx",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBTCWallet := mocks.NewMockBTCWallet(ctrl)
			logger := zaptest.NewLogger(t).Sugar()

			// Create a test relayer with the mocked components
			relayer := &Relayer{
				BTCWallet: mockBTCWallet,
				Estimator: mockEstimator,
				logger:    logger,
				finalizeTxFunc: func(tx *wire.MsgTx) (*types.BtcTxInfo, error) {
					if tc.expectedErrSubstr == "failed to sign tx" {
						return nil, errors.New("failed to sign tx: wallet unlock failed")
					}

					txHash := tx.TxHash()

					return &types.BtcTxInfo{
						TxID: &txHash,
						Tx:   tx,
						Size: 250,
						Fee:  btcutil.Amount(2000),
					}, nil
				},
			}

			tc.setupMocks(mockBTCWallet)
			result, err := relayer.buildChainedDataTx(tc.data, tc.prevTx)
			if tc.expectedErrSubstr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrSubstr)
				assert.Nil(t, result)
			} else if tc.validateResult != nil {
				tc.validateResult(t, result, err)
			}
		})
	}
}
