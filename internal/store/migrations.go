// Package store owns the Postgres connection pool and the schema
// migration entry points. The exported API is intentionally narrow:
// callers get a pool plus the small set of operations the `opps db`
// subcommand needs (up, down, status, redo, reset).
package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	"github.com/hackebrot/opportunities/db"
)

const migrationsDir = "migrations"

func init() {
	goose.SetBaseFS(db.Migrations)
	// goose's default logger prints to stdout on every migration;
	// the CLI owns its own status output.
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect("postgres"); err != nil {
		panic(fmt.Sprintf("store: set goose dialect: %v", err))
	}
}

// migrateUp applies every pending migration.
func migrateUp(ctx context.Context, sqlDB *sql.DB) error {
	if err := goose.UpContext(ctx, sqlDB, migrationsDir); err != nil {
		return fmt.Errorf("store: migrate up: %w", err)
	}
	return nil
}

// migrateDown rolls back every applied migration.
func migrateDown(ctx context.Context, sqlDB *sql.DB) error {
	if err := goose.DownToContext(ctx, sqlDB, migrationsDir, 0); err != nil {
		return fmt.Errorf("store: migrate down: %w", err)
	}
	return nil
}
