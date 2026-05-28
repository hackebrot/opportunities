// Command opps is the local-first CLI for tracking job opportunities,
// applications, contacts, and compensation.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hackebrot/opportunities/internal/cli"
	"github.com/hackebrot/opportunities/internal/prompt"
)

// version is overridden at build time via:
//
//	go build -ldflags "-X main.version=$(git describe --tags --always)"
var version = "dev"

// systemClock is the production service.Clock. It lives in main because
// the service layer is forbidden from calling time.Now() directly.
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func main() {
	// Cancel the root context on SIGINT/SIGTERM so long-running
	// operations (migrations, queries) abort cleanly instead of leaving
	// the pool mid-flight.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx = prompt.WithInterface(ctx, prompt.Huh{})
	ctx = cli.WithClock(ctx, systemClock{})

	if err := cli.NewRoot(version).ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "opps:", err)
		os.Exit(1)
	}
}
