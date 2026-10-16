// Command okub prepares, validates and bootstraps OpenShift/OKD clusters.
// See docs/design.md for the target architecture.
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
	fmt.Fprintln(os.Stderr, "okub: not implemented yet — see docs/design.md")
	os.Exit(2)
}
