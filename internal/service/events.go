package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/hackebrot/opportunities/internal/model"
	"github.com/hackebrot/opportunities/internal/store"
)

// EventInput is the caller-supplied subset of an event. Only the
// opportunity-only kinds are accepted today; application-tied kinds
// (applied, screen, …) will extend this struct when applications land.
//
// Label is required for Kind == "custom" and forbidden otherwise (matches
// the events_label_only_for_custom_chk constraint). Notes is free-form;
// for archived it doubles as the opportunity-level archive reason
// (declined leaves archive_reason untouched).
type EventInput struct {
	OpportunityID string
	Kind          string
	Label         string
	Notes         string
}

// opportunityOnlyEventKinds is the set of event kinds AppendEvent
// currently accepts: those whose preconditions and side effects do not
// depend on an application. Application-tied kinds (applied, screen,
// offer, accepted, rejected, withdrawn, …) are rejected with
// ErrValidation until the applications layer wires them in.
var opportunityOnlyEventKinds = map[string]bool{
	"exploring": true,
	"archived":  true,
	"note":      true,
	"follow_up": true,
	"custom":    true,
	"declined":  true,
}

// statusInputs is the data computeLatestStatus reads to derive an
// opportunity's latest_status. Loaded from the DB by the wider events
// engine; kept as a service-local struct so the rule can be
// exhaustively table-tested without standing up a database.
type statusInputs struct {
	archived           bool
	activeAppStatus    string
	latestAppStatus    string
	anyApp             bool
	anyNonPassiveEvent bool
}

// computeLatestStatus derives an opportunity's latest_status from its
// current state. Rules are evaluated top-down; the first match wins:
//
//  1. archived_at set → archived.
//  2. active application (applied/in_progress/offer) → mirror its status.
//  3. latest application is accepted → accepted.
//  4. any application exists but none active → dormant.
//  5. no applications, any non-passive event → exploring.
//  6. otherwise → watching.
//
// Passive event kinds (added, note, follow_up, custom, archived,
// reopened) carry no progression signal and so do not trigger rule 5.
func computeLatestStatus(in statusInputs) string {
	switch {
	case in.archived:
		return "archived"
	case in.activeAppStatus != "":
		return in.activeAppStatus
	case in.latestAppStatus == "accepted":
		return "accepted"
	case in.anyApp:
		return "dormant"
	case in.anyNonPassiveEvent:
		return "exploring"
	default:
		return "watching"
	}
}

// RecomputeLatestStatus reads the current state of the opportunity from
// q (pool or tx), applies computeLatestStatus, and writes the result back
// via store.SetLatestStatus. Returns the freshly written status. Called
// by AppendEvent at the end of every event-handling tx; application-tied
// flows will reuse it once they exist.
func (s *Service) RecomputeLatestStatus(ctx context.Context, q store.Querier, oppID string) (string, error) {
	const op = "service.RecomputeLatestStatus"
	raw, err := s.store.LoadOpportunityStatusInputs(ctx, q, oppID)
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}
	next := computeLatestStatus(statusInputs{
		archived:           raw.Archived,
		activeAppStatus:    raw.ActiveAppStatus,
		latestAppStatus:    raw.LatestAppStatus,
		anyApp:             raw.AnyApp,
		anyNonPassiveEvent: raw.AnyNonPassiveEvent,
	})
	if err := s.store.SetLatestStatus(ctx, q, oppID, next); err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}
	return next, nil
}

