package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/hackebrot/opportunities/internal/model"
)

// CompensationParams is the writable subset of model.Compensation for
// InsertCompensation / UpdateCompensation. Server-managed columns (id,
// created_at, updated_at) are not part of it. Exactly one of
// OpportunityID / ApplicationID must be non-nil — the compensations_
// target_xor_chk CHECK enforces it, surfacing as ErrConflict when both or
// neither is set.
type CompensationParams struct {
	OpportunityID *string
	ApplicationID *string
	Kind          string
	BaseMinor     int64
	BonusMinor    *int64
	EquityNotes   string
	VestingNotes  string
	Currency      string
	Notes         string
}

// compensationColumns lists the compensations table columns in the order
// scanCompensation expects.
const compensationColumns = `id, opportunity_id, application_id, kind,
	base_minor, bonus_minor, equity_notes, vesting_notes, currency, notes,
	created_at, updated_at`

// InsertCompensation inserts a compensation row and returns it. q may be
// the pool or a transaction. A CHECK violation (bad kind/currency, or the
// opportunity/application XOR unsatisfied) surfaces as ErrConflict; a
// missing opportunity_id / application_id surfaces as ErrNotFound.
func (s *Store) InsertCompensation(ctx context.Context, q Querier, p CompensationParams) (model.Compensation, error) {
	const query = `
		INSERT INTO compensations (opportunity_id, application_id, kind,
			base_minor, bonus_minor, equity_notes, vesting_notes, currency, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING ` + compensationColumns

	row := q.QueryRow(
		ctx, query,
		p.OpportunityID, p.ApplicationID, p.Kind, p.BaseMinor, p.BonusMinor,
		p.EquityNotes, p.VestingNotes, p.Currency, p.Notes,
	)
	c, err := scanCompensation(row)
	if err != nil {
		return model.Compensation{}, translateCompensationErr("create", err)
	}
	return c, nil
}

// GetCompensation returns the compensation by id, or ErrNotFound.
func (s *Store) GetCompensation(ctx context.Context, id string) (model.Compensation, error) {
	const q = `SELECT ` + compensationColumns + `
		FROM compensations
		WHERE id = $1`
	c, err := scanCompensation(s.Pool.QueryRow(ctx, q, id))
	if err != nil {
		return model.Compensation{}, translateCompensationErr("get", err)
	}
	return c, nil
}

// ListOpportunityCompensations returns the compensation rows linked to an
// opportunity (target/listed figures), oldest first with the id as a
// tie-breaker for a stable order. An opportunity with no compensations (or
// one that doesn't exist) returns an empty slice, no error — same contract
// as the other List* helpers.
func (s *Store) ListOpportunityCompensations(ctx context.Context, oppID string) ([]model.Compensation, error) {
	const q = `SELECT ` + compensationColumns + `
		FROM compensations
		WHERE opportunity_id = $1
		ORDER BY created_at, id`
	return s.listCompensations(ctx, "list opportunity compensations", q, oppID)
}

// ListApplicationCompensations returns the compensation rows linked to an
// application (offered/counter/accepted figures), oldest first with the id
// as a tie-breaker. Empty slice for an application with none.
func (s *Store) ListApplicationCompensations(ctx context.Context, appID string) ([]model.Compensation, error) {
	const q = `SELECT ` + compensationColumns + `
		FROM compensations
		WHERE application_id = $1
		ORDER BY created_at, id`
	return s.listCompensations(ctx, "list application compensations", q, appID)
}

// listCompensations runs a single-parameter SELECT and scans every row.
// Both ListOpportunityCompensations and ListApplicationCompensations share
// it — they differ only in the WHERE column.
func (s *Store) listCompensations(ctx context.Context, op, query, id string) ([]model.Compensation, error) {
	rows, err := s.Pool.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("store: %s: %w", op, err)
	}
	defer rows.Close()

	var out []model.Compensation
	for rows.Next() {
		c, err := scanCompensation(rows)
		if err != nil {
			return nil, fmt.Errorf("store: %s: %w", op, err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: %s: %w", op, err)
	}
	return out, nil
}

// UpdateCompensation overwrites the editable fields of id and bumps
// updated_at. Returns the post-update row. Missing id is ErrNotFound; a
// CHECK violation surfaces as ErrConflict.
func (s *Store) UpdateCompensation(ctx context.Context, id string, p CompensationParams) (model.Compensation, error) {
	const q = `
		UPDATE compensations
		SET opportunity_id = $2,
			application_id = $3,
			kind = $4,
			base_minor = $5,
			bonus_minor = $6,
			equity_notes = $7,
			vesting_notes = $8,
			currency = $9,
			notes = $10,
			updated_at = now()
		WHERE id = $1
		RETURNING ` + compensationColumns

	row := s.Pool.QueryRow(
		ctx, q, id,
		p.OpportunityID, p.ApplicationID, p.Kind, p.BaseMinor, p.BonusMinor,
		p.EquityNotes, p.VestingNotes, p.Currency, p.Notes,
	)
	c, err := scanCompensation(row)
	if err != nil {
		return model.Compensation{}, translateCompensationErr("update", err)
	}
	return c, nil
}

// DeleteCompensation removes the compensation by id. Missing id is
// ErrNotFound.
func (s *Store) DeleteCompensation(ctx context.Context, id string) error {
	const q = `DELETE FROM compensations WHERE id = $1`
	tag, err := s.Pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("store: delete compensation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanCompensation(r rowScanner) (model.Compensation, error) {
	var c model.Compensation
	err := r.Scan(
		&c.ID, &c.OpportunityID, &c.ApplicationID, &c.Kind, &c.BaseMinor,
		&c.BonusMinor, &c.EquityNotes, &c.VestingNotes, &c.Currency, &c.Notes,
		&c.CreatedAt, &c.UpdatedAt,
	)
	return c, err
}

func translateCompensationErr(op string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if pg, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch pg.Code {
		case pgForeignKeyViolation:
			// opportunity_id / application_id pointed at a row that doesn't
			// exist. Name the entity so the caller isn't left guessing.
			return fmt.Errorf("%w: unknown opportunity or application ID", ErrNotFound)
		case pgCheckViolation:
			// compensations_kind_chk / compensations_currency_chk /
			// compensations_target_xor_chk: a bad kind, currency, or
			// opportunity/application linkage. Surface as a conflict
			// instead of leaking the raw driver error.
			return fmt.Errorf("%w: invalid compensation (%s)", ErrConflict, pg.ConstraintName)
		}
	}
	return fmt.Errorf("store: %s compensation: %w", op, err)
}
