package testutil

import (
	"crypto/rand"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
	"math/big"
	"os"
	"testing"
)

func TempDir(t *testing.T) (string, error) {
	tempPath, err := os.MkdirTemp(os.TempDir(), randStr(t))
	if err != nil {
		return "", err
	}

	if err = os.Chmod(tempPath, 0777); err != nil {
		return "", err
	}

	t.Cleanup(func() {
		_ = os.RemoveAll(tempPath)
	})

	return tempPath, err
}

func randStr(t *testing.T) string {
	n, err := rand.Int(rand.Reader, big.NewInt(1e18))
	require.NoError(t, err)

	return hexutil.EncodeBig(n)
}
