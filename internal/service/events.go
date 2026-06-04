package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/hackebrot/opportunities/internal/model"
	"github.com/hackebrot/opportunities/internal/store"
)

// EventInput is the caller-supplied subset of an event.
//
// Label is required for Kind == "custom" and forbidden otherwise (matches
// the events_label_only_for_custom_chk constraint). Notes is free-form;
// for archived it doubles as the opportunity-level archive reason
// (declined-without-app leaves archive_reason untouched).
//
// ArchiveReasonCategory is required for the terminal application kinds
// rejected / declined-with-app / withdrawn (the spec mandates a
// category for the report's bucketing) and forbidden otherwise. The
// permitted values depend on the kind — see applicationTransitions.
type EventInput struct {
	OpportunityID         string
	Kind                  string
	Label                 string
	Notes                 string
	ArchiveReasonCategory string
}

// opportunityOnlyEventKinds is the set of event kinds whose
// preconditions and side effects do not depend on an application.
// "declined" is dual: the without-app branch is opportunity-only; with
// an application present it routes through the application path.
var opportunityOnlyEventKinds = map[string]bool{
	"exploring": true,
	"archived":  true,
	"note":      true,
	"follow_up": true,
	"custom":    true,
	"declined":  true,
}

// applicationEventKinds is the set of event kinds whose semantics
// require an existing application. AppendEvent routes them through the
// application-transition path. "declined" lives in
// applicationTransitions too but only routes here when an application
// already exists.
var applicationEventKinds = map[string]bool{
	"screen":        true,
	"technical":     true,
	"system_design": true,
	"behavioral":    true,
	"onsite":        true,
	"offer":         true,
	"counter":       true,
	"accepted":      true,
	"rejected":      true,
	"withdrawn":     true,
}

// appTransition describes how an application-tied event mutates the
// application row. from is the set of application statuses the kind
// accepts (the active app's status must be in it); to is the status to
// write; archives mirrors archived_at = events.occurred_at when true;
// reasonCategories is non-nil exactly when the kind requires an
// archive_reason_category, listing the permitted values.
type appTransition struct {
	from             map[string]bool
	to               string
	archives         bool
	reasonCategories map[string]bool
}

// fromInterviewStatuses: an interview event (screen/technical/…) is
// only valid while the application sits at applied or in_progress.
var fromInterviewStatuses = map[string]bool{
	"applied":     true,
	"in_progress": true,
}

// fromActiveStatuses covers every active state — applied, in_progress,
// offer. Used by offer/counter, rejected, declined (with-app), withdrawn.
var fromActiveStatuses = map[string]bool{
	"applied":     true,
	"in_progress": true,
	"offer":       true,
}

// fromOfferOnly is the precondition for accepted: an offer must be on
// the table before it can be accepted.
var fromOfferOnly = map[string]bool{"offer": true}

// rejectedReasonCategories mirrors applications_archive_reason_chk for
// status = 'rejected' — the reasons a recruiter ends a process.
var rejectedReasonCategories = map[string]bool{
	"fit_mismatch":    true,
	"team_preference": true,
	"role_change":     true,
	"process_ended":   true,
	"other":           true,
}

// declineReasonCategories mirrors the constraint's declined/withdrawn
// arms — the reasons the applicant ends the process.
var declineReasonCategories = map[string]bool{
	"compensation": true,
	"scope":        true,
	"team_fit":     true,
	"timing":       true,
	"other":        true,
}

