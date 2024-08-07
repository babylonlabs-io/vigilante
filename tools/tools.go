//go:build tools
// +build tools

package vigilante

import (
	_ "github.com/babylonlabs-io/babylon/cmd/babylond"
	_ "github.com/btcsuite/btcd"
	_ "github.com/btcsuite/btcwallet"
)
