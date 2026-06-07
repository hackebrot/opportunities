//go:build integration

package store

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/hackebrot/opportunities/internal/model"
)

func TestIntegrationApplicationsCRUD(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store := startPostgresStore(ctx, t)
	if err := store.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	oppID := seedOpportunity(ctx, t, store, "Acme Corp", "acmecorp")
	appliedAt := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	params := ApplicationParams{
		OpportunityID:    oppID,
		AppliedAt:        &appliedAt,
		AppliedWithEmail: "me@example.test",
		Notes:            "applied via careers page",
	}

	created, err := store.InsertApplication(ctx, store.Pool, params, "applied")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("create: empty id")
	}
	want := model.Application{
		ID:               created.ID,
		OpportunityID:    oppID,
		AppliedAt:        &appliedAt,
		AppliedWithEmail: "me@example.test",
		Status:           "applied",
		Notes:            "applied via careers page",
		CreatedAt:        created.CreatedAt,
		UpdatedAt:        created.UpdatedAt,
	}
	if !cmp.Equal(want, created) {
		t.Fatalf("create row (-want +got):\n%s", cmp.Diff(want, created))
	}

	got, err := store.GetApplication(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !cmp.Equal(created, got) {
		t.Fatalf("get round-trip (-want +got):\n%s", cmp.Diff(created, got))
	}

	// Second application on a second opportunity to exercise List ordering
	// (most-recently-created first) and a nullable applied_at.
	secondOppID := seedOpportunity(ctx, t, store, "Example Corp", "examplecorp")
	second, err := store.InsertApplication(ctx, store.Pool, ApplicationParams{
		OpportunityID: secondOppID,
	}, "applied")
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if second.AppliedAt != nil {
		t.Fatalf("second.AppliedAt: want nil, got %v", *second.AppliedAt)
	}

	list, err := store.ListApplications(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// Match ListApplications' ORDER BY created_at DESC, id so the
	// expectation survives a microsecond-precision tie between the two
	// inserts.
	wantList := []model.Application{second, created}
	sort.SliceStable(wantList, func(i, j int) bool {
		if !wantList[i].CreatedAt.Equal(wantList[j].CreatedAt) {
			return wantList[i].CreatedAt.After(wantList[j].CreatedAt)
		}
		return wantList[i].ID < wantList[j].ID
	})
	if !cmp.Equal(wantList, list) {
		t.Fatalf("list (-want +got):\n%s", cmp.Diff(wantList, list))
	}

	upd := params
	upd.AppliedWithEmail = "other@example.test"
	upd.Notes = "moved to the referral track"
	updated, err := store.UpdateApplication(ctx, created.ID, upd)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.AppliedWithEmail != "other@example.test" || updated.Notes != "moved to the referral track" {
		t.Fatalf("update: fields not written: %+v", updated)
	}
	if updated.Status != "applied" {
		t.Fatalf("update: status changed to %q", updated.Status)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("update: updated_at not bumped")
	}

	if err := store.DeleteApplication(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.GetApplication(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete: want ErrNotFound, got %v", err)
	}
	if err := store.DeleteApplication(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: want ErrNotFound, got %v", err)
	}
}

// TestIntegrationApplicationsUnknownOpportunity proves a bad opportunity
// FK is translated to ErrNotFound.
func TestIntegrationApplicationsUnknownOpportunity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store := startPostgresStore(ctx, t)
	if err := store.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	_, err := store.InsertApplication(ctx, store.Pool, ApplicationParams{
		OpportunityID: "00000000-0000-0000-0000-000000000000",
	}, "applied")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown opportunity: want ErrNotFound, got %v", err)
	}
}

// TestIntegrationApplicationsActiveSlot proves the partial unique index
// translates to ErrActiveExists on a second active insert.
func TestIntegrationApplicationsActiveSlot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store := startPostgresStore(ctx, t)
	if err := store.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	oppID := seedOpportunity(ctx, t, store, "Acme Corp", "acmecorp")
	if _, err := store.InsertApplication(ctx, store.Pool,
		ApplicationParams{OpportunityID: oppID}, "applied"); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	_, err := store.InsertApplication(ctx, store.Pool,
		ApplicationParams{OpportunityID: oppID}, "applied")
	if !errors.Is(err, ErrActiveExists) {
		t.Fatalf("second insert: want ErrActiveExists, got %v", err)
	}
}