// applicationTransitions enumerates the spec's transition table for
// application-tied event kinds. Includes "declined" because the
// with-app branch behaves like rejected/withdrawn; the no-app branch
// is handled in the opportunity-only path.
var applicationTransitions = map[string]appTransition{
	"screen":        {from: fromInterviewStatuses, to: "in_progress"},
	"technical":     {from: fromInterviewStatuses, to: "in_progress"},
	"system_design": {from: fromInterviewStatuses, to: "in_progress"},
	"behavioral":    {from: fromInterviewStatuses, to: "in_progress"},
	"onsite":        {from: fromInterviewStatuses, to: "in_progress"},
	"offer":         {from: fromActiveStatuses, to: "offer"},
	"counter":       {from: fromActiveStatuses, to: "offer"},
	"accepted":      {from: fromOfferOnly, to: "accepted", archives: true},
	"rejected":      {from: fromActiveStatuses, to: "rejected", archives: true, reasonCategories: rejectedReasonCategories},
	"declined":      {from: fromActiveStatuses, to: "declined", archives: true, reasonCategories: declineReasonCategories},
	"withdrawn":     {from: fromActiveStatuses, to: "withdrawn", archives: true, reasonCategories: declineReasonCategories},
}

// eventRoute is how AppendEvent decides which path handles a kind.
type eventRoute int

const (
	routeOpportunity eventRoute = iota + 1
	routeApplication
)

// routeEvent picks the path AppendEvent takes. Most kinds are statically
// routed; "declined" is dual — without an application it stays on the
// opportunity-only path (which archives the opportunity), with one it
// flips through the application transition.
func routeEvent(kind string, state store.OpportunityStatusInputs) (eventRoute, error) {
	if applicationEventKinds[kind] {
		return routeApplication, nil
	}
	if kind == "declined" && state.AnyApp {
		return routeApplication, nil
	}
	if opportunityOnlyEventKinds[kind] {
		return routeOpportunity, nil
	}
	return 0, fmt.Errorf("%w: kind %q is not yet supported", ErrValidation, kind)
}

