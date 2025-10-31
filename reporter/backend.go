package reporter

import (
	"context"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// Backend is the interface for submitting BTC headers to different backends (Babylon, Ethereum, etc.)
// This abstraction allows the reporter to work with multiple destination chains.
//
// Implementations:
//   - CosmosBackend: Submits headers to Babylon chain via Cosmos SDK
//   - EthereumBackend: Submits headers to Ethereum smart contract (BtcPrism.sol)
type Backend interface {
	// ContainsBlock checks if the backend has stored the given block hash.
	// This is used for deduplication to avoid submitting duplicate headers.
	ContainsBlock(ctx context.Context, hash *chainhash.Hash) (bool, error)

	// SubmitHeaders submits a batch of BTC headers starting at startHeight.
	// headers is a slice of 80-byte block headers in raw format.
	// Returns an error if submission fails.
	SubmitHeaders(ctx context.Context, startHeight uint64, headers [][]byte) error

	// GetTip returns the backend's current tip block height and hash.
	// This is used by the reporter to determine which blocks need to be synced.
	GetTip(ctx context.Context) (height uint32, hash *chainhash.Hash, err error)

	// Stop gracefully shuts down the backend, closing any open connections.
	Stop() error
}
