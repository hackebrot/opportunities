package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/hackebrot/opportunities/internal/model"
	"github.com/hackebrot/opportunities/internal/store"
)

// validSources is the source taxonomy from the spec. Ranking weights per
// source live in config; here we only validate membership.
var validSources = map[string]bool{
	"outbound":                   true,
	"inbound_inhouse_recruiter":  true,
	"inbound_external_recruiter": true,
	"inbound_founder":            true,
	"inbound_employee":           true,
	"referral":                   true,
	"network":                    true,
	"other":                      true,
}

var validPriorities = map[string]bool{
	"low":    true,
	"normal": true,
	"high":   true,
}

// validRelationships mirrors the opportunity_contacts.relationship CHECK
// constraint. The service rejects unknown values up front so the caller
// gets a typed ErrValidation instead of a generic CHECK violation.
var validRelationships = map[string]bool{
	"recruiter":      true,
	"hiring_manager": true,
	"referrer":       true,
	"interviewer":    true,
	"other":          true,
}

// OpportunityInput is the caller-supplied subset of an opportunity. ID,
// timestamps, and the status-machine fields (latest_status, archived_at)
// are owned by the service/store. RoleTitle is optional (blank → null).
type OpportunityInput struct {
	CompanyID         string
	RoleTitle         string
	Location          string
	OfficeDaysPerWeek int
	Source            string
	SourceDetail      string
	Priority          string
	Notes             string
}

// normalize validates the input and maps it to store.OpportunityParams.
// Priority defaults to "normal" when blank. Returns ErrValidation on a
// missing company id, unknown source/priority, blank role-title nilling,
// or office days outside 0..5.
func (in OpportunityInput) normalize() (store.OpportunityParams, error) {
	if strings.TrimSpace(in.CompanyID) == "" {
		return store.OpportunityParams{}, fmt.Errorf("%w: company is required", ErrValidation)
	}
	if !validSources[in.Source] {
		return store.OpportunityParams{}, fmt.Errorf("%w: unknown source %q", ErrValidation, in.Source)
	}
	priority := in.Priority
	if priority == "" {
		priority = "normal"
	}
	if !validPriorities[priority] {
		return store.OpportunityParams{}, fmt.Errorf("%w: unknown priority %q", ErrValidation, in.Priority)
	}
	if in.OfficeDaysPerWeek < 0 || in.OfficeDaysPerWeek > 5 {
		return store.OpportunityParams{}, fmt.Errorf("%w: office days per week must be 0..5, got %d", ErrValidation, in.OfficeDaysPerWeek)
	}

	return store.OpportunityParams{
		CompanyID:         strings.TrimSpace(in.CompanyID),
		RoleTitle:         nullableString(strings.TrimSpace(in.RoleTitle)),
		Location:          in.Location,
		OfficeDaysPerWeek: in.OfficeDaysPerWeek,
		Source:            in.Source,
		SourceDetail:      in.SourceDetail,
		Priority:          priority,
		Notes:             in.Notes,
	}, nil
}

// OpportunityCompanyChoice selects the company an opportunity is being
// created under. Exactly one of ID or New must be set: ID picks an
// existing company, New inserts a brand-new company in the same tx so
// the entire opportunity graph (company + opportunity + initial event +
// optional contact + opportunity_contacts row) commits atomically.
type OpportunityCompanyChoice struct {
	ID  string
	New *CompanyInput
}

// OpportunityContactChoice attaches a contact to the new opportunity.
// Exactly one of ID or New must be set; New auto-receives the resolved
// company id (so the inline contact lands under the same company as the
// opportunity, matching the "founder reach-out" / "recruiter messaged me"
// flows). Relationship must be one of validRelationships.
type OpportunityContactChoice struct {
	ID           string
	New          *ContactInput
	Relationship string
}

// OpportunityCreationInput bundles the full input for AddOpportunity.
// The opportunity's CompanyID is taken from Company (not
// Opportunity.CompanyID — the latter is overwritten so a caller doesn't
// have to keep the two in sync).
type OpportunityCreationInput struct {
	Company     OpportunityCompanyChoice
	Opportunity OpportunityInput
	Contact     *OpportunityContactChoice
}

