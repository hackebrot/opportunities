// Command opps is the local-first CLI for tracking job opportunities,
// applications, contacts, and compensation.
package main

import (
	"fmt"
	"os"

	"github.com/hackebrot/opportunities/internal/cli"
)

// version is overridden at build time via:
//
//	go build -ldflags "-X main.version=$(git describe --tags --always)"
var version = "dev"

func main() {
	if err := cli.NewRoot(version).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "opps:", err)
		os.Exit(1)
	}
}
