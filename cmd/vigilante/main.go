package main

import (
	"fmt"
	"github.com/babylonlabs-io/babylon/app/params"
	"github.com/babylonlabs-io/vigilante/cmd/vigilante/cmd"
	"os"
)

// TODO: init log

func main() {
	params.SetAddressPrefixes()

	rootCmd := cmd.NewRootCmd()

	if err := rootCmd.Execute(); err != nil {
		fmt.Print(err.Error())
		os.Exit(1)
	}
}
