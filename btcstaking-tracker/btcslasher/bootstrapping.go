package btcslasher

import (
	"fmt"

	"github.com/babylonlabs-io/babylon/v4/types"

	ftypes "github.com/babylonlabs-io/babylon/v4/x/finality/types"
	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/hashicorp/go-multierror"
)

// Bootstrap bootstraps the BTC slasher. Specifically, it checks all evidences
// since the given startHeight to see if any slashing tx is not submitted to Bitcoin.
// If the slashing tx under a finality provider with an equivocation evidence is still
// spendable on Bitcoin, then it will submit it to Bitcoin thus slashing this BTC delegation.
func (bs *BTCSlasher) Bootstrap(startHeight uint64) error {
	bs.logger.Infof("start bootstrapping BTC slasher from height %d", startHeight)

	if startHeight > 0 {
		bs.mu.Lock()
		bs.height = startHeight
		bs.mu.Unlock()
	}

	// load module parameters
	if err := bs.LoadParams(); err != nil {
		return err
	}

	// Replay any pending evidences from a previous run before processing new
	// evidences. This is what makes bootstrap progress durable across restarts:
	// a Babylon ListEvidences(startHeight) call cannot rediscover an FP whose
	// first slashable evidence is below startHeight, so we keep our own queue.
	if err := bs.replayPendingEvidences(); err != nil {
		return fmt.Errorf("failed to replay pending evidences: %w", err)
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

		if err := bs.store.PutHeight(lastSlashedHeight); err != nil {
			return fmt.Errorf("failed to store last processed height %d: %w", lastSlashedHeight, err)
		}
	}

	return nil
}

// replayPendingEvidences reads all evidences from the pending bucket and
// re-dispatches slashing for each. Bitcoin tx submission is idempotent via
// isTxSubmittedToBitcoin, so replay is safe even if a tx was already
// broadcast in a previous run.
func (bs *BTCSlasher) replayPendingEvidences() error {
	pending, err := bs.store.ListPendingEvidences()
	if err != nil {
		return fmt.Errorf("failed to list pending evidences: %w", err)
	}
	if len(pending) == 0 {
		return nil
	}
	bs.logger.Infof("replaying %d pending evidences from previous run", len(pending))

	var accumulatedErrs error
	for _, ev := range pending {
		btcPK, err := types.NewBIP340PubKeyFromHex(ev.FpBtcPkHex)
		if err != nil {
			bs.logger.Errorf("pending evidence has malformed fp_btc_pk_hex %q: %v", ev.FpBtcPkHex, err)
			accumulatedErrs = multierror.Append(accumulatedErrs, err)

			continue
		}
		e := ftypes.Evidence{
			FpBtcPk:              btcPK,
			BlockHeight:          ev.BlockHeight,
			PubRand:              ev.PubRand,
			CanonicalAppHash:     ev.CanonicalAppHash,
			ForkAppHash:          ev.ForkAppHash,
			CanonicalFinalitySig: ev.CanonicalFinalitySig,
			ForkFinalitySig:      ev.ForkFinalitySig,
		}
		fpBTCSK, err := e.ExtractBTCSK()
		if err != nil {
			bs.logger.Errorf("pending evidence at height %d for fp %s is no longer slashable: %v", ev.BlockHeight, ev.FpBtcPkHex, err)
			accumulatedErrs = multierror.Append(accumulatedErrs, err)

			continue
		}
		if err := bs.SlashFinalityProvider(fpBTCSK); err != nil {
			bs.logger.Errorf("failed to redispatch slashing for fp %s at height %d: %v", ev.FpBtcPkHex, ev.BlockHeight, err)
			accumulatedErrs = multierror.Append(accumulatedErrs, err)

			continue
		}
	}

	return accumulatedErrs
}

func (bs *BTCSlasher) LastEvidencesHeight() (uint64, bool, error) {
	return bs.store.LastProcessedHeight()
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

			// Persist the evidence to the pending bucket BEFORE dispatching slashing.
			// On restart, pending entries will be replayed regardless of cursor.
			if err := bs.store.PutPendingEvidence(evidence); err != nil {
				bs.logger.Errorf("failed to persist pending evidence for fp %s at height %d: %v", fpBTCPKHex, evidence.BlockHeight, err)
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
