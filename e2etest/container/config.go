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
	ElectrsRepository  string
	ElectrsVersion     string
}

//nolint:deadcode
const (
	dockerBitcoindRepository      = "lncm/bitcoind"
	dockerBitcoindVersionTag      = "v28.0"
	dockerBabylondRepository      = "babylonlabs/babylond"
	dockerBabylondMultisigVersion = "c8ef72fa5bc713f175306b750757ce03af0ffb52"
	dockerElectrsRepository       = "mempool/electrs"
	dockerElectrsVersionTag       = "v3.1.0"
)

// NewImageConfig returns ImageConfig needed for running e2e test.
func NewImageConfig(t *testing.T) ImageConfig {
	babylonVersion, err := testutil.GetBabylonVersion()
	require.NoError(t, err)

	return ImageConfig{
		BitcoindRepository: dockerBitcoindRepository,
		BitcoindVersion:    dockerBitcoindVersionTag,
		BabylonRepository:  dockerBabylondRepository,
		BabylonVersion:     babylonVersion,
		ElectrsRepository:  dockerElectrsRepository,
		ElectrsVersion:     dockerElectrsVersionTag,
	}
}
