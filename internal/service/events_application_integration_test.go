//go:build integration

package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hackebrot/opportunities/internal/service"
)

// seedApplicationAt returns a freshly-created application sitting in
// `applied` for a fresh opportunity. The helper hides the company +
// opportunity seed boilerplate so the transition tests can focus on
// the event flow.
func seedApplicationAt(ctx context.Context, t *testing.T, svc *service.Service) (oppID, appID string) {
	t.Helper()
	oppID = seedOpportunity(ctx, t, svc)
	app, err := svc.AddApplication(ctx, service.ApplicationInput{OpportunityID: oppID})
	if err != nil {
		t.Fatalf("add application: %v", err)
	}
	return oppID, app.ID
}

// TestIntegrationAppendEventInterviewKinds proves each interview kind
// (screen, technical, system_design, behavioral, onsite) flips an
// applied application to in_progress, mirrors that onto the
// opportunity's latest_status, and links the event to the application.
func TestIntegrationAppendEventInterviewKinds(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	for _, kind := range []string{"screen", "technical", "system_design", "behavioral", "onsite"} {
		t.Run(kind, func(t *testing.T) {
			st := startPostgresStore(ctx, t)
			if err := st.MigrateUp(ctx); err != nil {
				t.Fatalf("migrate up: %v", err)
			}
			svc := service.New(st, testClock)
			oppID, appID := seedApplicationAt(ctx, t, svc)

			ev, err := svc.AppendEvent(ctx, service.EventInput{
				OpportunityID: oppID,
				Kind:          kind,
			})
			if err != nil {
				t.Fatalf("append %s: %v", kind, err)
			}
			if ev.ApplicationID == nil || *ev.ApplicationID != appID {
				t.Fatalf("event application_id = %v, want %q", ev.ApplicationID, appID)
			}

			app, err := st.GetApplication(ctx, appID)
			if err != nil {
				t.Fatalf("get application: %v", err)
			}
			if app.Status != "in_progress" {
				t.Fatalf("application status = %q, want in_progress", app.Status)
			}
			if app.ArchivedAt != nil {
				t.Fatalf("non-terminal kind set archived_at = %v", app.ArchivedAt)
			}
			if got := readOpportunityLatestStatus(ctx, t, st, oppID); got != "in_progress" {
				t.Fatalf("latest_status = %q, want in_progress", got)
			}
		})
	}
}

// TestIntegrationAppendEventOfferKinds proves offer and counter flip an
// active application to offer (from any active state) and mirror that
// onto the opportunity's latest_status.
func TestIntegrationAppendEventOfferKinds(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	for _, kind := range []string{"offer", "counter"} {
		t.Run(kind, func(t *testing.T) {
			st := startPostgresStore(ctx, t)
			if err := st.MigrateUp(ctx); err != nil {
				t.Fatalf("migrate up: %v", err)
			}
			svc := service.New(st, testClock)
			oppID, appID := seedApplicationAt(ctx, t, svc)

			if _, err := svc.AppendEvent(ctx, service.EventInput{
				OpportunityID: oppID,
				Kind:          kind,
			}); err != nil {
				t.Fatalf("append %s: %v", kind, err)
			}
			app, err := st.GetApplication(ctx, appID)
			if err != nil {
				t.Fatalf("get application: %v", err)
			}
			if app.Status != "offer" {
				t.Fatalf("application status = %q, want offer", app.Status)
			}
			if got := readOpportunityLatestStatus(ctx, t, st, oppID); got != "offer" {
				t.Fatalf("latest_status = %q, want offer", got)
			}
		})
	}
}

