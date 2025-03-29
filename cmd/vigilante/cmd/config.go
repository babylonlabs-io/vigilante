package cmd

import (
	"fmt"
	"github.com/babylonlabs-io/vigilante/config"
	"github.com/spf13/cobra"
	"os"
	"path"
)

const (
	configFileDirFlag = "config-file-dir"
)

// CommandDumpConfig returns the command to dump the default vigilante config
func CommandDumpConfig() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "dump-cfg",
		Aliases: []string{"dc"},
		Short:   "Dumps the default vigilante at the specified path",
		Example: `vigilante dump-cfg --config-file-dir /path/to/config.yml`,
		Args:    cobra.NoArgs,
		RunE:    dumpConfig,
	}
	cmd.Flags().String(configFileDirFlag, "", "Path to where the default config file will be dumped")

	return cmd
}

func dumpConfig(cmd *cobra.Command, _ []string) error {
	configPath, err := cmd.Flags().GetString(configFileDirFlag)
	if err != nil {
		return fmt.Errorf("failed to read flag %s: %w", configFileDirFlag, err)
	}

	if fileExists(configPath) {
		return fmt.Errorf("config already exists under provided path: %s", configPath)
	}

	dir, _ := path.Split(configPath)

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, os.ModeDir); err != nil {
			return fmt.Errorf("could not create config directory: %w", err)
		}
	}

	defaultConfig := config.DefaultConfig()
	if err := defaultConfig.SaveToYAML(configPath); err != nil {
		return fmt.Errorf("failed to save config to file: %w", err)
	}

	return nil
}

func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}

	return true
}
