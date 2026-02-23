package main

import (
	"fmt"
	"os"

	"polis/work/internal/cli"
)

var version = "dev"

func main() {
	root := cli.NewRoot(version)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