// TestIntegrationAppendEventAccepted proves accepted requires an active
// offer, flips the application terminal, mirrors archived_at = the
// event's occurred_at, and the opportunity's latest_status becomes
// accepted.
func TestIntegrationAppendEventAccepted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)
	oppID, appID := seedApplicationAt(ctx, t, svc)

	// accepted before an offer exists fails the precondition.
	if _, err := svc.AppendEvent(ctx, service.EventInput{
		OpportunityID: oppID,
		Kind:          "accepted",
	}); !errors.Is(err, service.ErrPrecondition) {
		t.Fatalf("accepted before offer: want ErrPrecondition, got %v", err)
	}

	if _, err := svc.AppendEvent(ctx, service.EventInput{OpportunityID: oppID, Kind: "offer"}); err != nil {
		t.Fatalf("offer: %v", err)
	}
	ev, err := svc.AppendEvent(ctx, service.EventInput{OpportunityID: oppID, Kind: "accepted"})
	if err != nil {
		t.Fatalf("accepted: %v", err)
	}

	app, err := st.GetApplication(ctx, appID)
	if err != nil {
		t.Fatalf("get application: %v", err)
	}
	if app.Status != "accepted" {
		t.Fatalf("application status = %q, want accepted", app.Status)
	}
	if app.ArchivedAt == nil || !app.ArchivedAt.Equal(ev.OccurredAt) {
		t.Fatalf("archived_at = %v, want %s (event.occurred_at)", app.ArchivedAt, ev.OccurredAt)
	}
	if app.ArchiveReasonCategory != nil {
		t.Fatalf("archive_reason_category = %v, want nil for accepted", app.ArchiveReasonCategory)
	}
	if got := readOpportunityLatestStatus(ctx, t, st, oppID); got != "accepted" {
		t.Fatalf("latest_status = %q, want accepted", got)
	}
}

// TestIntegrationAppendEventRejected proves rejected flips an active
// application terminal, requires an archive_reason_category from the
// rejected bucket, mirrors archived_at, and drops the opportunity to
// dormant.
func TestIntegrationAppendEventRejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)
	oppID, appID := seedApplicationAt(ctx, t, svc)

	// Missing category is a validation error.
	if _, err := svc.AppendEvent(ctx, service.EventInput{
		OpportunityID: oppID,
		Kind:          "rejected",
	}); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("rejected without category: want ErrValidation, got %v", err)
	}
	// Wrong-bucket category is a validation error.
	if _, err := svc.AppendEvent(ctx, service.EventInput{
		OpportunityID:         oppID,
		Kind:                  "rejected",
		ArchiveReasonCategory: "compensation",
	}); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("rejected with declined-bucket category: want ErrValidation, got %v", err)
	}

	ev, err := svc.AppendEvent(ctx, service.EventInput{
		OpportunityID:         oppID,
		Kind:                  "rejected",
		ArchiveReasonCategory: "process_ended",
	})
	if err != nil {
		t.Fatalf("rejected: %v", err)
	}

	app, err := st.GetApplication(ctx, appID)
	if err != nil {
		t.Fatalf("get application: %v", err)
	}
	if app.Status != "rejected" {
		t.Fatalf("application status = %q, want rejected", app.Status)
	}
	if app.ArchivedAt == nil || !app.ArchivedAt.Equal(ev.OccurredAt) {
		t.Fatalf("archived_at = %v, want %s (event.occurred_at)", app.ArchivedAt, ev.OccurredAt)
	}
	if app.ArchiveReasonCategory == nil || *app.ArchiveReasonCategory != "process_ended" {
		t.Fatalf("archive_reason_category = %v, want process_ended", app.ArchiveReasonCategory)
	}
	if got := readOpportunityLatestStatus(ctx, t, st, oppID); got != "dormant" {
		t.Fatalf("latest_status = %q, want dormant", got)
	}

	// A second rejected against a now-terminal application is a
	// precondition failure (no active app).
	if _, err := svc.AppendEvent(ctx, service.EventInput{
		OpportunityID:         oppID,
		Kind:                  "rejected",
		ArchiveReasonCategory: "process_ended",
	}); !errors.Is(err, service.ErrPrecondition) {
		t.Fatalf("rejected on terminal app: want ErrPrecondition, got %v", err)
	}
}

