package cmd

import (
	"github.com/spf13/cobra"
)

// GetKeysCmd returns the keys command group
func GetKeysCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "keys",
		Short: "Key management commands",
		Long:  "Commands for managing cryptographic keys and keystores",
	}

	cmd.AddCommand(
		GetKeysEthCmd(),
	)

	return cmd
}
