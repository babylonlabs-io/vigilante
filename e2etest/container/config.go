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
	dockerBitcoindRepository = "lncm/bitcoind"
	dockerBitcoindVersionTag = "v27.0"
	dockerBabylondRepository = "babylonlabs/babylond"
	dockerElectrsRepository  = "mempool/electrs"
	dockerElectrsVersionTag  = "v3.1.0"
)

// NewImageConfig returns ImageConfig needed for running e2e test.
func NewImageConfig(t *testing.T) ImageConfig {
	babylonVersion, err := testutil.GetBabylonVersion()
	require.NoError(t, err)

	babylonVersion = "df4bcf3c6b0b53de3704a2c062c8e758e3266692" // todo: remove this, latest release/v1/x commit

	return ImageConfig{
		BitcoindRepository: dockerBitcoindRepository,
		BitcoindVersion:    dockerBitcoindVersionTag,
		BabylonRepository:  dockerBabylondRepository,
		BabylonVersion:     babylonVersion,
		ElectrsRepository:  dockerElectrsRepository,
		ElectrsVersion:     dockerElectrsVersionTag,
	}
}
