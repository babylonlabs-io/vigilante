package types

import (
	"errors"
	"github.com/babylonlabs-io/babylon/crypto/bls12381"
	"github.com/babylonlabs-io/babylon/x/checkpointing/types"
	"github.com/cometbft/cometbft/crypto/ed25519"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
)

func validPop(blsPubkey bls12381.PublicKey, valPubkey cryptotypes.PubKey, ed25519Sig, blsSig []byte) bool {
	ok, _ := bls12381.Verify(blsSig, blsPubkey, ed25519Sig)
	if !ok {
		return false
	}
	ed25519PK := ed25519.PubKey(valPubkey.Bytes())

	return ed25519PK.VerifySignature(blsPubkey, ed25519Sig)
}

func validateGenesisKeys(genesisKeys []*types.GenesisKey) error {
	addresses := make(map[string]struct{})
	for _, gk := range genesisKeys {
		if _, exists := addresses[gk.ValidatorAddress]; exists {
			return errors.New("duplicate genesis key")
		}
		addresses[gk.ValidatorAddress] = struct{}{}

		if !validPop(*gk.BlsKey.Pubkey, gk.ValPubkey, gk.BlsKey.Pop.Ed25519Sig, gk.BlsKey.Pop.BlsSig.Bytes()) {
			return types.ErrInvalidPoP
		}
	}

	return nil
}
