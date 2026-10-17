// Command okub prepares, validates and bootstraps bare-metal OKD clusters.
// See docs/architectures/00-general.md for the target architecture.
package main

import (
	"fmt"
	"os"
)

// version is overridden at build time:
// go build -ldflags "-X main.version=v0.1.0" ./cmd/okub
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Println("okub", version)
			return
		}
	}
	fmt.Fprintln(os.Stderr, "okub: not implemented yet — see docs/plan/summary.md")
	os.Exit(2)
}
