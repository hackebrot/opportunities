package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/hackebrot/opportunities/internal/model"
)

// AttachOpportunityContact links contactID to oppID with the given
// relationship. q may be the pool or a transaction. Duplicate attach
// (same opp, contact, relationship) surfaces as ErrConflict; an unknown
// opp or contact id surfaces as ErrNotFound.
func (s *Store) AttachOpportunityContact(ctx context.Context, q Querier, oppID, contactID, relationship string) error {
	const query = `
		INSERT INTO opportunity_contacts (opportunity_id, contact_id, relationship)
		VALUES ($1, $2, $3)`
	if _, err := q.Exec(ctx, query, oppID, contactID, relationship); err != nil {
		return translateOpportunityContactErr("attach", err)
	}
	return nil
}

// DetachOpportunityContact removes the single row identified by the
// (oppID, contactID, relationship) triple. Because the PK is the triple,
// the same contact can be attached under multiple relationships and
// detached one at a time. A row that doesn't match (whether because the
// opportunity, the contact, or the relationship is unknown) surfaces as
// ErrNotFound.
func (s *Store) DetachOpportunityContact(ctx context.Context, q Querier, oppID, contactID, relationship string) error {
	const query = `
		DELETE FROM opportunity_contacts
		WHERE opportunity_id = $1 AND contact_id = $2 AND relationship = $3`
	tag, err := q.Exec(ctx, query, oppID, contactID, relationship)
	if err != nil {
		return translateOpportunityContactErr("detach", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListOpportunityContacts returns every (contact, relationship) row
// attached to oppID, with the contact name resolved for display. Ordered
// by contact name (case-insensitive) then relationship so callers can
// rely on a stable display order. An opportunity with no attachments
// (or one that doesn't exist) returns an empty slice, no error — same
// contract as the other List* helpers.
func (s *Store) ListOpportunityContacts(ctx context.Context, oppID string) ([]model.OpportunityContact, error) {
	const query = `
		SELECT oc.opportunity_id, oc.contact_id, c.name, oc.relationship, oc.created_at
		FROM opportunity_contacts oc
		JOIN contacts c ON c.id = oc.contact_id
		WHERE oc.opportunity_id = $1
		ORDER BY lower(c.name), oc.relationship`
	rows, err := s.Pool.Query(ctx, query, oppID)
	if err != nil {
		return nil, fmt.Errorf("store: list opportunity contacts: %w", err)
	}
	defer rows.Close()

	var out []model.OpportunityContact
	for rows.Next() {
		var r model.OpportunityContact
		if err := rows.Scan(&r.OpportunityID, &r.ContactID, &r.ContactName, &r.Relationship, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: list opportunity contacts: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list opportunity contacts: %w", err)
	}
	return out, nil
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