// TestIntegrationSetApplicationFollowUp proves the follow-up writer
// honors nil-as-"leave alone" semantics for both columns and bumps
// updated_at. Three calls exercise the three combinations the service
// layer drives: stamp-only, block-only, and the done combo.
func TestIntegrationSetApplicationFollowUp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store := startPostgresStore(ctx, t)
	if err := store.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	oppID := seedOpportunity(ctx, t, store, "Acme Corp", "acmecorp")
	app, err := store.InsertApplication(ctx, store.Pool,
		ApplicationParams{OpportunityID: oppID}, "applied")
	if err != nil {
		t.Fatalf("insert application: %v", err)
	}
	if app.FollowUpBlocked {
		t.Fatalf("fresh application FollowUpBlocked = true, want false")
	}
	if app.LastFollowedUpAt != nil {
		t.Fatalf("fresh application LastFollowedUpAt = %v, want nil", app.LastFollowedUpAt)
	}

	// Stamp-only: writes last_followed_up_at, leaves block at false.
	// Also asserts the RETURNING row matches a subsequent GetApplication
	// — once is enough; the other cases below just check the persisted
	// state via the returned row directly.
	stamp := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	got, err := store.SetApplicationFollowUp(ctx, store.Pool, app.ID, &stamp, nil)
	if err != nil {
		t.Fatalf("set stamp-only: %v", err)
	}
	if got.LastFollowedUpAt == nil || !got.LastFollowedUpAt.Equal(stamp) {
		t.Fatalf("LastFollowedUpAt after stamp = %v, want %v", got.LastFollowedUpAt, stamp)
	}
	if got.FollowUpBlocked {
		t.Fatalf("FollowUpBlocked after stamp = true, want false (stamp leaves block alone)")
	}
	if !got.UpdatedAt.After(app.UpdatedAt) {
		t.Fatalf("updated_at not bumped by stamp")
	}
	reread, err := store.GetApplication(ctx, app.ID)
	if err != nil {
		t.Fatalf("get after stamp: %v", err)
	}
	if !cmp.Equal(got, reread) {
		t.Fatalf("RETURNING vs SELECT (-returning +select):\n%s", cmp.Diff(got, reread))
	}

	// Block-only: leaves last_followed_up_at intact, sets the block.
	blocked := true
	got, err = store.SetApplicationFollowUp(ctx, store.Pool, app.ID, nil, &blocked)
	if err != nil {
		t.Fatalf("set block-only: %v", err)
	}
	if !got.FollowUpBlocked {
		t.Fatalf("FollowUpBlocked after block = false, want true")
	}
	if got.LastFollowedUpAt == nil || !got.LastFollowedUpAt.Equal(stamp) {
		t.Fatalf("LastFollowedUpAt after block = %v, want %v (block leaves stamp alone)", got.LastFollowedUpAt, stamp)
	}

	// Done combo: clear the block and restamp in one call.
	stamp2 := stamp.Add(48 * time.Hour)
	blocked2 := false
	got, err = store.SetApplicationFollowUp(ctx, store.Pool, app.ID, &stamp2, &blocked2)
	if err != nil {
		t.Fatalf("set done: %v", err)
	}
	if got.FollowUpBlocked {
		t.Fatalf("FollowUpBlocked after done = true, want false")
	}
	if got.LastFollowedUpAt == nil || !got.LastFollowedUpAt.Equal(stamp2) {
		t.Fatalf("LastFollowedUpAt after done = %v, want %v", got.LastFollowedUpAt, stamp2)
	}

	// Missing id surfaces as ErrNotFound.
	if _, err := store.SetApplicationFollowUp(ctx, store.Pool,
		"00000000-0000-0000-0000-000000000000", &stamp, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing id: want ErrNotFound, got %v", err)
	}
}

// TestIntegrationApplicationsActiveSlotRace proves the partial unique
// index serializes concurrent inserts: with two goroutines racing to
// open an application on the same opportunity, exactly one wins, the
// other gets ErrActiveExists. The service-layer "any active app?" check
// is best-effort; the index is the authority.
func TestIntegrationApplicationsActiveSlotRace(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store := startPostgresStore(ctx, t)
	if err := store.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	oppID := seedOpportunity(ctx, t, store, "Acme Corp", "acmecorp")

	var (
		wg      sync.WaitGroup
		results = make([]error, 2)
	)
	start := make(chan struct{})
	for i := range results {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			_, err := store.InsertApplication(ctx, store.Pool,
				ApplicationParams{OpportunityID: oppID}, "applied")
			results[idx] = err
		}(i)
	}
	close(start)
	wg.Wait()

	var wins, losses int
	for _, err := range results {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, ErrActiveExists):
			losses++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if wins != 1 || losses != 1 {
		t.Fatalf("race: wins=%d losses=%d, want 1/1 (results=%v)", wins, losses, results)
	}
}
