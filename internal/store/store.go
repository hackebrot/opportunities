// Package store owns the Postgres connection pool and the embedded
// goose migration operations that back the `opps db` subcommand. It is
// the single entry point that other packages use to reach the database.
package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Store owns a pgx connection pool plus the migration operations that
// back the `opps db` subcommand.
type Store struct {
	Pool *pgxpool.Pool
}

// Open establishes a pgxpool against dsn. Connections are acquired
// lazily; the first acquisition performs the actual handshake.
func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open pool: %w", err)
	}
	return &Store{Pool: pool}, nil
}

// Close releases the underlying pool.
func (s *Store) Close() {
	s.Pool.Close()
}

// sqlDB bridges the pgxpool to database/sql so goose can drive
// migrations. Each Migrate*/Reset call opens a fresh *sql.DB that
// borrows from the pool for the duration of that one operation, then
// Closes it to return connections to the pool.
//
// Callers ignore the *sql.DB.Close error on purpose: the bridge does
// not own the pool, so "close" just releases borrowed conns; by the
// time it runs the migration result is already known, and surfacing a
// release error would mask the more useful goose error. The underlying
// pool stays alive on Store and is closed via Store.Close.
func (s *Store) sqlDB() *sql.DB {
	return stdlib.OpenDBFromPool(s.Pool)
}

// MigrateUp applies every pending migration.
func (s *Store) MigrateUp(ctx context.Context) error {
	db := s.sqlDB()
	defer func() { _ = db.Close() }()
	if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
		return fmt.Errorf("store: migrate up: %w", err)
	}
	return nil
}

// MigrateDown rolls back the most recently applied migration.
func (s *Store) MigrateDown(ctx context.Context) error {
	db := s.sqlDB()
	defer func() { _ = db.Close() }()
	if err := goose.DownContext(ctx, db, migrationsDir); err != nil {
		return fmt.Errorf("store: migrate down: %w", err)
	}
	return nil
}

// MigrateDownTo rolls back migrations down to (but not past) version.
// Pass 0 to roll back every applied migration.
func (s *Store) MigrateDownTo(ctx context.Context, version int64) error {
	db := s.sqlDB()
	defer func() { _ = db.Close() }()
	if err := goose.DownToContext(ctx, db, migrationsDir, version); err != nil {
		return fmt.Errorf("store: migrate down-to %d: %w", version, err)
	}
	return nil
}

// MigrateStatus prints applied/pending migrations through goose's
// configured logger.
func (s *Store) MigrateStatus(ctx context.Context) error {
	db := s.sqlDB()
	defer func() { _ = db.Close() }()
	if err := goose.StatusContext(ctx, db, migrationsDir); err != nil {
		return fmt.Errorf("store: migrate status: %w", err)
	}
	return nil
}

// MigrateRedo rolls the latest migration back and re-applies it.
func (s *Store) MigrateRedo(ctx context.Context) error {
	db := s.sqlDB()
	defer func() { _ = db.Close() }()
	if err := goose.RedoContext(ctx, db, migrationsDir); err != nil {
		return fmt.Errorf("store: migrate redo: %w", err)
	}
	return nil
}

// Reset rolls every migration back and re-applies them. Powers
// `opps db reset --yes` — destroys the schema's data.
//
// A failure in the down phase short-circuits before re-applying, which
// can leave the schema partially torn down. The intended recovery is to
// rerun reset (or `db migrate up`) — both are idempotent against a
// partial state.
func (s *Store) Reset(ctx context.Context) error {
	db := s.sqlDB()
	defer func() { _ = db.Close() }()
	if err := goose.DownToContext(ctx, db, migrationsDir, 0); err != nil {
		return fmt.Errorf("store: reset (down): %w", err)
	}
	if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
		return fmt.Errorf("store: reset (up): %w", err)
	}
	return nil
}
