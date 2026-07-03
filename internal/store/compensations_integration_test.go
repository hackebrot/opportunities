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

// TestIntegrationCompensationsCRUD round-trips every compensation store
// method against a real Postgres, exercising both the opportunity-linked
// and application-linked variants (the target/application XOR).
func TestIntegrationCompensationsCRUD(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store := startPostgresStore(ctx, t)
	if err := store.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	oppID := seedOpportunity(ctx, t, store, "Acme Corp", "acmecorp")

	params := CompensationParams{
		OpportunityID: &oppID,
		Kind:          "target",
		BaseMinor:     100,
		BonusMinor:    new(int64(50)),
		EquityNotes:   "equity placeholder",
		VestingNotes:  "vesting placeholder",
		Currency:      "EUR",
		Notes:         "target placeholder",
	}

	created, err := store.InsertCompensation(ctx, store.Pool, params)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("create: empty id")
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("create: zero timestamps: %+v", created)
	}
	want := model.Compensation{
		ID:            created.ID,
		OpportunityID: &oppID,
		Kind:          "target",
		BaseMinor:     100,
		BonusMinor:    new(int64(50)),
		EquityNotes:   "equity placeholder",
		VestingNotes:  "vesting placeholder",
		Currency:      "EUR",
		Notes:         "target placeholder",
		CreatedAt:     created.CreatedAt,
		UpdatedAt:     created.UpdatedAt,
	}
	if !cmp.Equal(want, created) {
		t.Fatalf("create row (-want +got):\n%s", cmp.Diff(want, created))
	}

	got, err := store.GetCompensation(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !cmp.Equal(created, got) {
		t.Fatalf("get round-trip (-want +got):\n%s", cmp.Diff(created, got))
	}

	// A second opportunity-linked row (no bonus) to exercise NULL handling
	// and List ordering.
	second, err := store.InsertCompensation(ctx, store.Pool, CompensationParams{
		OpportunityID: &oppID,
		Kind:          "listed",
		BaseMinor:     200,
		Currency:      "USD",
	})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if second.BonusMinor != nil {
		t.Fatalf("second.BonusMinor: want nil, got %d", *second.BonusMinor)
	}

	list, err := store.ListOpportunityCompensations(ctx, oppID)
	if err != nil {
		t.Fatalf("list opportunity: %v", err)
	}
	wantList := []model.Compensation{created, second}
	if !cmp.Equal(wantList, list) {
		t.Fatalf("list opportunity (-want +got):\n%s", cmp.Diff(wantList, list))
	}

	updateParams := params
	updateParams.Notes = "target placeholder updated"
	updateParams.BonusMinor = nil
	updated, err := store.UpdateCompensation(ctx, created.ID, updateParams)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Notes != "target placeholder updated" {
		t.Fatalf("update: notes not written: %+v", updated)
	}
	if updated.BonusMinor != nil {
		t.Fatalf("update: bonus not cleared: %d", *updated.BonusMinor)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("update: updated_at not bumped (was %s, now %s)",
			created.UpdatedAt, updated.UpdatedAt)
	}

	if err := store.DeleteCompensation(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.GetCompensation(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete: want ErrNotFound, got %v", err)
	}
	if err := store.DeleteCompensation(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: want ErrNotFound, got %v", err)
	}
}

// TestIntegrationCompensationsApplicationLink exercises the
// application-linked variant and its dedicated List helper, plus the
// target/application XOR CHECK.
func TestIntegrationCompensationsApplicationLink(t *testing.T) {
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

	created, err := store.InsertCompensation(ctx, store.Pool, CompensationParams{
		ApplicationID: &app.ID,
		Kind:          "offered",
		BaseMinor:     300,
		Currency:      "EUR",
	})
	if err != nil {
		t.Fatalf("create application compensation: %v", err)
	}
	if created.ApplicationID == nil || *created.ApplicationID != app.ID {
		t.Fatalf("application_id = %v, want %s", created.ApplicationID, app.ID)
	}
	if created.OpportunityID != nil {
		t.Fatalf("opportunity_id = %v, want nil", *created.OpportunityID)
	}

	appList, err := store.ListApplicationCompensations(ctx, app.ID)
	if err != nil {
		t.Fatalf("list application: %v", err)
	}
	if !cmp.Equal([]model.Compensation{created}, appList) {
		t.Fatalf("list application (-want +got):\n%s", cmp.Diff([]model.Compensation{created}, appList))
	}

	// The opportunity-scoped list must not surface application-linked rows.
	oppList, err := store.ListOpportunityCompensations(ctx, oppID)
	if err != nil {
		t.Fatalf("list opportunity: %v", err)
	}
	if len(oppList) != 0 {
		t.Fatalf("opportunity list leaked application rows: %+v", oppList)
	}

	// Both links set at once trips compensations_target_xor_chk.
	_, err = store.InsertCompensation(ctx, store.Pool, CompensationParams{
		OpportunityID: &oppID,
		ApplicationID: &app.ID,
		Kind:          "target",
		BaseMinor:     1,
		Currency:      "EUR",
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("both links set: want ErrConflict, got %v", err)
	}

	// Neither link set also trips the XOR.
	_, err = store.InsertCompensation(ctx, store.Pool, CompensationParams{
		Kind:      "target",
		BaseMinor: 1,
		Currency:  "EUR",
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("no link set: want ErrConflict, got %v", err)
	}

	// A bad currency trips compensations_currency_chk.
	_, err = store.InsertCompensation(ctx, store.Pool, CompensationParams{
		OpportunityID: &oppID,
		Kind:          "target",
		BaseMinor:     1,
		Currency:      "GBP",
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("bad currency: want ErrConflict, got %v", err)
	}

	// An unknown opportunity id surfaces as ErrNotFound.
	_, err = store.InsertCompensation(ctx, store.Pool, CompensationParams{
		OpportunityID: new("00000000-0000-0000-0000-000000000000"),
		Kind:          "target",
		BaseMinor:     1,
		Currency:      "EUR",
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown opportunity: want ErrNotFound, got %v", err)
	}
}
