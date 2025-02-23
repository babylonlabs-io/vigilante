package btcslasher

import (
	"fmt"
	"github.com/babylonlabs-io/babylon/types"

	ftypes "github.com/babylonlabs-io/babylon/x/finality/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/hashicorp/go-multierror"
)

// Bootstrap bootstraps the BTC slasher. Specifically, it checks all evidences
// since the given startHeight to see if any slashing tx is not submitted to Bitcoin.
// If the slashing tx under a finality provider with an equivocation evidence is still
// spendable on Bitcoin, then it will submit it to Bitcoin thus slashing this BTC delegation.
func (bs *BTCSlasher) Bootstrap(startHeight uint64) error {
	bs.logger.Info("start bootstrapping BTC slasher")

	// load module parameters
	if err := bs.LoadParams(); err != nil {
		return err
	}

	lastSlashedHeight, err := bs.processEvidencesFromHeight(startHeight)
	if err != nil {
		return fmt.Errorf("failed to bootstrap BTC slasher: %w", err)
	}

	// Only update height if we actually processed evidence
	if lastSlashedHeight > 0 {
		bs.mu.Lock()
		bs.height = lastSlashedHeight + 1
		bs.mu.Unlock()
	}

	return nil
}
func (bs *BTCSlasher) processEvidencesFromHeight(startHeight uint64) (uint64, error) {
	var lastSlashedHeight uint64
	var accumulatedErrs error

	err := bs.handleAllEvidences(startHeight, func(evidences []*ftypes.EvidenceResponse) error {
		for _, evidence := range evidences {
			fpBTCPKHex := evidence.FpBtcPkHex
			bs.logger.Infof("found evidence for finality provider %s at height %d", fpBTCPKHex, evidence.BlockHeight)

			btcPK, err := types.NewBIP340PubKeyFromHex(fpBTCPKHex)
			if err != nil {
				accumulatedErrs = multierror.Append(accumulatedErrs, fmt.Errorf("err parsing fp btc %w", err))

				continue
			}

			e := ftypes.Evidence{
				FpBtcPk:              btcPK,
				BlockHeight:          evidence.BlockHeight,
				PubRand:              evidence.PubRand,
				CanonicalAppHash:     evidence.CanonicalAppHash,
				ForkAppHash:          evidence.ForkAppHash,
				CanonicalFinalitySig: evidence.CanonicalFinalitySig,
				ForkFinalitySig:      evidence.ForkFinalitySig,
			}

			// Extract the SK of the slashed finality provider
			fpBTCSK, err := e.ExtractBTCSK()
			if err != nil {
				bs.logger.Errorf("failed to extract BTC SK of the slashed finality provider %s: %v", fpBTCPKHex, err)
				accumulatedErrs = multierror.Append(accumulatedErrs, err)

				continue
			}

			// Attempt to slash the finality provider
			if err := bs.SlashFinalityProvider(fpBTCSK); err != nil {
				bs.logger.Errorf("failed to slash finality provider %s: %v", fpBTCPKHex, err)
				accumulatedErrs = multierror.Append(accumulatedErrs, err)

				continue
			}

			if lastSlashedHeight < evidence.BlockHeight {
				lastSlashedHeight = evidence.BlockHeight
			}
		}

		return accumulatedErrs
	})

	if err != nil {
		return 0, fmt.Errorf("failed to process evidences from height %d: %w", startHeight, err)
	}

	return lastSlashedHeight, accumulatedErrs
}

func (bs *BTCSlasher) handleAllEvidences(startHeight uint64, handleFunc func(evidences []*ftypes.EvidenceResponse) error) error {
	pagination := query.PageRequest{Limit: defaultPaginationLimit}
	for {
		resp, err := bs.BBNQuerier.ListEvidences(startHeight, &pagination)
		if err != nil {
			return fmt.Errorf("failed to get evidences: %w", err)
		}
		if err := handleFunc(resp.Evidences); err != nil {
			// we should continue getting and handling evidences in subsequent pages
			// rather than return here
			bs.logger.Errorf("failed to handle evidences: %v", err)
		}
		if resp.Pagination == nil || resp.Pagination.NextKey == nil {
			break
		}
		pagination.Key = resp.Pagination.NextKey
	}

	return nil
}
