package testutil

import (
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/lightningnetwork/lnd/kvdb"
	"github.com/stretchr/testify/require"
	"testing"
)

func MakeTestBackend(t *testing.T) kvdb.Backend {
	tempDirName := t.TempDir()

	cfg := config.DefaultDBConfig()

	cfg.DBPath = tempDirName

	backend, err := cfg.GetDBBackend()
	require.NoError(t, err)

	t.Cleanup(func() {
		err := backend.Close()
		require.NoError(t, err)
	})

	return backend
}
