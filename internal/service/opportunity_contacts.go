package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/hackebrot/opportunities/internal/model"
)

// AttachOpportunityContact links contactID to oppID with the given
// relationship. Relationship is validated up front (ErrValidation on a
// blank or unknown value); a missing opportunity or contact surfaces as
// store.ErrNotFound. Duplicates (same triple already present) are
// store.ErrConflict.
func (s *Service) AttachOpportunityContact(ctx context.Context, oppID, contactID, relationship string) error {
	oppID = strings.TrimSpace(oppID)
	contactID = strings.TrimSpace(contactID)
	if err := s.validateOpportunityContact(oppID, contactID, relationship); err != nil {
		return err
	}
	return s.store.AttachOpportunityContact(ctx, s.store.Pool, oppID, contactID, relationship)
}

// DetachOpportunityContact removes the single (oppID, contactID,
// relationship) row. Relationship is required because the primary key is
// the triple — the same contact can be attached under multiple
// relationships and the caller must pick which one to remove. A
// non-matching triple is store.ErrNotFound.
func (s *Service) DetachOpportunityContact(ctx context.Context, oppID, contactID, relationship string) error {
	oppID = strings.TrimSpace(oppID)
	contactID = strings.TrimSpace(contactID)
	if err := s.validateOpportunityContact(oppID, contactID, relationship); err != nil {
		return err
	}
	return s.store.DetachOpportunityContact(ctx, s.store.Pool, oppID, contactID, relationship)
}

// ListOpportunityContacts returns every (contact, relationship) row
// attached to oppID, contact name resolved for display, ordered by
// contact name then relationship.
func (s *Service) ListOpportunityContacts(ctx context.Context, oppID string) ([]model.OpportunityContact, error) {
	return s.store.ListOpportunityContacts(ctx, strings.TrimSpace(oppID))
}

// validateOpportunityContact checks the structural contract shared by
// attach and detach. Kept lowercase so the unit test can reach it
// without forcing an exported surface.
func (s *Service) validateOpportunityContact(oppID, contactID, relationship string) error {
	if oppID == "" {
		return fmt.Errorf("%w: opportunity id is required", ErrValidation)
	}
	if contactID == "" {
		return fmt.Errorf("%w: contact id is required", ErrValidation)
	}
	if !validRelationships[relationship] {
		return fmt.Errorf("%w: unknown relationship %q", ErrValidation, relationship)
	}
	return nil
}
