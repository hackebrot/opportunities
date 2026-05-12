//go:build integration

package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// TestIntegrationPostgresSmoke is the canary for the integration tier:
// it spins up a Postgres container, connects, and pings. If Docker is
// missing or unreachable, this fails fast before any schema or store
// test does.
func TestIntegrationPostgresSmoke(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg, err := tcpg.Run(
		ctx, "postgres:16-alpine",
		tcpg.WithDatabase("opps_smoke"),
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

	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Logf("close conn: %v", err)
		}
	})

	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
}
