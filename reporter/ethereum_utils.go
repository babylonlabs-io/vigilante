package reporter

import (
	"fmt"
	"strings"
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
	ErrBlockNotYetSubmitted        = fmt.Errorf("block not yet submitted")
	ErrGapInHeaderChain            = fmt.Errorf("gap in header chain: blocks don't connect to contract tip")
)

// isBlockNotYetSubmittedError checks if an error is the contract's "Block not yet submitted" revert.
// This is used by ContainsBlock to distinguish between "block doesn't exist" and actual errors.
func isBlockNotYetSubmittedError(err error) bool {
	if err == nil {
		return false
	}

	return strings.Contains(err.Error(), "Block not yet submitted")
}

// parseContractError attempts to parse contract revert errors into friendly messages
func parseContractError(err error) error {
	if err == nil {
		return nil
	}

	errMsg := err.Error()

	// Check for specific contract errors first (custom errors from IBtcPrism.sol)
	// These are more specific and should be checked before generic messages
	switch {
	case strings.Contains(errMsg, "WrongHeaderLength"):
		return fmt.Errorf("%w (headers must be 80 bytes each): %s", ErrWrongHeaderLength, errMsg)
	case strings.Contains(errMsg, "NoBlocksSubmitted"):
		return fmt.Errorf("%w: %s", ErrNoBlocksSubmitted, errMsg)
	case strings.Contains(errMsg, "BadParent"):
		return fmt.Errorf("%w (parent block hash doesn't match): %s", ErrBadParent, errMsg)
	case strings.Contains(errMsg, "NoParent"):
		return fmt.Errorf("%w (starting height not in contract): %s", ErrNoParent, errMsg)
	case strings.Contains(errMsg, "HashAboveTarget"):
		return fmt.Errorf("%w (proof of work insufficient): %s", ErrHashAboveTarget, errMsg)
	case strings.Contains(errMsg, "DifficultyRetargetLT25"):
		return fmt.Errorf("%w: %s", ErrDifficultyRetargetLT25, errMsg)
	case strings.Contains(errMsg, "WrongDifficultyBits"):
		return fmt.Errorf("%w: %s", ErrWrongDifficultyBits, errMsg)
	case strings.Contains(errMsg, "OldDifficultyPeriod"):
		return fmt.Errorf("%w: %s", ErrOldDifficultyPeriod, errMsg)
	case strings.Contains(errMsg, "InsufficientTotalDifficulty"):
		return fmt.Errorf("%w: %s", ErrInsufficientTotalDifficulty, errMsg)
	case strings.Contains(errMsg, "InsufficientChainLength"):
		return fmt.Errorf("%w: %s", ErrInsufficientChainLength, errMsg)
	case strings.Contains(errMsg, "TooDeepReorg"):
		return fmt.Errorf("%w (max 1000 blocks): %s", ErrTooDeepReorg, errMsg)
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
			return fmt.Errorf("contract execution reverted: %s (full error: %s)", reason, errMsg)
		}

		return fmt.Errorf("contract execution reverted (no specific reason found): %s", errMsg)
	}

	// Return original error with full context if no match
	return fmt.Errorf("ethereum transaction failed: %s (original error: %w)", errMsg, err)
}

// extractRevertReason attempts to extract the revert reason from error message
func extractRevertReason(errMsg string) string {
	// Look for common revert reason patterns in order of specificity
	patterns := []struct {
		prefix string
		suffix string
	}{
		{"execution reverted: ", ""},
		{"revert ", ""},
		{"reverted with reason string '", "'"},
		{"custom error '", "'"},
	}

	for _, pattern := range patterns {
		if idx := strings.Index(errMsg, pattern.prefix); idx != -1 {
			start := idx + len(pattern.prefix)
			reason := errMsg[start:]

			// Find suffix if specified
			if pattern.suffix != "" {
				if endIdx := strings.Index(reason, pattern.suffix); endIdx != -1 {
					reason = reason[:endIdx]
				}
			}

			// Clean up the reason
			reason = strings.TrimSpace(reason)
			reason = strings.Trim(reason, `"`)

			// Stop at newline or comma if present
			if nlIdx := strings.IndexAny(reason, "\n,"); nlIdx != -1 {
				reason = reason[:nlIdx]
			}

			if reason != "" {
				return strings.TrimSpace(reason)
			}
		}
	}

	return ""
}
