package stakingeventwatcher

import (
	"context"
	"github.com/babylonlabs-io/vigilante/btcstaking-tracker/indexer"
)

type SpendChecker interface {
	GetOutspend(ctx context.Context, txID string, vout uint32) (*indexer.OutspendResponse, error)
}
