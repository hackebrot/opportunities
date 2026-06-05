package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/hackebrot/opportunities/internal/model"
)

// ApplicationParams is the writable subset of model.Application for
// InsertApplication. Server-managed columns (id, created_at, updated_at)
// and the status-machine columns (archived_at, archive_reason*) are not
// part of the create path — those are written by the events engine when
// terminal transitions land.
type ApplicationParams struct {
	OpportunityID    string
	AppliedAt        *time.Time
	AppliedWithEmail string
	Notes            string
}

// applicationColumns lists the applications table columns in the order
// scanApplication expects.
const applicationColumns = `id, opportunity_id, applied_at, applied_with_email,
	status, archived_at, archive_reason_category, archive_reason,
	follow_up_blocked, last_followed_up_at, notes, created_at, updated_at`

// InsertApplication inserts an application with the given initial status
// and returns the persisted row. q may be the pool or a transaction so
// the insert can join an enclosing tx that emits the matching event and
// recomputes latest_status atomically (see service.AddApplication).
//
// A partial-unique-index violation (uq_active_app_per_opportunity) means
// the opportunity already has an active application; it surfaces as
// ErrActiveExists. A missing opportunity_id surfaces as ErrNotFound.
func (s *Store) InsertApplication(ctx context.Context, q Querier, p ApplicationParams, status string) (model.Application, error) {
	const query = `
		INSERT INTO applications (opportunity_id, applied_at,
			applied_with_email, status, notes)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING ` + applicationColumns

	row := q.QueryRow(
		ctx, query,
		p.OpportunityID, p.AppliedAt, p.AppliedWithEmail, status, p.Notes,
	)
	a, err := scanApplication(row)
	if err != nil {
		return model.Application{}, translateApplicationErr("create", err)
	}
	return a, nil
}

// GetApplication returns the application by id, or ErrNotFound.
func (s *Store) GetApplication(ctx context.Context, id string) (model.Application, error) {
	const q = `SELECT ` + applicationColumns + `
		FROM applications
		WHERE id = $1`
	a, err := scanApplication(s.Pool.QueryRow(ctx, q, id))
	if err != nil {
		return model.Application{}, translateApplicationErr("get", err)
	}
	return a, nil
}

// ListApplications returns all applications, most-recently-created first.
func (s *Store) ListApplications(ctx context.Context) ([]model.Application, error) {
	const q = `SELECT ` + applicationColumns + `
		FROM applications
		ORDER BY created_at DESC, id`
	rows, err := s.Pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("store: list applications: %w", err)
	}
	defer rows.Close()

	var out []model.Application
	for rows.Next() {
		a, err := scanApplication(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list applications: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list applications: %w", err)
	}
	return out, nil
}

// SetApplicationStatus writes the status-machine columns of id and bumps
// updated_at. q may be the pool or a transaction so the write can join
// an enclosing tx that emits the matching event and recomputes
// latest_status atomically (see service.AppendEvent). Passing
// archivedAt = nil leaves the column NULL — used by non-terminal
// transitions (screen → in_progress, offer); terminal transitions
// (accepted/rejected/declined/withdrawn) pass the event's occurred_at so
// the application's archived_at mirrors it.
//
// A bad status or archive_reason_category combo trips
// applications_status_chk or applications_archive_reason_chk and
// surfaces as ErrConflict; a partial-unique-index violation on a
// status flip back into the active set surfaces as ErrActiveExists.
// Missing id is ErrNotFound.
func (s *Store) SetApplicationStatus(ctx context.Context, q Querier, id, status string, archivedAt *time.Time, archiveReasonCategory *string) error {
	const query = `
		UPDATE applications
		SET status = $2,
			archived_at = $3,
			archive_reason_category = $4,
			updated_at = now()
		WHERE id = $1`
	tag, err := q.Exec(ctx, query, id, status, archivedAt, archiveReasonCategory)
	if err != nil {
		return translateApplicationErr("set status", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateApplication overwrites the editable, non-status-machine fields of
// id and bumps updated_at. Status and the archived_at / archive_reason*
// columns are deliberately left untouched — those belong to the events
// engine via SetApplicationStatus. Returns the post-update row.
func (s *Store) UpdateApplication(ctx context.Context, id string, p ApplicationParams) (model.Application, error) {
	const q = `
		UPDATE applications
		SET opportunity_id = $2,
			applied_at = $3,
			applied_with_email = $4,
			notes = $5,
			updated_at = now()
		WHERE id = $1
		RETURNING ` + applicationColumns

	row := s.Pool.QueryRow(
		ctx, q, id,
		p.OpportunityID, p.AppliedAt, p.AppliedWithEmail, p.Notes,
	)
	a, err := scanApplication(row)
	if err != nil {
		return model.Application{}, translateApplicationErr("update", err)
	}
	return a, nil
}

// DeleteApplication removes the application by id. Missing id is
// ErrNotFound.
func (s *Store) DeleteApplication(ctx context.Context, id string) error {
	const q = `DELETE FROM applications WHERE id = $1`
	tag, err := s.Pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("store: delete application: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanApplication(r rowScanner) (model.Application, error) {
	var a model.Application
	err := r.Scan(
		&a.ID, &a.OpportunityID, &a.AppliedAt, &a.AppliedWithEmail,
		&a.Status, &a.ArchivedAt, &a.ArchiveReasonCategory, &a.ArchiveReason,
		&a.FollowUpBlocked, &a.LastFollowedUpAt, &a.Notes,
		&a.CreatedAt, &a.UpdatedAt,
	)
	return a, err
}

func translateApplicationErr(op string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if pg, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch pg.Code {
		case pgUniqueViolation:
			// The partial unique index enforces one active application per
			// opportunity; everything else on this table is keyed on a
			// server-generated UUID and isn't reachable in practice, but
			// gate on the constraint name so a future schema change can't
			// silently widen ErrActiveExists.
			if pg.ConstraintName == "uq_active_app_per_opportunity" {
				return ErrActiveExists
			}
			return fmt.Errorf("%w: %s", ErrConflict, pg.ConstraintName)
		case pgForeignKeyViolation:
			// opportunity_id pointed at an opportunity that doesn't
			// exist. Name the entity so the caller isn't left guessing.
			return fmt.Errorf("%w: unknown opportunity ID", ErrNotFound)
		case pgCheckViolation:
			// applications_status_chk / applications_archive_reason_chk:
			// a bad status or archive_reason landed in the row. Surface
			// as a conflict instead of leaking the raw driver error.
			return fmt.Errorf("%w: invalid application (%s)", ErrConflict, pg.ConstraintName)
		}
	}
	return fmt.Errorf("store: %s application: %w", op, err)
}
