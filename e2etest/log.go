package e2etest

import (
	"github.com/babylonlabs-io/vigilante/config"
)

var (
	logger, _ = config.NewRootLogger("auto", "debug")
	log       = logger.Sugar()
)
