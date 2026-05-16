//go:build integration

package store

import (
	"context"
	"database/sql"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// expectedTables is the schema's table list (excluding goose's own
// goose_db_version bookkeeping table). Keep in sync with
// db/migrations/0001_init.up.sql.
var expectedTables = []string{
	"application_stages",
	"applications",
	"companies",
	"compensations",
	"contacts",
	"events",
	"opportunities",
	"opportunity_contacts",
}

func TestIntegrationMigrationsRoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	sqlDB := startPostgres(ctx, t)

	if err := migrateUp(ctx, sqlDB); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	if diff := cmp.Diff(expectedTables, listAppTables(ctx, t, sqlDB)); diff != "" {
		t.Fatalf("tables after up (-want +got):\n%s", diff)
	}

	if err := migrateDown(ctx, sqlDB); err != nil {
		t.Fatalf("migrate down: %v", err)
	}
	if got := listAppTables(ctx, t, sqlDB); len(got) != 0 {
		t.Fatalf("tables after down: want none, got %v", got)
	}

	// Round-trip: up again to confirm migrations are re-runnable.
	if err := migrateUp(ctx, sqlDB); err != nil {
		t.Fatalf("migrate up (second time): %v", err)
	}
	if diff := cmp.Diff(expectedTables, listAppTables(ctx, t, sqlDB)); diff != "" {
		t.Fatalf("tables after second up (-want +got):\n%s", diff)
	}
}

// startPostgres spins up an ephemeral Postgres container and returns a
// *sql.DB connected to it. The container is terminated on test cleanup.
func startPostgres(ctx context.Context, t *testing.T) *sql.DB {
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

	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Logf("close sql.DB: %v", err)
		}
	})

	if err := sqlDB.PingContext(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return sqlDB
}

// listAppTables returns the user-defined tables in the public schema,
// excluding goose's bookkeeping table.
func listAppTables(ctx context.Context, t *testing.T, sqlDB *sql.DB) []string {
	t.Helper()

	const q = `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_name <> 'goose_db_version'
		ORDER BY table_name
	`
	rows, err := sqlDB.QueryContext(ctx, q)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	sort.Strings(names)
	return names
}

// Force-import the pgx stdlib driver so sql.Open("pgx", ...) works.
var _ = stdlib.GetDefaultDriver
