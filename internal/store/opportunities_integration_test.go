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

// seedCompany inserts a company and returns its id, for opportunity FKs.
func seedCompany(ctx context.Context, t *testing.T, s *Store, name, slug string) string {
	t.Helper()
	c, err := s.CreateCompany(ctx, CompanyParams{Name: name, Slug: slug})
	if err != nil {
		t.Fatalf("seed company: %v", err)
	}
	return c.ID
}

func TestIntegrationOpportunitiesCRUD(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store := startPostgresStore(ctx, t)
	if err := store.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	companyID := seedCompany(ctx, t, store, "Acme Corp", "acmecorp")

	role := "Member of Technical Staff"
	params := OpportunityParams{
		CompanyID:         companyID,
		RoleTitle:         &role,
		Location:          "Berlin",
		OfficeDaysPerWeek: 3,
		Source:            "referral",
		SourceDetail:      "former colleague",
		Priority:          "high",
		Notes:             "promising lead",
	}

	created, err := store.InsertOpportunity(ctx, store.Pool, params, "watching")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("create: empty id")
	}
	want := model.Opportunity{
		ID:                created.ID,
		CompanyID:         companyID,
		CompanyName:       "Acme Corp",
		RoleTitle:         &role,
		Location:          "Berlin",
		OfficeDaysPerWeek: 3,
		Source:            "referral",
		SourceDetail:      "former colleague",
		Priority:          "high",
		LatestStatus:      "watching",
		Notes:             "promising lead",
		CreatedAt:         created.CreatedAt,
		UpdatedAt:         created.UpdatedAt,
	}
	if diff := cmp.Diff(want, created); diff != "" {
		t.Fatalf("create row (-want +got):\n%s", diff)
	}

	got, err := store.GetOpportunity(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if diff := cmp.Diff(created, got); diff != "" {
		t.Fatalf("get round-trip (-want +got):\n%s", diff)
	}

	// An opportunity with no known role yet: role_title stays null.
	second, err := store.InsertOpportunity(ctx, store.Pool, OpportunityParams{
		CompanyID: companyID,
		Source:    "inbound_founder",
		Priority:  "normal",
	}, "watching")
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if second.RoleTitle != nil {
		t.Fatalf("second.RoleTitle: want nil, got %q", *second.RoleTitle)
	}

	list, err := store.ListOpportunities(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// Ordered most-recently-created first.
	wantList := []model.Opportunity{second, created}
	if diff := cmp.Diff(wantList, list); diff != "" {
		t.Fatalf("list (-want +got):\n%s", diff)
	}

	// Update edits the editable fields and bumps updated_at, leaving
	// latest_status untouched.
	upd := params
	upd.RoleTitle = nil
	upd.OfficeDaysPerWeek = 0 // full remote
	upd.Priority = "low"
	updated, err := store.UpdateOpportunity(ctx, created.ID, upd)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.RoleTitle != nil || updated.OfficeDaysPerWeek != 0 || updated.Priority != "low" {
		t.Fatalf("update: fields not written: %+v", updated)
	}
	if updated.LatestStatus != "watching" {
		t.Fatalf("update: latest_status changed to %q", updated.LatestStatus)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("update: updated_at not bumped")
	}

	if err := store.DeleteOpportunity(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.GetOpportunity(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete: want ErrNotFound, got %v", err)
	}
	if err := store.DeleteOpportunity(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: want ErrNotFound, got %v", err)
	}
}

func TestIntegrationOpportunitySetLatestStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store := startPostgresStore(ctx, t)
	if err := store.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	companyID := seedCompany(ctx, t, store, "Acme Corp", "acmecorp")
	opp, err := store.InsertOpportunity(ctx, store.Pool, OpportunityParams{
		CompanyID: companyID, Source: "outbound", Priority: "normal",
	}, "watching")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Active transition: no archived_at.
	if err := store.SetLatestStatus(ctx, opp.ID, "exploring", nil); err != nil {
		t.Fatalf("set exploring: %v", err)
	}
	got, err := store.GetOpportunity(ctx, opp.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.LatestStatus != "exploring" || got.ArchivedAt != nil {
		t.Fatalf("after exploring: status=%q archived=%v", got.LatestStatus, got.ArchivedAt)
	}

	// Archive transition: archived_at set.
	archivedAt := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	if err := store.SetLatestStatus(ctx, opp.ID, "archived", &archivedAt); err != nil {
		t.Fatalf("set archived: %v", err)
	}
	got, err = store.GetOpportunity(ctx, opp.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.LatestStatus != "archived" || got.ArchivedAt == nil || !got.ArchivedAt.Equal(archivedAt) {
		t.Fatalf("after archived: status=%q archived=%v", got.LatestStatus, got.ArchivedAt)
	}

	// Missing id is ErrNotFound.
	if err := store.SetLatestStatus(ctx, "00000000-0000-0000-0000-000000000000", "watching", nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("set missing: want ErrNotFound, got %v", err)
	}
}

// TestIntegrationOpportunityEventTxRollback proves the seam that
// service.AddOpportunity relies on: an opportunity and an event inserted
// through a Querier participate in the caller's transaction, so a failure
// on the second insert rolls the first one back. A bogus event kind trips
// the events_kind_chk CHECK; after rollback the opportunity is gone.
func TestIntegrationOpportunityEventTxRollback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store := startPostgresStore(ctx, t)
	if err := store.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	companyID := seedCompany(ctx, t, store, "Acme Corp", "acmecorp")

	tx, err := store.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	opp, err := store.InsertOpportunity(ctx, tx, OpportunityParams{
		CompanyID: companyID, Source: "outbound", Priority: "normal",
	}, "watching")
	if err != nil {
		t.Fatalf("insert opportunity: %v", err)
	}

	_, err = store.InsertEvent(ctx, tx, EventParams{
		OpportunityID: opp.ID,
		Kind:          "bogus_kind", // not in events_kind_chk → CHECK violation
		OccurredAt:    time.Now(),
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("insert bogus event: want ErrConflict, got %v", err)
	}

	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	if _, err := store.GetOpportunity(ctx, opp.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("opportunity after rollback: want ErrNotFound, got %v", err)
	}
}

// TestIntegrationInsertEventCrossOpportunityApplication proves the
// composite FK is reported as a relationship conflict, not a missing row:
// an event for opportunity B that references an application belonging to
// opportunity A trips events_application_belongs_to_opportunity_fk, which
// must surface as ErrConflict (the opportunity_id itself exists).
func TestIntegrationInsertEventCrossOpportunityApplication(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store := startPostgresStore(ctx, t)
	if err := store.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	companyID := seedCompany(ctx, t, store, "Acme Corp", "acmecorp")

	oppA, err := store.InsertOpportunity(ctx, store.Pool, OpportunityParams{
		CompanyID: companyID, Source: "outbound", Priority: "normal",
	}, "watching")
	if err != nil {
		t.Fatalf("insert opportunity A: %v", err)
	}
	oppB, err := store.InsertOpportunity(ctx, store.Pool, OpportunityParams{
		CompanyID: companyID, Source: "outbound", Priority: "normal",
	}, "watching")
	if err != nil {
		t.Fatalf("insert opportunity B: %v", err)
	}

	// Application that belongs to opportunity A. Inserted directly: there
	// is no application store layer yet.
	var appID string
	if err := store.Pool.QueryRow(
		ctx,
		`INSERT INTO applications (opportunity_id, status) VALUES ($1, 'applied') RETURNING id`,
		oppA.ID,
	).Scan(&appID); err != nil {
		t.Fatalf("seed application: %v", err)
	}

	// Event for opportunity B referencing A's application: the composite FK
	// rejects it. opportunity_id is valid, so this is a conflict, not a
	// missing row.
	_, err = store.InsertEvent(ctx, store.Pool, EventParams{
		OpportunityID: oppB.ID,
		ApplicationID: &appID,
		Kind:          "applied",
		OccurredAt:    time.Now(),
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("cross-opportunity application event: want ErrConflict, got %v", err)
	}
	if errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-opportunity application event: misclassified as ErrNotFound: %v", err)
	}
}
