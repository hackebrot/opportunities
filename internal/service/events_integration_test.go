//go:build integration

package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hackebrot/opportunities/internal/service"
	"github.com/hackebrot/opportunities/internal/store"
)

func TestIntegrationAppendEventExploring(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)
	oppID := seedOpportunity(ctx, t, svc)

	ev, err := svc.AppendEvent(ctx, service.EventInput{
		OpportunityID: oppID,
		Kind:          "exploring",
	})
	if err != nil {
		t.Fatalf("append exploring: %v", err)
	}
	if ev.Kind != "exploring" {
		t.Fatalf("event kind = %q, want exploring", ev.Kind)
	}
	if !ev.OccurredAt.Equal(testClock.t) {
		t.Fatalf("event occurred_at = %s, want %s", ev.OccurredAt, testClock.t)
	}
	if got := readOpportunityLatestStatus(ctx, t, st, oppID); got != "exploring" {
		t.Fatalf("latest_status = %q, want exploring", got)
	}
}

func TestIntegrationAppendEventArchived(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)
	oppID := seedOpportunity(ctx, t, svc)

	ev, err := svc.ArchiveOpportunity(ctx, oppID, "delisted")
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	if ev.Kind != "archived" || ev.Notes != "delisted" {
		t.Fatalf("event = %+v, want kind=archived notes=delisted", ev)
	}

	opp, err := st.GetOpportunity(ctx, oppID)
	if err != nil {
		t.Fatalf("get opp: %v", err)
	}
	if opp.LatestStatus != "archived" {
		t.Fatalf("latest_status = %q, want archived", opp.LatestStatus)
	}
	if opp.ArchivedAt == nil || !opp.ArchivedAt.Equal(testClock.t) {
		t.Fatalf("archived_at = %v, want %s", opp.ArchivedAt, testClock.t)
	}
	if opp.ArchiveReason == nil || *opp.ArchiveReason != "delisted" {
		t.Fatalf("archive_reason = %v, want %q", opp.ArchiveReason, "delisted")
	}

	// Re-archiving an already-archived opportunity is a precondition error.
	if _, err := svc.ArchiveOpportunity(ctx, oppID, "again"); !errors.Is(err, service.ErrPrecondition) {
		t.Fatalf("second archive: want ErrPrecondition, got %v", err)
	}
}

func TestIntegrationAppendEventDeclinedNoApp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)
	oppID := seedOpportunity(ctx, t, svc)

	if _, err := svc.AppendEvent(ctx, service.EventInput{
		OpportunityID: oppID,
		Kind:          "declined",
		Notes:         "not interested",
	}); err != nil {
		t.Fatalf("declined: %v", err)
	}

	opp, err := st.GetOpportunity(ctx, oppID)
	if err != nil {
		t.Fatalf("get opp: %v", err)
	}
	if opp.LatestStatus != "archived" {
		t.Fatalf("latest_status = %q, want archived", opp.LatestStatus)
	}
	if opp.ArchivedAt == nil || !opp.ArchivedAt.Equal(testClock.t) {
		t.Fatalf("archived_at = %v, want %s", opp.ArchivedAt, testClock.t)
	}
	// Spec: declined-without-app sets archived_at only — archive_reason
	// stays untouched (the event row carries the notes).
	if opp.ArchiveReason != nil {
		t.Fatalf("archive_reason = %q, want untouched (nil)", *opp.ArchiveReason)
	}
}

func TestIntegrationAppendEventPassiveKinds(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)
	oppID := seedOpportunity(ctx, t, svc)

	// Each passive kind appends a row and leaves latest_status alone.
	cases := []service.EventInput{
		{OpportunityID: oppID, Kind: "note", Notes: "promising"},
		{OpportunityID: oppID, Kind: "follow_up", Notes: "pinged Friday"},
		{OpportunityID: oppID, Kind: "custom", Label: "prep material received", Notes: "system design prep doc from recruiter"},
	}
	for _, in := range cases {
		if _, err := svc.AppendEvent(ctx, in); err != nil {
			t.Fatalf("append %s: %v", in.Kind, err)
		}
	}
	if got := readOpportunityLatestStatus(ctx, t, st, oppID); got != "watching" {
		t.Fatalf("latest_status = %q, want watching (passive kinds do not transition)", got)
	}

	// custom requires a label; missing one is ErrValidation.
	if _, err := svc.AppendEvent(ctx, service.EventInput{
		OpportunityID: oppID,
		Kind:          "custom",
	}); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("custom without label: want ErrValidation, got %v", err)
	}
	// Non-custom kinds reject a label.
	if _, err := svc.AppendEvent(ctx, service.EventInput{
		OpportunityID: oppID,
		Kind:          "note",
		Label:         "stray label",
	}); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("note with label: want ErrValidation, got %v", err)
	}
}

