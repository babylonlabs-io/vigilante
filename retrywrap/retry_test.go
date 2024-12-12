package retrywrap

import (
	"errors"
	"github.com/avast/retry-go/v4"
	btcctypes "github.com/babylonlabs-io/babylon/x/btccheckpoint/types"
	btclctypes "github.com/babylonlabs-io/babylon/x/btclightclient/types"
	"testing"
)

func TestWrapDo(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		retryableErr error
		expectedErr  error
		attempts     int
	}{
		{
			name:         "Retry on regular error",
			retryableErr: errors.New("regular error"),
			expectedErr:  errors.New("regular error"),
			attempts:     3,
		},

		{
			name:         "No retry on unrecoverable error",
			retryableErr: btclctypes.ErrInvalidHeader,
			expectedErr:  btclctypes.ErrInvalidHeader,
			attempts:     1,
		},
		{
			name:         "Ignore expected error",
			retryableErr: btcctypes.ErrDuplicatedSubmission,
			expectedErr:  nil,
			attempts:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var attempts int
			retryableFunc := func() error {
				attempts++

				return tt.retryableErr
			}

			err := Do(
				retryableFunc,
				retry.Attempts(uint(tt.attempts)),
				retry.LastErrorOnly(true),
			)

			if err != nil && err.Error() != tt.expectedErr.Error() {
				t.Errorf("WrapDo() error = %v, want %v", err, tt.expectedErr)
			}

			if attempts != tt.attempts {
				t.Errorf("WrapDo() attempts = %v, want %v", attempts, tt.attempts)
			}
		})
	}
}
