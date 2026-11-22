// Command autobrowser is the Go port of the autobrowser CLI.
package main

import (
	"os"

	"github.com/foxzi/autobrowser-client/go/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr))
}
