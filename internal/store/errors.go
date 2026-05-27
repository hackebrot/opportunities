package store

import "errors"

// Sentinel errors returned by store methods. Callers compare with
// errors.Is to map driver-specific failures (pgx.ErrNoRows, SQLSTATE
// 23505) onto a stable, package-public surface.
var (
	// ErrNotFound is returned when a lookup by id finds no row.
	ErrNotFound = errors.New("store: not found")

	// ErrConflict is returned when an insert or update violates a
	// unique constraint (e.g. duplicate company slug).
	ErrConflict = errors.New("store: conflict")
)

// SQLSTATE codes (class 23 — integrity constraint violation): 23505
// unique violation, 23503 foreign-key violation, 23514 check violation.
// Kept as literals so the store package does not depend on a separate
// pgerrcode module.
//
// See https://www.postgresql.org/docs/current/errcodes-appendix.html
const (
	pgUniqueViolation     = "23505"
	pgForeignKeyViolation = "23503"
	pgCheckViolation      = "23514"
)