// TestIntegrationAppendEventDeclinedWithApp proves declined with an
// active app routes through the application path: flips the app
// terminal, requires a declined-bucket category, and recomputes the
// opportunity to dormant (not archived — the opportunity row stays
// alive so a re-apply can land later).
func TestIntegrationAppendEventDeclinedWithApp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)
	oppID, appID := seedApplicationAt(ctx, t, svc)

	if _, err := svc.AppendEvent(ctx, service.EventInput{
		OpportunityID:         oppID,
		Kind:                  "declined",
		ArchiveReasonCategory: "compensation",
	}); err != nil {
		t.Fatalf("declined: %v", err)
	}

	app, err := st.GetApplication(ctx, appID)
	if err != nil {
		t.Fatalf("get application: %v", err)
	}
	if app.Status != "declined" {
		t.Fatalf("application status = %q, want declined", app.Status)
	}
	if app.ArchivedAt == nil || !app.ArchivedAt.Equal(testClock.t) {
		t.Fatalf("archived_at = %v, want %s", app.ArchivedAt, testClock.t)
	}
	if app.ArchiveReasonCategory == nil || *app.ArchiveReasonCategory != "compensation" {
		t.Fatalf("archive_reason_category = %v, want compensation", app.ArchiveReasonCategory)
	}

	opp, err := st.GetOpportunity(ctx, oppID)
	if err != nil {
		t.Fatalf("get opportunity: %v", err)
	}
	if opp.LatestStatus != "dormant" {
		t.Fatalf("latest_status = %q, want dormant", opp.LatestStatus)
	}
	if opp.ArchivedAt != nil {
		t.Fatalf("opportunity archived_at = %v, want nil (declined-with-app must not archive the opportunity)", opp.ArchivedAt)
	}
}

// TestIntegrationAppendEventWithdrawn proves withdrawn is identical in
// shape to rejected/declined-with-app: terminal, archived_at mirrors
// the event, declined-bucket category required, opportunity goes
// dormant.
func TestIntegrationAppendEventWithdrawn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)
	oppID, appID := seedApplicationAt(ctx, t, svc)

	if _, err := svc.AppendEvent(ctx, service.EventInput{
		OpportunityID:         oppID,
		Kind:                  "withdrawn",
		ArchiveReasonCategory: "team_fit",
	}); err != nil {
		t.Fatalf("withdrawn: %v", err)
	}

	app, err := st.GetApplication(ctx, appID)
	if err != nil {
		t.Fatalf("get application: %v", err)
	}
	if app.Status != "withdrawn" {
		t.Fatalf("application status = %q, want withdrawn", app.Status)
	}
	if app.ArchivedAt == nil || !app.ArchivedAt.Equal(testClock.t) {
		t.Fatalf("archived_at = %v, want %s", app.ArchivedAt, testClock.t)
	}
	if got := readOpportunityLatestStatus(ctx, t, st, oppID); got != "dormant" {
		t.Fatalf("latest_status = %q, want dormant", got)
	}
}

// TestIntegrationAppendEventApplicationPreconditionShortCircuits proves
// the app-tied path fails before any write when the active application
// isn't in the kind's "from" set: count of events stays at 2 (added +
// applied).
func TestIntegrationAppendEventApplicationPreconditionShortCircuits(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)
	oppID, _ := seedApplicationAt(ctx, t, svc)

	// accepted is invalid when the app is in 'applied' (needs 'offer').
	if _, err := svc.AppendEvent(ctx, service.EventInput{
		OpportunityID: oppID,
		Kind:          "accepted",
	}); !errors.Is(err, service.ErrPrecondition) {
		t.Fatalf("accepted from applied: want ErrPrecondition, got %v", err)
	}

	var n int
	if err := st.Pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE opportunity_id = $1`, oppID).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if n != 2 {
		t.Fatalf("event count = %d, want 2 (added + applied)", n)
	}
}

// TestIntegrationAppendEventArchivedRejectsAppKinds proves an archived
// opportunity can't accept application-tied events, even when an
// active app would otherwise be eligible.
func TestIntegrationAppendEventArchivedRejectsAppKinds(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)
	oppID, _ := seedApplicationAt(ctx, t, svc)

	// Archive the opportunity directly via the store (the service path
	// would refuse on the active-app check; this is the only way to land
	// the test's preconditions).
	if err := st.SetOpportunityArchived(ctx, st.Pool, oppID, testClock.t, nil); err != nil {
		t.Fatalf("archive opportunity: %v", err)
	}

	if _, err := svc.AppendEvent(ctx, service.EventInput{
		OpportunityID: oppID,
		Kind:          "screen",
	}); !errors.Is(err, service.ErrPrecondition) {
		t.Fatalf("screen on archived opp: want ErrPrecondition, got %v", err)
	}
}
