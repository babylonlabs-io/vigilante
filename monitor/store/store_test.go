package store_test

import (
	bbndatagen "github.com/babylonlabs-io/babylon/testutil/datagen"
	"github.com/babylonlabs-io/vigilante/monitor/store"
	"github.com/babylonlabs-io/vigilante/testutil"
	"github.com/stretchr/testify/require"
	"math/rand"
	"testing"
)

func TestEmptyStore(t *testing.T) {
	t.Parallel()
	db := testutil.MakeTestBackend(t)
	s, err := store.NewMonitorStore(db)
	require.NoError(t, err)

	_, err = s.LatestEpoch()
	require.ErrorIs(t, err, store.ErrNotFound)
}

func FuzzStoringEpochs(f *testing.F) {
	// only 3 seeds as this is pretty slow test opening/closing db
	bbndatagen.AddRandomSeedsToFuzzer(f, 3)

	f.Fuzz(func(t *testing.T, seed int64) {
		t.Parallel()
		r := rand.New(rand.NewSource(seed))
		db := testutil.MakeTestBackend(t)
		s, err := store.NewMonitorStore(db)
		require.NoError(t, err)

		_, err = s.LatestEpoch()
		require.ErrorIs(t, err, store.ErrNotFound)

		epoch := uint64(r.Int63n(1000) + 1)
		err = s.PutLatestEpoch(epoch)
		require.NoError(t, err)

		storedLatestEpoch, err := s.LatestEpoch()
		require.NoError(t, err)
		require.Equal(t, epoch, storedLatestEpoch)
	})
}
