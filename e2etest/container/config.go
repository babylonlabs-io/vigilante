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
	dockerBabylondRepository = "babylonlabs/babylond"
	dockerBabylondVersionTag = "8e0222804ed19b18d74d599b80baa18f05e87d8a" // this is built from commit b1e255a
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
