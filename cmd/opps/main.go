// Command opps is the local-first CLI for tracking job opportunities,
// applications, contacts, and compensation.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/hackebrot/opportunities/internal/cli"
)

// version is overridden at build time via:
//
//	go build -ldflags "-X main.version=$(git describe --tags --always)"
var version = "dev"

func main() {
	// Cancel the root context on SIGINT/SIGTERM so long-running
	// operations (migrations, queries) abort cleanly instead of leaving
	// the pool mid-flight.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := cli.NewRoot(version).ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "opps:", err)
		os.Exit(1)
	}
}
