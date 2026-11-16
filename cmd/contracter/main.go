// Contracter records the shape of a set of config files as a contract and
// checks any other copy of those files against it.
package main

import (
	"os"

	"contracter/internal/cli"
)

func main() { os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr)) }
