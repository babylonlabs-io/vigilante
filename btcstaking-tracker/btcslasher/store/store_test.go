package store_test

import (
	"math/rand"
	"testing"

	bbndatagen "github.com/babylonlabs-io/babylon/v3/testutil/datagen"
	"github.com/babylonlabs-io/vigilante/btcstaking-tracker/btcslasher/store"
	"github.com/babylonlabs-io/vigilante/testutil"
	"github.com/stretchr/testify/require"
)

func FuzzStoringHeight(f *testing.F) {
	bbndatagen.AddRandomSeedsToFuzzer(f, 3)

	f.Fuzz(func(t *testing.T, seed int64) {
		t.Parallel()
		r := rand.New(rand.NewSource(seed))
		db := testutil.MakeTestBackend(t)
		s, err := store.NewSlasherStore(db)
		require.NoError(t, err)

		_, exists, err := s.LastProcessedHeight()
		require.NoError(t, err)
		require.False(t, exists)

		height := uint64(r.Int63n(1000000) + 1)
		err = s.PutHeight(height)
		require.NoError(t, err)

		storedHeight, exists, err := s.LastProcessedHeight()
		require.NoError(t, err)
		require.True(t, exists)
		require.Equal(t, height, storedHeight)
	})
}