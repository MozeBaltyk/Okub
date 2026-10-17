// Command okub prepares, validates and bootstraps bare-metal OKD clusters.
// See docs/architectures/00-general.md for the target architecture.
package main

import (
	"os"

	"github.com/mozebaltyk/Okub/internal/cli"
)

// version is overridden at build time:
// go build -ldflags "-X main.version=v0.1.0" ./cmd/okub
var version = "dev"

func main() {
	os.Exit(cli.Execute(os.Args[1:], version, os.Stdout, os.Stderr))
}
