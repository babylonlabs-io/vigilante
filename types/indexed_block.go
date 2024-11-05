package types

import (
	"bytes"
	"fmt"

	babylontypes "github.com/babylonlabs-io/babylon/types"
	btcctypes "github.com/babylonlabs-io/babylon/x/btccheckpoint/types"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// IndexedBlock is a BTC block with some extra information compared to wire.MsgBlock, including:
// - block height
// - txHash, txHashWitness, txIndex for each Tx
// These are necessary for generating Merkle proof (and thus the `MsgInsertBTCSpvProof` message in babylon) of a certain tx
type IndexedBlock struct {
	Height uint32
	Header *wire.BlockHeader
	Txs    []*btcutil.Tx
}

func NewIndexedBlock(height uint32, header *wire.BlockHeader, txs []*btcutil.Tx) *IndexedBlock {
	return &IndexedBlock{height, header, txs}
}

func NewIndexedBlockFromMsgBlock(height uint32, block *wire.MsgBlock) *IndexedBlock {
	return &IndexedBlock{
		height,
		&block.Header,
		GetWrappedTxs(block),
	}
}

func (ib *IndexedBlock) MsgBlock() *wire.MsgBlock {
	msgTxs := make([]*wire.MsgTx, 0, len(ib.Txs))
	for _, tx := range ib.Txs {
		msgTxs = append(msgTxs, tx.MsgTx())
	}

	return &wire.MsgBlock{
		Header:       *ib.Header,
		Transactions: msgTxs,
	}
}

func (ib *IndexedBlock) BlockHash() chainhash.Hash {
	return ib.Header.BlockHash()
}

// GenSPVProof generates a Merkle proof of a certain tx with index txIdx
func (ib *IndexedBlock) GenSPVProof(txIdx int) (*btcctypes.BTCSpvProof, error) {
	if txIdx < 0 {
		return nil, fmt.Errorf("transaction index should not be negative")
	}
	if txIdx >= len(ib.Txs) {
		return nil, fmt.Errorf("transaction index is out of scope: idx=%d, len(Txs)=%d", txIdx, len(ib.Txs))
	}

	headerBytes := babylontypes.NewBTCHeaderBytesFromBlockHeader(ib.Header)

	var txsBytes [][]byte
	for _, tx := range ib.Txs {
		var txBuf bytes.Buffer
		if err := tx.MsgTx().Serialize(&txBuf); err != nil {
			return nil, err
		}
		txBytes := txBuf.Bytes()
		txsBytes = append(txsBytes, txBytes)
	}

	return btcctypes.SpvProofFromHeaderAndTransactions(&headerBytes, txsBytes, uint(txIdx))
}
