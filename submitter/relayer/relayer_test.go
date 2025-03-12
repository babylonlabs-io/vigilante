package relayer

import (
	"errors"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/babylonlabs-io/vigilante/submitter/store"
	"github.com/babylonlabs-io/vigilante/testutil/mocks"
	"github.com/babylonlabs-io/vigilante/types"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/golang/mock/gomock"
	"github.com/lightningnetwork/lnd/lnwallet/chainfee"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
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

// MockEstimator implements the chainfee.Estimator interface for testing
type MockEstimator struct {
	estimateFeePerKWFn func(uint32) (chainfee.SatPerKWeight, error)
}

func (m *MockEstimator) Start() error {
	panic("implement me")
}

func (m *MockEstimator) Stop() error {
	panic("implement me")
}

func (m *MockEstimator) RelayFeePerKW() chainfee.SatPerKWeight {
	panic("implement me")
}

func (m *MockEstimator) EstimateFeePerKW(confirmationTarget uint32) (chainfee.SatPerKWeight, error) {
	return m.estimateFeePerKWFn(confirmationTarget)
}

// mockBTCConfig is a helper to create a mock BTC Config
type mockBTCConfig struct {
	targetBlockNum int64
	defaultFee     chainfee.SatPerKVByte
	txFeeMin       chainfee.SatPerKVByte
	txFeeMax       chainfee.SatPerKVByte
}

func (m *mockBTCConfig) GetBTCConfig() *config.BTCConfig {
	return &config.BTCConfig{
		TargetBlockNum: m.targetBlockNum,
		DefaultFee:     m.defaultFee,
		TxFeeMin:       m.txFeeMin,
		TxFeeMax:       m.txFeeMax,
	}
}

// TestCalculateBumpedFee tests the calculateBumpedFee function
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

		btcConfig := &mockBTCConfig{
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

	t.Run("Basic fee calculation with 'other' error type", func(t *testing.T) {
		ckptInfo := createCheckpointInfo()
		submitterConfig := &config.SubmitterConfig{
			ResubmitFeeMultiplier: 1.2,
		}
		// 5000 sat/kvB = 5 sat/B
		rl := createRelayer(submitterConfig, 5000, nil)

		// Calculate bumped fee with some error
		someError := errors.New("other ")
		bumpedFee, err := rl.calculateBumpedFee(ckptInfo, someError)
		require.NoError(t, err)

		expectedRequiredFee := btcutil.Amount(1250) // 5 * 250
		require.Equal(t, expectedRequiredFee, bumpedFee)
	})

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

		// Calculate bumped fee with insufficient fee error
		insufficientFeeError := errors.New("insufficient fee")
		bumpedFee, err := rl.calculateBumpedFee(ckptInfo, insufficientFeeError)
		require.NoError(t, err)

		expectedAdjustedFee := btcutil.Amount(2500) // 2000 * (1 + 0.25)
		require.Equal(t, expectedAdjustedFee, bumpedFee)
	})

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

		insufficientFeerateError := errors.New("feerate insufficient")
		bumpedFee, err := rl.calculateBumpedFee(ckptInfo, insufficientFeerateError)
		require.NoError(t, err)

		expectedRequiredFee := btcutil.Amount(2000) // 8 * 250
		require.Equal(t, expectedRequiredFee, bumpedFee)
	})

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

		// Calculate bumped fee with fee increment error
		feeIncrementError := errors.New("fee increment too small")
		bumpedFee, err := rl.calculateBumpedFee(ckptInfo, feeIncrementError)
		require.NoError(t, err)

		expectedRequiredFee := btcutil.Amount(1500) // 6 * 250
		assert.Equal(t, expectedRequiredFee, bumpedFee)
	})
}
