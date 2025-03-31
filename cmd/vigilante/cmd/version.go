package cmd

import (
	"github.com/babylonlabs-io/vigilante/version"
	"github.com/spf13/cobra"
	"strings"
)

// CommandVersion prints version of the binary
func CommandVersion() *cobra.Command {
	var cmd = &cobra.Command{
		Use:     "version",
		Short:   "Prints version of this binary.",
		Aliases: []string{"v"},
		Example: "vigilante version",
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			v := version.Version()
			commit, ts := version.CommitInfo()
			if v == "" {
				v = "main"
			}

			var sb strings.Builder
			_, _ = sb.WriteString("Version:       " + v)
			_, _ = sb.WriteString("\n")
			_, _ = sb.WriteString("Git Commit:    " + commit)
			_, _ = sb.WriteString("\n")
			_, _ = sb.WriteString("Git Timestamp: " + ts)
			_, _ = sb.WriteString("\n")

			cmd.Printf(sb.String()) //nolint:govet // it's not an issue
		},
	}

	return cmd
}
