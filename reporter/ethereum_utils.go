package reporter

import (
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

// Contract error types matching IBtcPrism.sol
var (
	ErrWrongHeaderLength           = fmt.Errorf("wrong header length")
	ErrNoBlocksSubmitted           = fmt.Errorf("no blocks submitted")
	ErrBadParent                   = fmt.Errorf("bad parent hash")
	ErrNoParent                    = fmt.Errorf("no parent block found")
	ErrHashAboveTarget             = fmt.Errorf("block hash above target (invalid PoW)")
	ErrDifficultyRetargetLT25      = fmt.Errorf("difficulty retarget less than 25%%")
	ErrWrongDifficultyBits         = fmt.Errorf("wrong difficulty bits")
	ErrOldDifficultyPeriod         = fmt.Errorf("old difficulty period")
	ErrInsufficientTotalDifficulty = fmt.Errorf("insufficient total difficulty")
	ErrInsufficientChainLength     = fmt.Errorf("insufficient chain length")
	ErrTooDeepReorg                = fmt.Errorf("reorg too deep (max 1000 blocks)")
)

// parseContractError attempts to parse contract revert errors into friendly messages
func parseContractError(err error) error {
	if err == nil {
		return nil
	}

	errMsg := err.Error()

	// Check for specific contract errors
	// These match the custom errors defined in IBtcPrism.sol
	switch {
	case strings.Contains(errMsg, "WrongHeaderLength"):
		return ErrWrongHeaderLength
	case strings.Contains(errMsg, "NoBlocksSubmitted"):
		return ErrNoBlocksSubmitted
	case strings.Contains(errMsg, "BadParent"):
		return ErrBadParent
	case strings.Contains(errMsg, "NoParent"):
		return ErrNoParent
	case strings.Contains(errMsg, "HashAboveTarget"):
		return ErrHashAboveTarget
	case strings.Contains(errMsg, "DifficultyRetargetLT25"):
		return ErrDifficultyRetargetLT25
	case strings.Contains(errMsg, "WrongDifficultyBits"):
		return ErrWrongDifficultyBits
	case strings.Contains(errMsg, "OldDifficultyPeriod"):
		return ErrOldDifficultyPeriod
	case strings.Contains(errMsg, "InsufficientTotalDifficulty"):
		return ErrInsufficientTotalDifficulty
	case strings.Contains(errMsg, "InsufficientChainLength"):
		return ErrInsufficientChainLength
	case strings.Contains(errMsg, "TooDeepReorg"):
		return ErrTooDeepReorg
	}

	// Check for common Ethereum errors
	switch {
	case strings.Contains(errMsg, "insufficient funds"):
		return fmt.Errorf("insufficient funds for gas: %w", err)
	case strings.Contains(errMsg, "gas required exceeds allowance"):
		return fmt.Errorf("gas limit too low: %w", err)
	case strings.Contains(errMsg, "nonce too low"):
		return fmt.Errorf("transaction nonce too low (already used): %w", err)
	case strings.Contains(errMsg, "replacement transaction underpriced"):
		return fmt.Errorf("replacement transaction underpriced: %w", err)
	case strings.Contains(errMsg, "execution reverted"):
		// Try to extract revert reason if available
		if reason := extractRevertReason(errMsg); reason != "" {
			return fmt.Errorf("contract execution reverted: %s", reason)
		}
		return fmt.Errorf("contract execution reverted: %w", err)
	}

	// Return original error if no match
	return fmt.Errorf("ethereum transaction failed: %w", err)
}

// extractRevertReason attempts to extract the revert reason from error message
func extractRevertReason(errMsg string) string {
	// Look for common revert reason patterns
	patterns := []string{
		"execution reverted: ",
		"revert: ",
	}

	for _, pattern := range patterns {
		if idx := strings.Index(errMsg, pattern); idx != -1 {
			reason := errMsg[idx+len(pattern):]
			// Clean up the reason
			reason = strings.TrimSpace(reason)
			reason = strings.Trim(reason, `"`)
			return reason
		}
	}

	return ""
}

// unpackError attempts to unpack ABI-encoded error data
// This is useful for custom errors with parameters
func unpackError(data []byte, errorABI *abi.Error) (map[string]interface{}, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("error data too short")
	}

	// Skip the 4-byte function selector
	values, err := errorABI.Inputs.Unpack(data[4:])
	if err != nil {
		return nil, err
	}

	result := make(map[string]interface{})
	for i, input := range errorABI.Inputs {
		if i < len(values) {
			result[input.Name] = values[i]
		}
	}

	return result, nil
}