// normalize trims whitespace from the existing-entity IDs and enforces
// the structural contract: exactly one of {ID, New} on each choice, and
// a known relationship if a contact is present. The opportunity body's
// own validation (source, priority, office days) is deferred to
// OpportunityInput.normalize on the tx path.
//
// Returns the cleaned input alongside any validation error so the
// caller (AddOpportunity) can use the trimmed IDs directly without
// re-trimming.
func (in OpportunityCreationInput) normalize() (OpportunityCreationInput, error) {
	in.Company.ID = strings.TrimSpace(in.Company.ID)
	switch {
	case in.Company.ID == "" && in.Company.New == nil:
		return in, fmt.Errorf("%w: a company (existing id or new) is required", ErrValidation)
	case in.Company.ID != "" && in.Company.New != nil:
		return in, fmt.Errorf("%w: pick an existing company or create a new one, not both", ErrValidation)
	}
	if in.Contact == nil {
		return in, nil
	}
	if !validRelationships[in.Contact.Relationship] {
		return in, fmt.Errorf("%w: unknown relationship %q", ErrValidation, in.Contact.Relationship)
	}
	// Copy through the pointer so trimming the contact ID doesn't reach
	// back into the caller's struct.
	contact := *in.Contact
	contact.ID = strings.TrimSpace(contact.ID)
	in.Contact = &contact
	switch {
	case contact.ID == "" && contact.New == nil:
		return in, fmt.Errorf("%w: a contact (existing id or new) is required when Contact is set", ErrValidation)
	case contact.ID != "" && contact.New != nil:
		return in, fmt.Errorf("%w: pick an existing contact or create a new one, not both", ErrValidation)
	}
	return in, nil
}

// GetOpportunity returns an opportunity by id.
func (s *Service) GetOpportunity(ctx context.Context, id string) (model.Opportunity, error) {
	return s.store.GetOpportunity(ctx, id)
}

// ListOpportunities returns all opportunities, most-recently-created
// first.
func (s *Service) ListOpportunities(ctx context.Context) ([]model.Opportunity, error) {
	return s.store.ListOpportunities(ctx)
}

// UpdateOpportunity validates the input, then overwrites the editable
// fields of id. Owner-of-status fields (latest_status, archived_at) are
// left untouched — they belong to the events engine.
func (s *Service) UpdateOpportunity(ctx context.Context, id string, in OpportunityInput) (model.Opportunity, error) {
	params, err := in.normalize()
	if err != nil {
		return model.Opportunity{}, err
	}
	return s.store.UpdateOpportunity(ctx, id, params)
}

// DeleteOpportunity removes an opportunity by id.
func (s *Service) DeleteOpportunity(ctx context.Context, id string) error {
	return s.store.DeleteOpportunity(ctx, id)
}

// AddOpportunity writes the full opportunity graph in a single
// transaction: optional new company, opportunity row (latest_status
// "watching"), the initial "added" event, and optionally a contact
// (existing or new) attached with the given relationship. Either every
// row lands or none does — a failure midway through rolls the whole
// flow back, so the user is never left with an orphan company or
// contact.
func (s *Service) AddOpportunity(ctx context.Context, in OpportunityCreationInput) (model.Opportunity, error) {
	const op = "service.AddOpportunity"

	in, err := in.normalize()
	if err != nil {
		return model.Opportunity{}, err
	}

	tx, err := s.store.Begin(ctx)
	if err != nil {
		return model.Opportunity{}, fmt.Errorf("%s: %w", op, err)
	}
	// Safety net for the error paths below; a no-op once Commit succeeds.
	defer func() { _ = tx.Rollback(ctx) }()

	companyID := in.Company.ID
	if in.Company.New != nil {
		c, err := s.createCompany(ctx, tx, *in.Company.New)
		if err != nil {
			return model.Opportunity{}, err
		}
		companyID = c.ID
	}

	oppIn := in.Opportunity
	oppIn.CompanyID = companyID
	params, err := oppIn.normalize()
	if err != nil {
		return model.Opportunity{}, err
	}

	opp, err := s.store.InsertOpportunity(ctx, tx, params, "watching")
	if err != nil {
		return model.Opportunity{}, fmt.Errorf("%s: %w", op, err)
	}

	if _, err := s.store.InsertEvent(ctx, tx, store.EventParams{
		OpportunityID: opp.ID,
		Kind:          "added",
		OccurredAt:    s.clock.Now(),
	}); err != nil {
		return model.Opportunity{}, fmt.Errorf("%s: %w", op, err)
	}

	if in.Contact != nil {
		contactID := in.Contact.ID
		if in.Contact.New != nil {
			// Inline contacts default to the resolved company: when a
			// contact is created in the same flow as the opportunity, it
			// almost always belongs to the same company. A flag-driven
			// caller can still pass a different CompanyID on the contact
			// input to override.
			newContact := *in.Contact.New
			if newContact.CompanyID == nil {
				newContact.CompanyID = &companyID
			}
			c, err := s.createContact(ctx, tx, newContact)
			if err != nil {
				return model.Opportunity{}, err
			}
			contactID = c.ID
		}
		if err := s.store.AttachOpportunityContact(ctx, tx, opp.ID, contactID, in.Contact.Relationship); err != nil {
			return model.Opportunity{}, fmt.Errorf("%s: %w", op, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return model.Opportunity{}, fmt.Errorf("%s: commit: %w", op, err)
	}
	return opp, nil
}
