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

// OpportunityParams is the writable subset of model.Opportunity for
// Create. Server-managed columns (id, created_at, updated_at) and the
// status-machine columns (latest_status, archived_at, archive_reason) are
// not part of it — the latter are set via InsertOpportunity's initial
// status and SetLatestStatus.
type OpportunityParams struct {
	CompanyID         string
	RoleTitle         *string
	Location          string
	OfficeDaysPerWeek int
	Source            string
	SourceDetail      string
	Priority          string
	Notes             string
}

// opportunityColumns are selected with the o.* / comp.name prefixes so an
// opportunity row can be returned alongside its joined company name.
const opportunityColumns = `o.id, o.company_id, comp.name, o.role_title,
	o.location, o.office_days_per_week, o.source, o.source_detail,
	o.priority, o.latest_status, o.archived_at, o.archive_reason, o.notes,
	o.created_at, o.updated_at`

// opportunityOwnColumns lists the opportunities table's own columns
// (no prefix), for use in INSERT/UPDATE RETURNING clauses.
const opportunityOwnColumns = `id, company_id, role_title, location,
	office_days_per_week, source, source_detail, priority, latest_status,
	archived_at, archive_reason, notes, created_at, updated_at`

// InsertOpportunity inserts an opportunity with the given initial
// latest_status and returns the persisted row with its company name
// resolved. q may be the pool or a transaction, so a caller can write the
// opportunity and its initial event atomically. A company_id that
// references no company surfaces as ErrNotFound.
func (s *Store) InsertOpportunity(ctx context.Context, q Querier, p OpportunityParams, latestStatus string) (model.Opportunity, error) {
	const query = `
		WITH ins AS (
			INSERT INTO opportunities (company_id, role_title, location,
				office_days_per_week, source, source_detail, priority,
				latest_status, notes)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING ` + opportunityOwnColumns + `
		)
		SELECT ` + opportunityColumns + `
		FROM ins o
		JOIN companies comp ON comp.id = o.company_id`

	row := q.QueryRow(
		ctx, query,
		p.CompanyID, p.RoleTitle, p.Location, p.OfficeDaysPerWeek,
		p.Source, p.SourceDetail, p.Priority, latestStatus, p.Notes,
	)
	o, err := scanOpportunity(row)
	if err != nil {
		return model.Opportunity{}, translateOpportunityErr("create", err)
	}
	return o, nil
}

// GetOpportunity returns the opportunity by id with its company name, or
// ErrNotFound.
func (s *Store) GetOpportunity(ctx context.Context, id string) (model.Opportunity, error) {
	const q = `SELECT ` + opportunityColumns + `
		FROM opportunities o
		JOIN companies comp ON comp.id = o.company_id
		WHERE o.id = $1`
	o, err := scanOpportunity(s.Pool.QueryRow(ctx, q, id))
	if err != nil {
		return model.Opportunity{}, translateOpportunityErr("get", err)
	}
	return o, nil
}

// ListOpportunities returns all opportunities, most-recently-created
// first, each with its company name resolved.
func (s *Store) ListOpportunities(ctx context.Context) ([]model.Opportunity, error) {
	const q = `SELECT ` + opportunityColumns + `
		FROM opportunities o
		JOIN companies comp ON comp.id = o.company_id
		ORDER BY o.created_at DESC, o.id`
	rows, err := s.Pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("store: list opportunities: %w", err)
	}
	defer rows.Close()

	var out []model.Opportunity
	for rows.Next() {
		o, err := scanOpportunity(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list opportunities: %w", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list opportunities: %w", err)
	}
	return out, nil
}

// UpdateOpportunity overwrites the editable fields of id and bumps
// updated_at. It deliberately leaves latest_status, archived_at, and
// archive_reason untouched — those are owned by the status machine
// (InsertOpportunity / SetLatestStatus). Returns the post-update row.
func (s *Store) UpdateOpportunity(ctx context.Context, id string, p OpportunityParams) (model.Opportunity, error) {
	const q = `
		WITH upd AS (
			UPDATE opportunities
			SET company_id = $2,
				role_title = $3,
				location = $4,
				office_days_per_week = $5,
				source = $6,
				source_detail = $7,
				priority = $8,
				notes = $9,
				updated_at = now()
			WHERE id = $1
			RETURNING ` + opportunityOwnColumns + `
		)
		SELECT ` + opportunityColumns + `
		FROM upd o
		JOIN companies comp ON comp.id = o.company_id`

	row := s.Pool.QueryRow(
		ctx, q, id,
		p.CompanyID, p.RoleTitle, p.Location, p.OfficeDaysPerWeek,
		p.Source, p.SourceDetail, p.Priority, p.Notes,
	)
	o, err := scanOpportunity(row)
	if err != nil {
		return model.Opportunity{}, translateOpportunityErr("update", err)
	}
	return o, nil
}

// SetLatestStatus updates the denormalized latest_status and archived_at
// of id. Pass a non-nil archivedAt to mark the opportunity dead, nil to
// leave it active. Missing id is ErrNotFound. Called only by the service
// layer, which owns latest_status; q may be the pool or a transaction so
// the write can join an enclosing tx (e.g. the events-engine flow that
// appends an event and recomputes latest_status atomically).
func (s *Store) SetLatestStatus(ctx context.Context, q Querier, id, newStatus string, archivedAt *time.Time) error {
	const query = `
		UPDATE opportunities
		SET latest_status = $2,
			archived_at = $3,
			updated_at = now()
		WHERE id = $1`
	tag, err := q.Exec(ctx, query, id, newStatus, archivedAt)
	if err != nil {
		return fmt.Errorf("store: set latest_status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteOpportunity removes the opportunity by id. Missing id is
// ErrNotFound.
func (s *Store) DeleteOpportunity(ctx context.Context, id string) error {
	const q = `DELETE FROM opportunities WHERE id = $1`
	tag, err := s.Pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("store: delete opportunity: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanOpportunity(r rowScanner) (model.Opportunity, error) {
	var o model.Opportunity
	err := r.Scan(
		&o.ID, &o.CompanyID, &o.CompanyName, &o.RoleTitle, &o.Location,
		&o.OfficeDaysPerWeek, &o.Source, &o.SourceDetail, &o.Priority,
		&o.LatestStatus, &o.ArchivedAt, &o.ArchiveReason, &o.Notes,
		&o.CreatedAt, &o.UpdatedAt,
	)
	return o, err
}

func translateOpportunityErr(op string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if pg, ok := errors.AsType[*pgconn.PgError](err); ok && pg.Code == pgForeignKeyViolation {
		// company_id pointed at a company that doesn't exist. Name the
		// company so the caller isn't left guessing which entity is missing.
		return fmt.Errorf("%w: unknown company ID", ErrNotFound)
	}
	return fmt.Errorf("store: %s opportunity: %w", op, err)
}
