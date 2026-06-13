package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hackebrot/opportunities/internal/model"
	"github.com/hackebrot/opportunities/internal/store"
)

// ApplicationInput is the caller-supplied subset of an application. The
// status-machine fields (status, archived_at, archive_reason*) are
// written by the events engine, not by callers; id and timestamps are
// server-managed. AppliedAt is optional — an application can be
// captured before the exact submission time is known.
type ApplicationInput struct {
	OpportunityID    string
	AppliedAt        *time.Time
	AppliedWithEmail string
	Notes            string
}

// normalize trims the opportunity id and validates the input. The
// applications table has no further "well-formed" checks at this layer
// today — applied_with_email is just a free-text override on the
// applicant's reply-to, intentionally not constrained to RFC 5322 (the
// user may want to record a private inbox or a forwarding alias).
func (in ApplicationInput) normalize() (store.ApplicationParams, error) {
	oppID := strings.TrimSpace(in.OpportunityID)
	if oppID == "" {
		return store.ApplicationParams{}, fmt.Errorf("%w: opportunity is required", ErrValidation)
	}
	return store.ApplicationParams{
		OpportunityID:    oppID,
		AppliedAt:        in.AppliedAt,
		AppliedWithEmail: in.AppliedWithEmail,
		Notes:            in.Notes,
	}, nil
}

