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

// ApplicationStageParams is the writable subset of model.ApplicationStage
// for InsertApplicationStage / UpdateApplicationStage. Server-managed
// columns (id, created_at, updated_at) are not part of it. Outcome, when
// non-nil, must be one of 'passed'/'failed'/'skipped' (enforced by
// application_stages_outcome_chk, surfacing as ErrConflict).
type ApplicationStageParams struct {
	ApplicationID    string
	OrderIndex       int
	Label            string
	ExpectationNotes string
	PrepNotes        string
	ScheduledAt      *time.Time
	CompletedAt      *time.Time
	Outcome          *string
}

// applicationStageColumns lists the application_stages table columns in the
// order scanApplicationStage expects.
const applicationStageColumns = `id, application_id, order_index, label,
	expectation_notes, prep_notes, scheduled_at, completed_at, outcome,
	created_at, updated_at`

// InsertApplicationStage inserts a stage row and returns it. q may be the
// pool or a transaction. A duplicate (application_id, order_index) surfaces
// as ErrConflict; a missing application_id as ErrNotFound; a bad outcome as
// ErrConflict.
func (s *Store) InsertApplicationStage(ctx context.Context, q Querier, p ApplicationStageParams) (model.ApplicationStage, error) {
	const query = `
		INSERT INTO application_stages (application_id, order_index, label,
			expectation_notes, prep_notes, scheduled_at, completed_at, outcome)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING ` + applicationStageColumns

	row := q.QueryRow(
		ctx, query,
		p.ApplicationID, p.OrderIndex, p.Label, p.ExpectationNotes,
		p.PrepNotes, p.ScheduledAt, p.CompletedAt, p.Outcome,
	)
	st, err := scanApplicationStage(row)
	if err != nil {
		return model.ApplicationStage{}, translateApplicationStageErr("create", err)
	}
	return st, nil
}

// GetApplicationStage returns the stage by id, or ErrNotFound.
func (s *Store) GetApplicationStage(ctx context.Context, id string) (model.ApplicationStage, error) {
	const q = `SELECT ` + applicationStageColumns + `
		FROM application_stages
		WHERE id = $1`
	st, err := scanApplicationStage(s.Pool.QueryRow(ctx, q, id))
	if err != nil {
		return model.ApplicationStage{}, translateApplicationStageErr("get", err)
	}
	return st, nil
}

// ListApplicationStages returns every stage of appID in ascending
// order_index — the display order of the interview pipeline. order_index
// is UNIQUE per application, so the ordering is deterministic without a
// secondary sort key (unlike the compensation lists, which tie-break on
// id). An application with no stages (or one that doesn't exist) returns
// an empty slice, no error — same contract as the other List* helpers.
func (s *Store) ListApplicationStages(ctx context.Context, appID string) ([]model.ApplicationStage, error) {
	const q = `SELECT ` + applicationStageColumns + `
		FROM application_stages
		WHERE application_id = $1
		ORDER BY order_index`
	rows, err := s.Pool.Query(ctx, q, appID)
	if err != nil {
		return nil, fmt.Errorf("store: list application stages: %w", err)
	}
	defer rows.Close()

	var out []model.ApplicationStage
	for rows.Next() {
		st, err := scanApplicationStage(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list application stages: %w", err)
		}
		out = append(out, st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list application stages: %w", err)
	}
	return out, nil
}

// UpdateApplicationStage overwrites the editable fields of id and bumps
// updated_at. Returns the post-update row. Missing id is ErrNotFound; a
// duplicate order_index or bad outcome surfaces as ErrConflict.
func (s *Store) UpdateApplicationStage(ctx context.Context, id string, p ApplicationStageParams) (model.ApplicationStage, error) {
	const q = `
		UPDATE application_stages
		SET application_id = $2,
			order_index = $3,
			label = $4,
			expectation_notes = $5,
			prep_notes = $6,
			scheduled_at = $7,
			completed_at = $8,
			outcome = $9,
			updated_at = now()
		WHERE id = $1
		RETURNING ` + applicationStageColumns

	row := s.Pool.QueryRow(
		ctx, q, id,
		p.ApplicationID, p.OrderIndex, p.Label, p.ExpectationNotes,
		p.PrepNotes, p.ScheduledAt, p.CompletedAt, p.Outcome,
	)
	st, err := scanApplicationStage(row)
	if err != nil {
		return model.ApplicationStage{}, translateApplicationStageErr("update", err)
	}
	return st, nil
}

// DeleteApplicationStage removes the stage by id. Missing id is
// ErrNotFound.
func (s *Store) DeleteApplicationStage(ctx context.Context, id string) error {
	const q = `DELETE FROM application_stages WHERE id = $1`
	tag, err := s.Pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("store: delete application stage: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanApplicationStage(r rowScanner) (model.ApplicationStage, error) {
	var st model.ApplicationStage
	err := r.Scan(
		&st.ID, &st.ApplicationID, &st.OrderIndex, &st.Label,
		&st.ExpectationNotes, &st.PrepNotes, &st.ScheduledAt, &st.CompletedAt,
		&st.Outcome, &st.CreatedAt, &st.UpdatedAt,
	)
	return st, err
}

func translateApplicationStageErr(op string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if pg, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch pg.Code {
		case pgUniqueViolation:
			// UNIQUE (application_id, order_index): two stages can't share a
			// position in the same pipeline.
			return fmt.Errorf("%w: order_index already used for this application", ErrConflict)
		case pgForeignKeyViolation:
			// Moving a stage to another application breaks the
			// (application_id, stage_id) link of any event that references
			// it; the destination application itself exists.
			if pg.ConstraintName == "events_stage_belongs_to_app_fk" {
				return fmt.Errorf("%w: stage is referenced by events on its current application", ErrConflict)
			}
			// application_id pointed at an application that doesn't exist.
			return fmt.Errorf("%w: unknown application ID", ErrNotFound)
		case pgCheckViolation:
			// application_stages_outcome_chk: a bad outcome value.
			return fmt.Errorf("%w: invalid stage (%s)", ErrConflict, pg.ConstraintName)
		}
	}
	return fmt.Errorf("store: %s application stage: %w", op, err)
}
