package container

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
	dockerBabylondRepository = "babylonlabs-io/babylond"
	dockerBabylondVersionTag = "latest" // todo(Lazar): we need version b1e255a
)

// NewImageConfig returns ImageConfig needed for running e2e test.
func NewImageConfig() ImageConfig {
	return ImageConfig{
		BitcoindRepository: dockerBitcoindRepository,
		BitcoindVersion:    dockerBitcoindVersionTag,
		BabylonRepository:  dockerBabylondRepository,
		BabylonVersion:     dockerBabylondVersionTag,
	}
}
