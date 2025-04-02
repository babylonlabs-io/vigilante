package types

import (
	babylontypes "github.com/babylonlabs-io/babylon/types"
	btcltypes "github.com/babylonlabs-io/babylon/x/btclightclient/types"
)

func NewMsgInsertHeaders(
	signer string,
	indexedBlocks []*IndexedBlock,
) *btcltypes.MsgInsertHeaders {
	headerBytes := make([]babylontypes.BTCHeaderBytes, len(indexedBlocks))
	for i, ib := range indexedBlocks {
		headerBytes[i] = babylontypes.NewBTCHeaderBytesFromBlockHeader(ib.Header)
	}

	return &btcltypes.MsgInsertHeaders{
		Signer:  signer,
		Headers: headerBytes,
	}
}
