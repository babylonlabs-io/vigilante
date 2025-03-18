package relayer

import (
	"errors"
	"github.com/babylonlabs-io/babylon/testutil/datagen"
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

// MockEstimator implements the chainfee.Estimator interface for testing
type MockEstimator struct {
	estimateFeePerKWFn func(uint32) (chainfee.SatPerKWeight, error)
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
