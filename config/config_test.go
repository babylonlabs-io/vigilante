package config_test

import (
	"fmt"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestDefaultConfig tests the default configuration of the Vigilante application.
// SaveToYAML is used in the dump-cfg command, we want to make sure that all properties names are correctly saved
func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	defaultCfg := config.DefaultConfig()
	cfgPath := fmt.Sprintf("%s/%s", t.TempDir(), "config.yaml")

	err := defaultCfg.SaveToYAML(cfgPath)
	require.NoError(t, err, "failed to save default config to yaml")

	loadedCfg, err := config.New(cfgPath)
	require.NoError(t, err, "failed to load config from yaml")

	require.Equal(t, *defaultCfg, loadedCfg, "loaded config does not match default config")
}
