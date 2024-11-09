package relayer

import (
	"errors"
	"github.com/babylonlabs-io/vigilante/submitter/store"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
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
				return nil, errors.New("transaction not found") // Simulate transaction not found
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
				return nil, errors.New("transaction not found")
			},
			sendTransaction: func(_ *wire.MsgTx) (*chainhash.Hash, error) {
				return nil, errors.New("send error")
			},
			expectedResult: false,
			expectedError:  errors.New("send error"),
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
