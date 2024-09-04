package retrywrap

import (
	"errors"
	"github.com/avast/retry-go/v4"
	btcctypes "github.com/babylonlabs-io/babylon/x/btccheckpoint/types"
	btclctypes "github.com/babylonlabs-io/babylon/x/btclightclient/types"
	checkpointingtypes "github.com/babylonlabs-io/babylon/x/checkpointing/types"
)

// unrecoverableErrors is a list of errors which are unsafe and should not be retried.
var unrecoverableErrors = []error{
	btclctypes.ErrHeaderParentDoesNotExist,
	btclctypes.ErrChainWithNotEnoughWork,
	btclctypes.ErrInvalidHeader,
	btcctypes.ErrProvidedHeaderDoesNotHaveAncestor,
	btcctypes.ErrInvalidHeader,
	btcctypes.ErrNoCheckpointsForPreviousEpoch,
	btcctypes.ErrInvalidCheckpointProof,
	checkpointingtypes.ErrBlsPrivKeyDoesNotExist,
}

// expectedErrors is a list of errors which can safely be ignored and should not be retried.
var expectedErrors = []error{
	btcctypes.ErrDuplicatedSubmission,
	btcctypes.ErrInvalidHeader,
}

func containsErr(errs []error, err error) bool {
	for _, e := range errs {
		if errors.Is(err, e) {
			return true
		}
	}

	return false
}

// Do - executes a retryable function with customizable retry behavior using retry-go.
// It retries the function unless the error is considered unrecoverable or expected.
// Unrecoverable errors will stop further retries and return the error.
// Expected errors are ignored and considered as successful executions.
func Do(retryableFunc retry.RetryableFunc, opts ...retry.Option) error {
	opt := retry.RetryIf(func(err error) bool {
		// Don't retry on unrecoverable errors
		if containsErr(unrecoverableErrors, err) || containsErr(expectedErrors, err) {
			return false
		}

		return true // Retry on all other errors
	})

	opts = append(opts, opt)

	err := retry.Do(retryableFunc, opts...)
	if containsErr(expectedErrors, err) {
		// Return nil for expected errors to ignore them
		return nil
	}

	return err
}
