package reporter

import (
	"bytes"
	"context"
	"fmt"

	coserrors "cosmossdk.io/errors"
	btclctypes "github.com/babylonlabs-io/babylon/v4/x/btclightclient/types"
	"github.com/babylonlabs-io/vigilante/types"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// BabylonBackend wraps the existing BabylonClient to implement the Backend interface.
// This allows the reporter to submit BTC headers to a Babylon chain via Cosmos SDK messages.
type BabylonBackend struct {
	client BabylonClient
}

// NewBabylonBackend creates a new BabylonBackend wrapping the given BabylonClient.
func NewBabylonBackend(client BabylonClient) Backend {
	return &BabylonBackend{
		client: client,
	}
}

// ContainsBlock checks if Babylon has stored the given block hash.
func (b *BabylonBackend) ContainsBlock(_ context.Context, hash *chainhash.Hash) (bool, error) {
	res, err := b.client.ContainsBTCBlock(hash)
	if err != nil {
		return false, fmt.Errorf("failed to check if Babylon contains block: %w", err)
	}

	return res.Contains, nil
}

// GetTip returns Babylon's BTC light client tip height and hash.
func (b *BabylonBackend) GetTip(_ context.Context) (uint32, *chainhash.Hash, error) {
	tipRes, err := b.client.BTCHeaderChainTip()
	if err != nil {
		return 0, nil, fmt.Errorf("failed to get Babylon BTC header chain tip: %w", err)
	}

	// Parse the hash from hex string
	hash, err := chainhash.NewHashFromStr(tipRes.Header.HashHex)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to parse tip hash: %w", err)
	}

	return tipRes.Header.Height, hash, nil
}

// SubmitHeaders submits a batch of BTC headers to Babylon.
// It converts the raw 80-byte headers to MsgInsertHeaders format and sends via the Babylon client.
func (b *BabylonBackend) SubmitHeaders(ctx context.Context, startHeight uint64, headers [][]byte) error {
	if len(headers) == 0 {
		return fmt.Errorf("no headers to submit")
	}

	// Convert raw headers to IndexedBlock format for MsgInsertHeaders
	ibs := make([]*types.IndexedBlock, len(headers))
	for i, headerBytes := range headers {
		if len(headerBytes) != 80 {
			return fmt.Errorf("invalid header length at index %d: got %d, want 80", i, len(headerBytes))
		}

		// Parse the 80-byte header into wire.BlockHeader
		var header wire.BlockHeader
		if err := header.Deserialize(bytes.NewReader(headerBytes)); err != nil {
			return fmt.Errorf("failed to parse header at index %d: %w", i, err)
		}

		// Create IndexedBlock with just the header (no transactions needed for header submission)
		// #nosec G115 -- Bitcoin block heights are well below uint32 max, i is bounded by headers length
		ibs[i] = types.NewIndexedBlock(uint32(startHeight)+uint32(i), &header, nil)
	}

	// Create MsgInsertHeaders
	signer := b.client.MustGetAddr()
	msg := types.NewMsgInsertHeaders(signer, ibs)

	// Submit to Babylon with expected errors
	expectedErrors := []*coserrors.Error{btclctypes.ErrForkStartWithKnownHeader}
	res, err := b.client.ReliablySendMsg(ctx, msg, expectedErrors, nil)
	if err != nil {
		return fmt.Errorf("failed to submit headers to Babylon: %w", err)
	}

	// Check if res == nil, which indicates an expected error occurred (ErrForkStartWithKnownHeader)
	// This means some/all of these headers are already in Babylon (race condition with another reporter)
	// Return the error so ProcessHeaders can handle it appropriately
	if res == nil {
		return btclctypes.ErrForkStartWithKnownHeader
	}

	return nil
}

// Stop gracefully shuts down the Babylon client.
func (b *BabylonBackend) Stop() error {
	return b.client.Stop()
}
