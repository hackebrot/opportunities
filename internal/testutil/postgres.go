//go:build integration

// Package testutil holds helpers shared across integration test suites.
// It is compiled only under the integration build tag and is never
// imported by production code.
package testutil

import (
	"context"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// StartPostgres spins up an ephemeral Postgres container and returns its
// connection string. The container is terminated on test cleanup. It does
// not import the store package, so white-box store tests can use it
// without an import cycle.
func StartPostgres(ctx context.Context, t *testing.T) string {
	t.Helper()

	pg, err := tcpg.Run(
		ctx, "postgres:16-alpine",
		tcpg.WithDatabase("opps_test"),
		tcpg.WithUsername("opps"),
		tcpg.WithPassword("opps"),
		tcpg.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(pg); err != nil {
			t.Logf("terminate container: %v", err)
		}
	})

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	return dsn
}
