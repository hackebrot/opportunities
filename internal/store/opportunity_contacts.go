package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

// AttachOpportunityContact links contactID to oppID with the given
// relationship. q may be the pool or a transaction. Duplicate attach
// (same opp, contact, relationship) surfaces as ErrConflict; an unknown
// opp or contact id surfaces as ErrNotFound.
//
// The minimal attach is all this task needs — the full attach/detach/list
// CRUD lands with the secondary `opportunity contact` CLI.
func (s *Store) AttachOpportunityContact(ctx context.Context, q Querier, oppID, contactID, relationship string) error {
	const query = `
		INSERT INTO opportunity_contacts (opportunity_id, contact_id, relationship)
		VALUES ($1, $2, $3)`
	if _, err := q.Exec(ctx, query, oppID, contactID, relationship); err != nil {
		return translateOpportunityContactErr("attach", err)
	}
	return nil
}

func translateOpportunityContactErr(op string, err error) error {
	if pg, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch pg.Code {
		case pgUniqueViolation:
			return fmt.Errorf("%w: contact already attached with this relationship", ErrConflict)
		case pgForeignKeyViolation:
			return fmt.Errorf("%w: opportunity or contact does not exist", ErrNotFound)
		case pgCheckViolation:
			// Defense-in-depth: the service rejects unknown relationships
			// via validRelationships before reaching the store. A CHECK
			// violation here means the schema enum and the service enum
			// have drifted; ErrConflict is the closest existing sentinel.
			return fmt.Errorf("%w: invalid relationship", ErrConflict)
		}
	}
	return fmt.Errorf("store: %s opportunity contact: %w", op, err)
}
