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

// TestIntegrationFollowUpApplicationStamp proves the no-flag path:
// stamps last_followed_up_at to the clock instant, leaves the block
// untouched, and writes a follow_up event linked to the application.
func TestIntegrationFollowUpApplicationStamp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)
	oppID, appID := seedApplicationAt(ctx, t, svc)

	app, err := svc.FollowUpApplication(ctx, appID, service.FollowUpStamp)
	if err != nil {
		t.Fatalf("follow-up stamp: %v", err)
	}
	if app.LastFollowedUpAt == nil || !app.LastFollowedUpAt.Equal(testClock.t) {
		t.Fatalf("LastFollowedUpAt = %v, want %v", app.LastFollowedUpAt, testClock.t)
	}
	if app.FollowUpBlocked {
		t.Fatalf("FollowUpBlocked = true, want false (stamp leaves block alone)")
	}

	// Exactly one follow_up event tied to the application.
	var (
		count       int
		eventAppID  *string
		occurredAt  time.Time
		anyLabelSet bool
	)
	err = st.Pool.QueryRow(ctx, `
		SELECT count(*) OVER (), application_id, occurred_at, label IS NOT NULL
		FROM events
		WHERE opportunity_id = $1 AND kind = 'follow_up'`, oppID).
		Scan(&count, &eventAppID, &occurredAt, &anyLabelSet)
	if err != nil {
		t.Fatalf("query follow_up event: %v", err)
	}
	if count != 1 {
		t.Fatalf("follow_up events = %d, want 1", count)
	}
	if eventAppID == nil || *eventAppID != appID {
		t.Fatalf("follow_up application_id = %v, want %q", eventAppID, appID)
	}
	if !occurredAt.Equal(testClock.t) {
		t.Fatalf("follow_up occurred_at = %s, want %s", occurredAt, testClock.t)
	}
	if anyLabelSet {
		t.Fatalf("follow_up label set, want NULL")
	}
	// latest_status is "applied" before and after — follow-up doesn't move
	// the state machine.
	if got := readOpportunityLatestStatus(ctx, t, st, oppID); got != "applied" {
		t.Fatalf("latest_status = %q, want applied", got)
	}
}

// TestIntegrationFollowUpApplicationBlock proves --blocked sets the
// flag, leaves the timestamp alone, and does not emit an event.
func TestIntegrationFollowUpApplicationBlock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)
	oppID, appID := seedApplicationAt(ctx, t, svc)

	app, err := svc.FollowUpApplication(ctx, appID, service.FollowUpBlock)
	if err != nil {
		t.Fatalf("follow-up block: %v", err)
	}
	if !app.FollowUpBlocked {
		t.Fatalf("FollowUpBlocked = false, want true")
	}
	if app.LastFollowedUpAt != nil {
		t.Fatalf("LastFollowedUpAt = %v, want nil (block leaves stamp alone)", app.LastFollowedUpAt)
	}

	// No follow_up event written by block-only.
	var n int
	if err := st.Pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE opportunity_id = $1 AND kind = 'follow_up'`,
		oppID).Scan(&n); err != nil {
		t.Fatalf("count follow_up events: %v", err)
	}
	if n != 0 {
		t.Fatalf("follow_up events after block = %d, want 0", n)
	}
}

// TestIntegrationFollowUpApplicationDone proves --done clears the block,
// stamps the timestamp, and emits a follow_up event in one call.
func TestIntegrationFollowUpApplicationDone(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)
	oppID, appID := seedApplicationAt(ctx, t, svc)

	if _, err := svc.FollowUpApplication(ctx, appID, service.FollowUpBlock); err != nil {
		t.Fatalf("seed block: %v", err)
	}
	app, err := svc.FollowUpApplication(ctx, appID, service.FollowUpDone)
	if err != nil {
		t.Fatalf("follow-up done: %v", err)
	}
	if app.FollowUpBlocked {
		t.Fatalf("FollowUpBlocked after done = true, want false")
	}
	if app.LastFollowedUpAt == nil || !app.LastFollowedUpAt.Equal(testClock.t) {
		t.Fatalf("LastFollowedUpAt after done = %v, want %v", app.LastFollowedUpAt, testClock.t)
	}

	var n int
	if err := st.Pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE opportunity_id = $1 AND kind = 'follow_up'`,
		oppID).Scan(&n); err != nil {
		t.Fatalf("count follow_up events: %v", err)
	}
	if n != 1 {
		t.Fatalf("follow_up events after done = %d, want 1", n)
	}
}

// TestIntegrationFollowUpApplicationTerminal proves a terminal
// application is rejected with ErrPrecondition: there is nothing to
// follow up on once an app is archived.
func TestIntegrationFollowUpApplicationTerminal(t *testing.T) {
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
		Kind:                  "rejected",
		ArchiveReasonCategory: "process_ended",
	}); err != nil {
		t.Fatalf("reject: %v", err)
	}

	if _, err := svc.FollowUpApplication(ctx, appID, service.FollowUpStamp); !errors.Is(err, service.ErrPrecondition) {
		t.Fatalf("stamp terminal: err=%v, want ErrPrecondition", err)
	}
}

// TestIntegrationFollowUpApplicationUnknown proves a missing
// application id surfaces as store.ErrNotFound.
func TestIntegrationFollowUpApplicationUnknown(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)

	_, err := svc.FollowUpApplication(ctx, "00000000-0000-0000-0000-000000000000", service.FollowUpStamp)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown id: want store.ErrNotFound, got %v", err)
	}
}