// AppendEvent validates an event against the opportunity-only contract,
// then writes the event, applies opportunity-level side effects
// (archived stamps archived_at + archive_reason; declined stamps
// archived_at only), and recomputes latest_status — all in one
// transaction. Either every change lands or none do.
//
// Returns ErrValidation for malformed input, ErrPrecondition when the
// event is well-formed but invalid for the opportunity's current state
// (e.g. exploring once applications exist; archive of an already
// archived opportunity), store.ErrNotFound for an unknown opportunity.
func (s *Service) AppendEvent(ctx context.Context, in EventInput) (model.Event, error) {
	const op = "service.AppendEvent"

	oppID := strings.TrimSpace(in.OpportunityID)
	if oppID == "" {
		return model.Event{}, fmt.Errorf("%w: opportunity is required", ErrValidation)
	}
	if !opportunityOnlyEventKinds[in.Kind] {
		return model.Event{}, fmt.Errorf("%w: kind %q is not yet supported", ErrValidation, in.Kind)
	}

	label, err := normalizeEventLabel(in.Kind, in.Label)
	if err != nil {
		return model.Event{}, err
	}

	tx, err := s.store.Begin(ctx)
	if err != nil {
		return model.Event{}, fmt.Errorf("%s: %w", op, err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	state, err := s.store.LoadOpportunityStatusInputs(ctx, tx, oppID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return model.Event{}, err
		}
		return model.Event{}, fmt.Errorf("%s: %w", op, err)
	}

	if err := checkEventPrecondition(in.Kind, state); err != nil {
		return model.Event{}, err
	}

	now := s.clock.Now()

	switch in.Kind {
	case "archived":
		// archive_reason mirrors the event's notes so a caller reading
		// opportunities directly doesn't need to JOIN events.
		if err := s.store.SetOpportunityArchived(ctx, tx, oppID, now, nullableString(in.Notes)); err != nil {
			return model.Event{}, fmt.Errorf("%s: %w", op, err)
		}
	case "declined":
		// declined-without-app: only archived_at is required;
		// archive_reason stays untouched (NULL on a fresh opportunity).
		if err := s.store.SetOpportunityArchived(ctx, tx, oppID, now, nil); err != nil {
			return model.Event{}, fmt.Errorf("%s: %w", op, err)
		}
	}

	ev, err := s.store.InsertEvent(ctx, tx, store.EventParams{
		OpportunityID: oppID,
		Kind:          in.Kind,
		OccurredAt:    now,
		Label:         label,
		Notes:         in.Notes,
	})
	if err != nil {
		return model.Event{}, fmt.Errorf("%s: %w", op, err)
	}

	if _, err := s.RecomputeLatestStatus(ctx, tx, oppID); err != nil {
		return model.Event{}, fmt.Errorf("%s: %w", op, err)
	}

	if err := tx.Commit(ctx); err != nil {
		return model.Event{}, fmt.Errorf("%s: commit: %w", op, err)
	}
	return ev, nil
}

// ArchiveOpportunity is the convenience entry point for the
// `opps opportunity archive` flow: append an `archived` event whose
// notes double as the free-text archive reason.
func (s *Service) ArchiveOpportunity(ctx context.Context, oppID, reason string) (model.Event, error) {
	return s.AppendEvent(ctx, EventInput{
		OpportunityID: oppID,
		Kind:          "archived",
		Notes:         reason,
	})
}

func normalizeEventLabel(kind, label string) (*string, error) {
	trimmed := strings.TrimSpace(label)
	if kind == "custom" {
		if trimmed == "" {
			return nil, fmt.Errorf("%w: label is required for custom events", ErrValidation)
		}
		return &trimmed, nil
	}
	if trimmed != "" {
		return nil, fmt.Errorf("%w: label is only valid for custom events", ErrValidation)
	}
	return nil, nil
}

// checkEventPrecondition enforces the "required state" for the
// opportunity-only kinds before any write hits the DB.
func checkEventPrecondition(kind string, state store.OpportunityStatusInputs) error {
	switch kind {
	case "exploring":
		if state.AnyApp {
			return fmt.Errorf("%w: cannot mark exploring once an application exists", ErrPrecondition)
		}
		if state.Archived {
			return fmt.Errorf("%w: opportunity is archived", ErrPrecondition)
		}
	case "archived":
		// Re-archive is rejected to prevent silently overwriting an
		// existing archived_at + archive_reason. Unarchiving belongs to
		// a dedicated flow, not a second archive call.
		if state.Archived {
			return fmt.Errorf("%w: opportunity is already archived", ErrPrecondition)
		}
	case "declined":
		// Only the no-application branch is implemented; declining an
		// existing application goes through the applications layer. The
		// rejection is state-based (state.AnyApp), so it surfaces as
		// ErrPrecondition like the other state-machine rejections.
		if state.AnyApp {
			return fmt.Errorf("%w: declined with an application is not yet supported", ErrPrecondition)
		}
		if state.Archived {
			return fmt.Errorf("%w: opportunity is already archived", ErrPrecondition)
		}
	case "note", "follow_up", "custom":
		// Informational; no state-machine precondition.
	}
	return nil
}
