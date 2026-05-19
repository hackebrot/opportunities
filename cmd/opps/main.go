// Command opps is the local-first CLI for tracking job opportunities,
// applications, contacts, and compensation.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hackebrot/opportunities/internal/cli"
	"github.com/hackebrot/opportunities/internal/prompt"
)

// version is overridden at build time via:
//
//	go build -ldflags "-X main.version=$(git describe --tags --always)"
var version = "dev"

func main() {
	// Cancel the root context on SIGINT/SIGTERM so long-running
	// operations (migrations, queries) abort cleanly instead of leaving
	// the pool mid-flight.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx = prompt.WithInterface(ctx, prompt.Huh{})

	if err := cli.NewRoot(version).ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "opps:", err)
		os.Exit(1)
	}
}
