package types

import (
	babylontypes "github.com/babylonlabs-io/babylon/types"
	btcltypes "github.com/babylonlabs-io/babylon/x/btclightclient/types"
)

func NewMsgInsertHeaders(
	signer string,
	headers []*IndexedBlock,
) *btcltypes.MsgInsertHeaders {
	headerBytes := make([]babylontypes.BTCHeaderBytes, len(headers))
	for i, h := range headers {
		header := h
		headerBytes[i] = babylontypes.NewBTCHeaderBytesFromBlockHeader(header.Header)
	}

	return &btcltypes.MsgInsertHeaders{
		Signer:  signer,
		Headers: headerBytes,
	}
}
