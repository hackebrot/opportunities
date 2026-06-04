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

	// ErrActiveExists is returned when an applications insert or update
	// would violate the partial unique index uq_active_app_per_opportunity
	// — the opportunity already has an application in an active status
	// (applied/in_progress/offer). Updates can hit this when
	// opportunity_id is reassigned to an opportunity that already owns an
	// active application. Distinct from ErrConflict so callers can
	// present the user-facing "you already have an open application for
	// this opportunity" message instead of a generic conflict.
	ErrActiveExists = errors.New("store: active application already exists")
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
