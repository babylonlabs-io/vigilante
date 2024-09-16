package store_test

import (
	bbndatagen "github.com/babylonlabs-io/babylon/testutil/datagen"
	"github.com/babylonlabs-io/vigilante/submitter/store"
	"github.com/babylonlabs-io/vigilante/testutil"
	"github.com/babylonlabs-io/vigilante/testutil/datagen"
	"github.com/stretchr/testify/require"
	"math/rand"
	"testing"
)

func FuzzStoringCkpt(f *testing.F) {
	bbndatagen.AddRandomSeedsToFuzzer(f, 3)

	f.Fuzz(func(t *testing.T, seed int64) {
		t.Parallel()
		r := rand.New(rand.NewSource(seed))
		db := testutil.MakeTestBackend(t)
		s, err := store.NewSubmitterStore(db)
		require.NoError(t, err)

		_, exists, err := s.LatestCheckpoint()
		require.NoError(t, err)
		require.False(t, exists)

		epoch := uint64(r.Int63n(1000) + 1)
		tx1 := datagen.GenRandomTx(r)
		tx2 := datagen.GenRandomTx(r)
		err = s.PutCheckpoint(store.NewStoredCheckpoint(tx1, tx2, epoch))
		require.NoError(t, err)

		storedCkpt, exists, err := s.LatestCheckpoint()
		require.NoError(t, err)
		require.True(t, exists)
		require.Equal(t, storedCkpt.Epoch, epoch)
		require.Equal(t, storedCkpt.Tx1.TxHash().String(), tx1.TxHash().String())
		require.Equal(t, storedCkpt.Tx2.TxHash().String(), tx2.TxHash().String())
	})
}
