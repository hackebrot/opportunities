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

// EventParams is the writable subset of model.Event. Server-managed
// columns (id, created_at) are not part of it. ApplicationID, StageID, and
// Label are nullable; Label must be non-nil exactly when Kind is "custom"
// (enforced by a table CHECK).
//
// The full events engine (status transitions, the contextual kind menu)
// lands in a later task; this insert covers the kinds the opportunity
// create flow needs (notably "added").
type EventParams struct {
	OpportunityID string
	ApplicationID *string
	StageID       *string
	Kind          string
	OccurredAt    time.Time
	Label         *string
	Notes         string
}

const eventColumns = `id, opportunity_id, application_id, stage_id, kind,
	occurred_at, label, notes, created_at`

// InsertEvent inserts a timeline event and returns the persisted row. q
// may be the pool or a transaction. A composite application/opportunity FK
// or kind/label CHECK violation surfaces as ErrConflict; a missing
// opportunity_id or stage_id reference surfaces as ErrNotFound.
func (s *Store) InsertEvent(ctx context.Context, q Querier, p EventParams) (model.Event, error) {
	const query = `
		INSERT INTO events (opportunity_id, application_id, stage_id, kind,
			occurred_at, label, notes)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING ` + eventColumns

	row := q.QueryRow(
		ctx, query,
		p.OpportunityID, p.ApplicationID, p.StageID, p.Kind,
		p.OccurredAt, p.Label, p.Notes,
	)
	e, err := scanEvent(row)
	if err != nil {
		return model.Event{}, translateEventErr("create", err)
	}
	return e, nil
}

func scanEvent(r rowScanner) (model.Event, error) {
	var e model.Event
	err := r.Scan(
		&e.ID, &e.OpportunityID, &e.ApplicationID, &e.StageID, &e.Kind,
		&e.OccurredAt, &e.Label, &e.Notes, &e.CreatedAt,
	)
	return e, err
}

func translateEventErr(op string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case pgForeignKeyViolation:
			// With application_id set, the composite FK requires a matching
			// (opportunity_id, application_id) application row. It fires both
			// when no such application exists and when the application belongs
			// to a different opportunity; the two are indistinguishable by
			// constraint name. Either way the opportunity_id is valid, so this
			// is a relationship conflict. The other FKs (opportunity_id,
			// stage_id) fire when the referenced row is simply absent.
			if pg.ConstraintName == "events_application_belongs_to_opportunity_fk" {
				return fmt.Errorf("%w: event application is missing or belongs to another opportunity", ErrConflict)
			}
			return fmt.Errorf("%w: event references a missing opportunity or stage", ErrNotFound)
		case pgCheckViolation:
			return fmt.Errorf("%w: invalid event (%s)", ErrConflict, pg.ConstraintName)
		}
	}
	return fmt.Errorf("store: %s event: %w", op, err)
}
