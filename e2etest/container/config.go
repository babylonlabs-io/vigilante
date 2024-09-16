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
)

// NewImageConfig returns ImageConfig needed for running e2e test.
func NewImageConfig() ImageConfig {
	config := ImageConfig{
		BitcoindRepository: dockerBitcoindRepository,
		BitcoindVersion:    dockerBitcoindVersionTag,
		BabylonRepository:  "babylonlabs-io/babylond",
		BabylonVersion:     "latest",
	}
	return config
}
