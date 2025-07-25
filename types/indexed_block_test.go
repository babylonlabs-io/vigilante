// Copyright (c) 2022-2022 The Babylon developers
// Copyright (c) 2013-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
package types_test

import (
	"math/rand"
	"testing"

	"github.com/babylonlabs-io/babylon/v3/testutil/datagen"
	btcctypes "github.com/babylonlabs-io/babylon/v3/x/btccheckpoint/types"
	vdatagen "github.com/babylonlabs-io/vigilante/testutil/datagen"
	"github.com/babylonlabs-io/vigilante/types"
	"github.com/btcsuite/btcd/chaincfg"
	_ "github.com/btcsuite/btcd/database/ffldb"
	"github.com/stretchr/testify/require"
)

func FuzzIndexedBlock(f *testing.F) {
	datagen.AddRandomSeedsToFuzzer(f, 10)

	f.Fuzz(func(t *testing.T, seed int64) {
		t.Parallel()
		r := rand.New(rand.NewSource(seed))

		blocks, _, rawCkpts := vdatagen.GenRandomBlockchainWithBabylonTx(r, 100, 0, 0.4)
		for i, block := range blocks {
			ib := types.NewIndexedBlockFromMsgBlock(uint32(i), block)

			if rawCkpts[i] != nil { // Babylon tx
				spvProof, err := ib.GenSPVProof(1)
				require.NoError(t, err)

				parsedProof, err := btcctypes.ParseProof(
					spvProof.BtcTransaction,
					spvProof.BtcTransactionIndex,
					spvProof.MerkleNodes,
					spvProof.ConfirmingBtcHeader,
					chaincfg.SimNetParams.PowLimit,
				)

				require.NotNil(t, parsedProof)
				require.NoError(t, err)
			} else { // non-Babylon tx
				spvProof, err := ib.GenSPVProof(1)
				require.NoError(t, err) // GenSPVProof allows to generate spvProofs for non-Babylon tx

				parsedProof, err := btcctypes.ParseProof(
					spvProof.BtcTransaction,
					spvProof.BtcTransactionIndex,
					spvProof.MerkleNodes,
					spvProof.ConfirmingBtcHeader,
					chaincfg.SimNetParams.PowLimit,
				)

				require.Nil(t, parsedProof)
				require.Error(t, err)
			}
		}
	})
}
