package types // nolint:revive

import (
	"testing"

	"github.com/babylonlabs-io/babylon/v3/crypto/bls12381"
	"github.com/babylonlabs-io/babylon/v3/x/checkpointing/types"
	cmtcrypto "github.com/cometbft/cometbft/crypto"
	"github.com/cometbft/cometbft/crypto/ed25519"
	"github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/stretchr/testify/require"
)

func TestProofOfPossession_ValidPop(t *testing.T) {
	t.Parallel()
	valPrivKey := ed25519.GenPrivKey()
	blsPrivKey := bls12381.GenPrivKey()
	pop := buildPoP(t, valPrivKey, blsPrivKey)
	valpk, err := codec.FromCmtPubKeyInterface(valPrivKey.PubKey())
	require.NoError(t, err)
	require.True(t, validPop(blsPrivKey.PubKey(), valpk, pop.Ed25519Sig, pop.BlsSig.Bytes()))
}

func buildPoP(t *testing.T, valPrivKey cmtcrypto.PrivKey, blsPrivkey bls12381.PrivateKey) *types.ProofOfPossession {
	t.Helper()
	require.NotNil(t, valPrivKey)
	require.NotNil(t, blsPrivkey)
	data, err := valPrivKey.Sign(blsPrivkey.PubKey().Bytes())
	require.NoError(t, err)

	pop := bls12381.Sign(blsPrivkey, data)

	return &types.ProofOfPossession{
		Ed25519Sig: data,
		BlsSig:     &pop,
	}
}
