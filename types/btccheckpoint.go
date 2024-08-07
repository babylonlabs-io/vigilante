package types

import (
	"fmt"

	"github.com/babylonlabs-io/babylon/btctxformatter"
	btcctypes "github.com/babylonlabs-io/babylon/x/btccheckpoint/types"
)

// MustNewMsgInsertBTCSpvProof returns a MsgInsertBTCSpvProof msg given the submitter address and SPV proofs of two BTC txs
func MustNewMsgInsertBTCSpvProof(submitter string, proofs []*btcctypes.BTCSpvProof) *btcctypes.MsgInsertBTCSpvProof {
	var err error
	if len(proofs) != btctxformatter.NumberOfParts {
		err = fmt.Errorf("incorrect number of proofs: want %d, got %d", btctxformatter.NumberOfParts, len(proofs))
		panic(err)
	}

	return &btcctypes.MsgInsertBTCSpvProof{
		Submitter: submitter,
		Proofs:    proofs,
	}
}
