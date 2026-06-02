//go:build integration

package cli_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/hackebrot/opportunities/internal/cli"
	"github.com/hackebrot/opportunities/internal/store"
	"github.com/hackebrot/opportunities/internal/testutil"
)

// runCmd executes the root cobra command and fails the test on error.
// Returns stdout.
func runCmd(ctx context.Context, t *testing.T, args ...string) string {
	t.Helper()
	out, err := tryRun(ctx, t, args...)
	if err != nil {
		t.Fatalf("opps %v: %v\n%s", args, err, out)
	}
	return out
}

func tryRun(ctx context.Context, t *testing.T, args ...string) (string, error) {
	t.Helper()
	// main seeds a real clock into the command context; mirror that here,
	// since the service layer requires one and refuses to construct without.
	ctx = cli.WithClock(ctx, cliTestClock{})
	root := cli.NewRoot("v0.0.0-test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	return out.String(), err
}

// cliTestClock is the test analog of main's system clock. CLI integration
// tests don't assert on timestamps, so a live clock is fine.
type cliTestClock struct{}

func (cliTestClock) Now() time.Time { return time.Now() }

// startPostgresStore opens a *store.Store against an ephemeral Postgres
// container (see testutil.StartPostgres). Released on test cleanup.
func startPostgresStore(ctx context.Context, t *testing.T) *store.Store {
	t.Helper()

	s, err := store.Open(ctx, testutil.StartPostgres(ctx, t))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(s.Close)

	if err := s.Pool.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return s
}
