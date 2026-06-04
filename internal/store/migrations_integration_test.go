//go:build integration

package store

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5/pgxpool"
)

// expectedTables is the schema's table list (excluding goose's own
// goose_db_version bookkeeping table). Keep in sync with
// db/migrations/00001_init.sql.
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

	store := startPostgresStore(ctx, t)

	if err := store.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	if got := listAppTables(ctx, t, store.Pool); !cmp.Equal(expectedTables, got) {
		t.Fatalf("tables after up (-want +got):\n%s", cmp.Diff(expectedTables, got))
	}

	if err := store.MigrateDownTo(ctx, 0); err != nil {
		t.Fatalf("migrate down-to 0: %v", err)
	}
	if got := listAppTables(ctx, t, store.Pool); len(got) != 0 {
		t.Fatalf("tables after down: want none, got %v", got)
	}

	// Round-trip: up again to confirm migrations are re-runnable.
	if err := store.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up (second time): %v", err)
	}
	if got := listAppTables(ctx, t, store.Pool); !cmp.Equal(expectedTables, got) {
		t.Fatalf("tables after second up (-want +got):\n%s", cmp.Diff(expectedTables, got))
	}
}

// listAppTables returns the user-defined tables in the public schema,
// excluding goose's bookkeeping table.
func listAppTables(ctx context.Context, t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()

	const q = `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_name <> 'goose_db_version'
		ORDER BY table_name
	`
	rows, err := pool.Query(ctx, q)
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
