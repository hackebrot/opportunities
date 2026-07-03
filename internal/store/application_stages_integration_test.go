//go:build integration

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/hackebrot/opportunities/internal/model"
)

// TestIntegrationApplicationStagesCRUD round-trips every application_stages
// store method against a real Postgres, covering the nullable
// scheduled_at/completed_at/outcome columns and order_index ordering.
func TestIntegrationApplicationStagesCRUD(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store := startPostgresStore(ctx, t)
	if err := store.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	oppID := seedOpportunity(ctx, t, store, "Acme Corp", "acmecorp")
	app, err := store.InsertApplication(ctx, store.Pool, ApplicationParams{
		OpportunityID: oppID,
	}, "applied")
	if err != nil {
		t.Fatalf("seed application: %v", err)
	}

	params := ApplicationStageParams{
		ApplicationID:    app.ID,
		OrderIndex:       1,
		Label:            "Recruiter screen",
		ExpectationNotes: "30 min intro call",
		PrepNotes:        "read the JD again",
		ScheduledAt:      new(time.Date(2026, 6, 1, 14, 0, 0, 0, time.UTC)),
	}

	created, err := store.InsertApplicationStage(ctx, store.Pool, params)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("create: empty id")
	}
	if created.CompletedAt != nil || created.Outcome != nil {
		t.Fatalf("create: expected nil completed_at/outcome, got %+v", created)
	}
	want := model.ApplicationStage{
		ID:               created.ID,
		ApplicationID:    app.ID,
		OrderIndex:       1,
		Label:            "Recruiter screen",
		ExpectationNotes: "30 min intro call",
		PrepNotes:        "read the JD again",
		ScheduledAt:      new(time.Date(2026, 6, 1, 14, 0, 0, 0, time.UTC)),
		CreatedAt:        created.CreatedAt,
		UpdatedAt:        created.UpdatedAt,
	}
	if !cmp.Equal(want, created) {
		t.Fatalf("create row (-want +got):\n%s", cmp.Diff(want, created))
	}

	got, err := store.GetApplicationStage(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !cmp.Equal(created, got) {
		t.Fatalf("get round-trip (-want +got):\n%s", cmp.Diff(created, got))
	}

	// A second stage at a lower order_index proves List sorts by
	// order_index, not insertion order.
	second, err := store.InsertApplicationStage(ctx, store.Pool, ApplicationStageParams{
		ApplicationID: app.ID,
		OrderIndex:    0,
		Label:         "Application submitted",
	})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	list, err := store.ListApplicationStages(ctx, app.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	wantList := []model.ApplicationStage{second, created}
	if !cmp.Equal(wantList, list) {
		t.Fatalf("list (-want +got):\n%s", cmp.Diff(wantList, list))
	}

	// Mark the recruiter screen completed with an outcome.
	updateParams := params
	updateParams.CompletedAt = new(time.Date(2026, 6, 1, 14, 30, 0, 0, time.UTC))
	updateParams.Outcome = new("passed")
	updated, err := store.UpdateApplicationStage(ctx, created.ID, updateParams)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Outcome == nil || *updated.Outcome != "passed" {
		t.Fatalf("update: outcome not written: %+v", updated)
	}
	if updated.CompletedAt == nil || !updated.CompletedAt.Equal(*updateParams.CompletedAt) {
		t.Fatalf("update: completed_at not written: %+v", updated)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("update: updated_at not bumped (was %s, now %s)",
			created.UpdatedAt, updated.UpdatedAt)
	}

	if err := store.DeleteApplicationStage(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.GetApplicationStage(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete: want ErrNotFound, got %v", err)
	}
	if err := store.DeleteApplicationStage(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: want ErrNotFound, got %v", err)
	}
}

// TestIntegrationApplicationStagesConstraints covers the guard rails: a
// duplicate order_index, a bad outcome, and an unknown application id.
func TestIntegrationApplicationStagesConstraints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store := startPostgresStore(ctx, t)
	if err := store.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	oppID := seedOpportunity(ctx, t, store, "Acme Corp", "acmecorp")
	app, err := store.InsertApplication(ctx, store.Pool, ApplicationParams{
		OpportunityID: oppID,
	}, "applied")
	if err != nil {
		t.Fatalf("seed application: %v", err)
	}

	if _, err := store.InsertApplicationStage(ctx, store.Pool, ApplicationStageParams{
		ApplicationID: app.ID,
		OrderIndex:    1,
		Label:         "Screen",
	}); err != nil {
		t.Fatalf("first stage: %v", err)
	}

	// A second stage at the same order_index trips the UNIQUE constraint.
	_, err = store.InsertApplicationStage(ctx, store.Pool, ApplicationStageParams{
		ApplicationID: app.ID,
		OrderIndex:    1,
		Label:         "Duplicate slot",
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate order_index: want ErrConflict, got %v", err)
	}

	// A bad outcome trips application_stages_outcome_chk.
	_, err = store.InsertApplicationStage(ctx, store.Pool, ApplicationStageParams{
		ApplicationID: app.ID,
		OrderIndex:    2,
		Label:         "Onsite",
		Outcome:       new("aced_it"),
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("bad outcome: want ErrConflict, got %v", err)
	}

	// An unknown application id surfaces as ErrNotFound.
	_, err = store.InsertApplicationStage(ctx, store.Pool, ApplicationStageParams{
		ApplicationID: "00000000-0000-0000-0000-000000000000",
		OrderIndex:    1,
		Label:         "Ghost",
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown application: want ErrNotFound, got %v", err)
	}
}
