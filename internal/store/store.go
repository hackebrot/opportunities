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
// migrations. The returned *sql.DB shares the pool; Close it after use
// to release the borrowed conn back to the pool.
func (s *Store) sqlDB() *sql.DB {
	return stdlib.OpenDBFromPool(s.Pool)
}

// MigrateUp applies every pending migration.
func (s *Store) MigrateUp(ctx context.Context) error {
	db := s.sqlDB()
	defer db.Close()
	if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
		return fmt.Errorf("store: migrate up: %w", err)
	}
	return nil
}

// MigrateDown rolls back the most recently applied migration.
func (s *Store) MigrateDown(ctx context.Context) error {
	db := s.sqlDB()
	defer db.Close()
	if err := goose.DownContext(ctx, db, migrationsDir); err != nil {
		return fmt.Errorf("store: migrate down: %w", err)
	}
	return nil
}

// MigrateDownTo rolls back migrations down to (but not past) version.
// Pass 0 to roll back every applied migration.
func (s *Store) MigrateDownTo(ctx context.Context, version int64) error {
	db := s.sqlDB()
	defer db.Close()
	if err := goose.DownToContext(ctx, db, migrationsDir, version); err != nil {
		return fmt.Errorf("store: migrate down-to %d: %w", version, err)
	}
	return nil
}

// MigrateStatus prints applied/pending migrations through goose's
// configured logger.
func (s *Store) MigrateStatus(ctx context.Context) error {
	db := s.sqlDB()
	defer db.Close()
	if err := goose.StatusContext(ctx, db, migrationsDir); err != nil {
		return fmt.Errorf("store: migrate status: %w", err)
	}
	return nil
}

// MigrateRedo rolls the latest migration back and re-applies it.
func (s *Store) MigrateRedo(ctx context.Context) error {
	db := s.sqlDB()
	defer db.Close()
	if err := goose.RedoContext(ctx, db, migrationsDir); err != nil {
		return fmt.Errorf("store: migrate redo: %w", err)
	}
	return nil
}

// Reset rolls every migration back and re-applies them. Powers
// `opps db reset --yes` — destroys the schema's data.
func (s *Store) Reset(ctx context.Context) error {
	db := s.sqlDB()
	defer db.Close()
	if err := goose.DownToContext(ctx, db, migrationsDir, 0); err != nil {
		return fmt.Errorf("store: reset (down): %w", err)
	}
	if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
		return fmt.Errorf("store: reset (up): %w", err)
	}
	return nil
}
