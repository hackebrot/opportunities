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

// AddOpportunity validates the input, then writes the opportunity (with
// latest_status "watching") and its initial "added" event in one
// transaction. Either both land or neither does. Returns the persisted
// opportunity with its company name resolved.
func (s *Service) AddOpportunity(ctx context.Context, in OpportunityInput) (model.Opportunity, error) {
	const op = "service.AddOpportunity"

	params, err := in.normalize()
	if err != nil {
		return model.Opportunity{}, err
	}

	tx, err := s.store.Begin(ctx)
	if err != nil {
		return model.Opportunity{}, fmt.Errorf("%s: %w", op, err)
	}
	// Safety net for the error paths below; a no-op once Commit succeeds.
	defer func() { _ = tx.Rollback(ctx) }()

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

	if err := tx.Commit(ctx); err != nil {
		return model.Opportunity{}, fmt.Errorf("%s: commit: %w", op, err)
	}
	return opp, nil
}