// AddApplication writes a new application against an existing
// opportunity, in one transaction: insert the application row (status
// "applied"), emit the matching `applied` event, recompute the
// opportunity's latest_status. Either every row lands or none do.
//
// Returns ErrValidation for malformed input, ErrPrecondition when the
// opportunity is already archived, store.ErrNotFound for an unknown
// opportunity, and store.ErrActiveExists when an active application
// (applied/in_progress/offer) already exists for the same opportunity —
// the partial unique index, not this code path, is the authority on
// that conflict, which means concurrent callers reliably collapse to
// exactly one winner.
func (s *Service) AddApplication(ctx context.Context, in ApplicationInput) (model.Application, error) {
	const op = "service.AddApplication"

	params, err := in.normalize()
	if err != nil {
		return model.Application{}, err
	}

	tx, err := s.store.Begin(ctx)
	if err != nil {
		return model.Application{}, fmt.Errorf("%s: %w", op, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Load + lock the opportunity row so the archived check below is
	// consistent for the duration of the tx; the partial unique index
	// remains the authority on the "no second active app" rule.
	state, err := s.store.LoadOpportunityStatusInputs(ctx, tx, params.OpportunityID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return model.Application{}, err
		}
		return model.Application{}, fmt.Errorf("%s: %w", op, err)
	}
	if state.Archived {
		return model.Application{}, fmt.Errorf("%w: opportunity is archived", ErrPrecondition)
	}

	app, err := s.store.InsertApplication(ctx, tx, params, "applied")
	if err != nil {
		if errors.Is(err, store.ErrActiveExists) || errors.Is(err, store.ErrNotFound) {
			return model.Application{}, err
		}
		return model.Application{}, fmt.Errorf("%s: %w", op, err)
	}

	appID := app.ID
	if _, err := s.store.InsertEvent(ctx, tx, store.EventParams{
		OpportunityID: params.OpportunityID,
		ApplicationID: &appID,
		Kind:          "applied",
		OccurredAt:    s.clock.Now(),
	}); err != nil {
		return model.Application{}, fmt.Errorf("%s: %w", op, err)
	}

	if _, err := s.RecomputeLatestStatus(ctx, tx, params.OpportunityID); err != nil {
		return model.Application{}, fmt.Errorf("%s: %w", op, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return model.Application{}, fmt.Errorf("%s: commit: %w", op, err)
	}
	return app, nil
}

// GetApplication returns an application by id.
func (s *Service) GetApplication(ctx context.Context, id string) (model.Application, error) {
	return s.store.GetApplication(ctx, id)
}

// ListApplications returns all applications, most-recently-created first.
func (s *Service) ListApplications(ctx context.Context) ([]model.Application, error) {
	return s.store.ListApplications(ctx)
}

// UpdateApplication overwrites the editable, non-status-machine fields of
// an application. Status transitions are emitted via the events engine,
// not here.
//
// OpportunityID is immutable on update: applications carry the foreign
// key into the events table (events.opportunity_id, events.application_id)
// and re-parenting would orphan every event already written against the
// original opportunity. Callers that need to "move" an application must
// delete and re-create. ErrPrecondition signals an attempted change;
// preserving the current value (no-op) is allowed.
func (s *Service) UpdateApplication(ctx context.Context, id string, in ApplicationInput) (model.Application, error) {
	params, err := in.normalize()
	if err != nil {
		return model.Application{}, err
	}
	current, err := s.store.GetApplication(ctx, id)
	if err != nil {
		return model.Application{}, err
	}
	if params.OpportunityID != current.OpportunityID {
		return model.Application{}, fmt.Errorf("%w: application opportunity_id is immutable", ErrPrecondition)
	}
	return s.store.UpdateApplication(ctx, id, params)
}

// DeleteApplication removes an application by id.
func (s *Service) DeleteApplication(ctx context.Context, id string) error {
	return s.store.DeleteApplication(ctx, id)
}

// FollowUpMode is the operation FollowUpApplication should perform.
type FollowUpMode int

const (
	// FollowUpStamp marks "just pinged them": writes
	// last_followed_up_at = now and emits a follow_up event linked to
	// the application. Leaves follow_up_blocked alone.
	FollowUpStamp FollowUpMode = iota + 1

	// FollowUpBlock sets follow_up_blocked = true. The dashboard reads
	// this flag to suppress staleness alerts for the row. Does not
	// stamp the timestamp and does not emit an event — blocking is a
	// UI hint, not a contact-with-the-recruiter signal.
	FollowUpBlock

	// FollowUpDone clears follow_up_blocked, stamps
	// last_followed_up_at = now, and emits a follow_up event. Used
	// after a previously blocked application has been pinged again.
	FollowUpDone
)

// FollowUpApplication mutates the follow-up tracking columns of an
// application and (for stamp/done) emits a follow_up event linked to
// it, all in one transaction.
//
// The application's status must be active (applied/in_progress/offer);
// a terminal application has nothing to follow up on and is rejected
// with ErrPrecondition. Returns the refreshed application row.
//
// Returns ErrValidation for an unknown mode, ErrPrecondition when the
// application is not active, and store.ErrNotFound for an unknown id.
func (s *Service) FollowUpApplication(ctx context.Context, id string, mode FollowUpMode) (model.Application, error) {
	const op = "service.FollowUpApplication"

	switch mode {
	case FollowUpStamp, FollowUpBlock, FollowUpDone:
	default:
		return model.Application{}, fmt.Errorf("%w: unknown follow-up mode", ErrValidation)
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return model.Application{}, fmt.Errorf("%w: application id is required", ErrValidation)
	}

	tx, err := s.store.Begin(ctx)
	if err != nil {
		return model.Application{}, fmt.Errorf("%s: %w", op, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	app, err := s.store.GetApplicationForUpdate(ctx, tx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return model.Application{}, err
		}
		return model.Application{}, fmt.Errorf("%s: %w", op, err)
	}
	if !IsActiveAppStatus(app.Status) {
		return model.Application{}, fmt.Errorf("%w: follow-up requires an active application", ErrPrecondition)
	}

	now := s.clock.Now()

	var (
		lastFollowedUpAt *time.Time
		blocked          *bool
		emitEvent        bool
	)
	switch mode {
	case FollowUpStamp:
		lastFollowedUpAt = &now
		emitEvent = true
	case FollowUpBlock:
		blocked = new(true)
	case FollowUpDone:
		lastFollowedUpAt = &now
		blocked = new(false)
		emitEvent = true
	}

	refreshed, err := s.store.SetApplicationFollowUp(ctx, tx, app.ID, lastFollowedUpAt, blocked)
	if err != nil {
		return model.Application{}, fmt.Errorf("%s: %w", op, err)
	}

	if emitEvent {
		appID := app.ID
		if _, err := s.store.InsertEvent(ctx, tx, store.EventParams{
			OpportunityID: app.OpportunityID,
			ApplicationID: &appID,
			Kind:          "follow_up",
			OccurredAt:    now,
		}); err != nil {
			return model.Application{}, fmt.Errorf("%s: %w", op, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return model.Application{}, fmt.Errorf("%s: commit: %w", op, err)
	}
	return refreshed, nil
}
