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

// pgUniqueViolation is SQLSTATE 23505. Kept as a literal so the store
// package does not depend on a separate pgerrcode module.
const pgUniqueViolation = "23505"