// applyAppTransition validates kind/current/reason against the
// transition table and returns the rule to apply. It is pure — no DB
// access — so the table is unit-tested in isolation.
//
// Returns ErrPrecondition when no active app exists or its status is
// not in the kind's "from" set; ErrValidation when the
// archive_reason_category is missing, supplied for a kind that doesn't
// take one, or not in the kind's permitted set.
func applyAppTransition(kind, currentStatus, reasonCategory string) (appTransition, error) {
	rule, ok := applicationTransitions[kind]
	if !ok {
		return appTransition{}, fmt.Errorf("%w: kind %q is not an application transition", ErrValidation, kind)
	}
	if currentStatus == "" {
		return appTransition{}, fmt.Errorf("%w: %s requires an active application", ErrPrecondition, kind)
	}
	if !rule.from[currentStatus] {
		return appTransition{}, fmt.Errorf("%w: %s is not valid while application status is %q", ErrPrecondition, kind, currentStatus)
	}
	if rule.reasonCategories == nil {
		if reasonCategory != "" {
			return appTransition{}, fmt.Errorf("%w: archive_reason_category is only valid for terminal events that require one", ErrValidation)
		}
	} else {
		if reasonCategory == "" {
			return appTransition{}, fmt.Errorf("%w: archive_reason_category is required for %s", ErrValidation, kind)
		}
		if !rule.reasonCategories[reasonCategory] {
			return appTransition{}, fmt.Errorf("%w: archive_reason_category %q is not valid for %s", ErrValidation, reasonCategory, kind)
		}
	}
	return rule, nil
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

// AppendEvent validates an event against the spec's transition table,
// then writes the event, applies the matching side effects
// (opportunity archive stamps for archived/declined-without-app;
// application status + archived_at mirror for the application-tied
// kinds), and recomputes latest_status — all in one transaction.
// Either every change lands or none do.
//
// Returns ErrValidation for malformed input (unknown kind, label/
// archive_reason_category mismatch), ErrPrecondition when the event is
// well-formed but invalid for the opportunity or application's current
// state (e.g. exploring once applications exist; accepted without an
// offer), and store.ErrNotFound for an unknown opportunity.
func (s *Service) AppendEvent(ctx context.Context, in EventInput) (model.Event, error) {
	const op = "service.AppendEvent"

	oppID := strings.TrimSpace(in.OpportunityID)
	if oppID == "" {
		return model.Event{}, fmt.Errorf("%w: opportunity is required", ErrValidation)
	}
	// `applied` only lands via AddApplication so the partial-unique-index
	// guard runs as part of the insert; calling AppendEvent("applied")
	// would skip that contract.
	if in.Kind == "applied" {
		return model.Event{}, fmt.Errorf("%w: use AddApplication to emit applied", ErrValidation)
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

	route, err := routeEvent(in.Kind, state)
	if err != nil {
		return model.Event{}, err
	}

	now := s.clock.Now()

	switch route {
	case routeOpportunity:
		return s.appendOpportunityEvent(ctx, tx, oppID, in, state, label, now)
	case routeApplication:
		return s.appendApplicationEvent(ctx, tx, oppID, in, state, label, now)
	default:
		return model.Event{}, fmt.Errorf("%s: unreachable route %d", op, route)
	}
}

// appendOpportunityEvent handles the kinds whose side effects live on
// the opportunity row: exploring/note/follow_up/custom (no side effect
// beyond the event row), archived (stamps archived_at + archive_reason),
// and the no-app declined branch (stamps archived_at only).
func (s *Service) appendOpportunityEvent(ctx context.Context, tx pgx.Tx, oppID string, in EventInput, state store.OpportunityStatusInputs, label *string, now time.Time) (model.Event, error) {
	const op = "service.AppendEvent"

	if err := checkOpportunityEventPrecondition(in.Kind, state); err != nil {
		return model.Event{}, err
	}
	if in.ArchiveReasonCategory != "" {
		return model.Event{}, fmt.Errorf("%w: archive_reason_category is only valid for terminal application events", ErrValidation)
	}

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

// appendApplicationEvent handles the application-tied kinds (screen,
// technical, system_design, behavioral, onsite, offer, counter,
// accepted, rejected, declined-with-app, withdrawn): flip the active
// application's status (mirroring archived_at on terminals), insert the
// event linked to that application, recompute latest_status.
func (s *Service) appendApplicationEvent(ctx context.Context, tx pgx.Tx, oppID string, in EventInput, state store.OpportunityStatusInputs, label *string, now time.Time) (model.Event, error) {
	const op = "service.AppendEvent"

	if state.Archived {
		return model.Event{}, fmt.Errorf("%w: opportunity is archived", ErrPrecondition)
	}

	rule, err := applyAppTransition(in.Kind, state.ActiveAppStatus, in.ArchiveReasonCategory)
	if err != nil {
		return model.Event{}, err
	}

	var archivedAt *time.Time
	if rule.archives {
		archivedAt = &now
	}
	if err := s.store.SetApplicationStatus(ctx, tx, state.ActiveAppID, rule.to, archivedAt, nullableString(in.ArchiveReasonCategory)); err != nil {
		return model.Event{}, fmt.Errorf("%s: %w", op, err)
	}

	appID := state.ActiveAppID
	ev, err := s.store.InsertEvent(ctx, tx, store.EventParams{
		OpportunityID: oppID,
		ApplicationID: &appID,
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

// checkOpportunityEventPrecondition enforces the "required state" for
// the opportunity-only kinds before any write hits the DB. The
// declined-with-app branch is routed away by routeEvent and so isn't
// re-checked here; only the no-app declined path reaches this function.
func checkOpportunityEventPrecondition(kind string, state store.OpportunityStatusInputs) error {
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
		// Only the no-application branch reaches here (routeEvent sends
		// declined-with-app to the application path). Archived
		// opportunities are still off-limits.
		if state.Archived {
			return fmt.Errorf("%w: opportunity is already archived", ErrPrecondition)
		}
	case "note", "follow_up", "custom":
		// Informational; no state-machine precondition.
	}
	return nil
}
