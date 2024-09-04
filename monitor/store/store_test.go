package store_test

import (
	"github.com/babylonlabs-io/vigilante/monitor/store"
	"github.com/babylonlabs-io/vigilante/testutil"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestEmptyStore(t *testing.T) {
	db := testutil.MakeTestBackend(t)
	s, err := store.NewMonitorStore(db)
	require.NoError(t, err)

	e, err := s.LatestEpoch()
	require.Equal(t, e, uint64(0))
	require.NoError(t, err)
}