func TestIntegrationAppendEventValidation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)

	// Missing opportunity id.
	if _, err := svc.AppendEvent(ctx, service.EventInput{Kind: "note"}); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("missing opportunity: want ErrValidation, got %v", err)
	}

	// `applied` is reserved for AddApplication so the partial-unique-index
	// guard runs as part of the insert; AppendEvent rejects it as a
	// validation error regardless of state.
	oppID := seedOpportunity(ctx, t, svc)
	if _, err := svc.AppendEvent(ctx, service.EventInput{
		OpportunityID: oppID,
		Kind:          "applied",
	}); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("kind \"applied\": want ErrValidation, got %v", err)
	}

	// The other application-tied kinds are well-formed but require an
	// active application; without one they fail the precondition.
	for _, kind := range []string{"screen", "offer", "accepted", "rejected", "withdrawn"} {
		if _, err := svc.AppendEvent(ctx, service.EventInput{
			OpportunityID: oppID,
			Kind:          kind,
		}); !errors.Is(err, service.ErrPrecondition) {
			t.Fatalf("kind %q without active app: want ErrPrecondition, got %v", kind, err)
		}
	}

	// Unknown opportunity id surfaces store.ErrNotFound.
	if _, err := svc.AppendEvent(ctx, service.EventInput{
		OpportunityID: "00000000-0000-0000-0000-000000000000",
		Kind:          "note",
	}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown opportunity: want store.ErrNotFound, got %v", err)
	}
}

// TestIntegrationAppendEventOppKindRejectsReasonCategory proves the
// opportunity-only path refuses ArchiveReasonCategory — that field is
// reserved for terminal application events. Caught at the service
// layer before any DB write.
func TestIntegrationAppendEventOppKindRejectsReasonCategory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)
	oppID := seedOpportunity(ctx, t, svc)

	for _, kind := range []string{"note", "follow_up", "exploring", "archived", "declined"} {
		t.Run(kind, func(t *testing.T) {
			if _, err := svc.AppendEvent(ctx, service.EventInput{
				OpportunityID:         oppID,
				Kind:                  kind,
				ArchiveReasonCategory: "other",
			}); !errors.Is(err, service.ErrValidation) {
				t.Fatalf("kind %q with reason category: want ErrValidation, got %v", kind, err)
			}
		})
	}

	// Nothing landed: still just the `added` event from seedOpportunity.
	var n int
	if err := st.Pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE opportunity_id = $1`, oppID).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if n != 1 {
		t.Fatalf("event count = %d, want 1 (added only)", n)
	}
}

func TestIntegrationAppendEventPreconditionShortCircuitsBeforeWrite(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)
	oppID := seedOpportunity(ctx, t, svc)

	// Archive once — succeeds and sets archived_at.
	if _, err := svc.ArchiveOpportunity(ctx, oppID, "delisted"); err != nil {
		t.Fatalf("first archive: %v", err)
	}

	// Once archived, exploring is no longer reachable through the
	// service entry point: precondition rejects it before any DB write,
	// so the event count for the opportunity stays at 2 (added +
	// archived).
	if _, err := svc.AppendEvent(ctx, service.EventInput{
		OpportunityID: oppID,
		Kind:          "exploring",
	}); !errors.Is(err, service.ErrPrecondition) {
		t.Fatalf("exploring on archived opp: want ErrPrecondition, got %v", err)
	}
	var n int
	if err := st.Pool.QueryRow(ctx, `SELECT count(*) FROM events WHERE opportunity_id = $1`, oppID).Scan(&n); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if n != 2 {
		t.Fatalf("event count = %d, want 2 (added + archived)", n)
	}
}
