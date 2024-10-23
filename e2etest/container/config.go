package container

import (
	"github.com/babylonlabs-io/vigilante/testutil"
	"github.com/stretchr/testify/require"
	"testing"
)

// ImageConfig contains all images and their respective tags
// needed for running e2e tests.
type ImageConfig struct {
	BitcoindRepository string
	BitcoindVersion    string
	BabylonRepository  string
	BabylonVersion     string
}

//nolint:deadcode
const (
	dockerBitcoindRepository = "lncm/bitcoind"
	dockerBitcoindVersionTag = "v27.0"
	dockerBabylondRepository = "babylonlabs/babylond"
)

// NewImageConfig returns ImageConfig needed for running e2e test.
func NewImageConfig(t *testing.T) ImageConfig {
	babylondVersion, err := testutil.GetBabylonVersion()
	require.NoError(t, err)
	babylondVersion = "6a0ccacc41435a249316a77fe6e1e06aeb654d13" // todo(lazar): remove this when we have a tag

	return ImageConfig{
		BitcoindRepository: dockerBitcoindRepository,
		BitcoindVersion:    dockerBitcoindVersionTag,
		BabylonRepository:  dockerBabylondRepository,
		BabylonVersion:     babylondVersion,
	}
}
