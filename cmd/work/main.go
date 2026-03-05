package main

import (
	"polis/work/internal/cli"

	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	root := cli.NewRoot(version)
	cobra.CheckErr(root.Execute())
}
