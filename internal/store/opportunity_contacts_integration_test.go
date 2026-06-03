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

// TestIntegrationOpportunityContactsAttachDetachList exercises the full
// attach/detach/list surface against a real Postgres. The PK is the
// triple (opportunity_id, contact_id, relationship), so the same contact
// must be attachable twice under different relationships and detachable
// one row at a time.
func TestIntegrationOpportunityContactsAttachDetachList(t *testing.T) {
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
		t.Fatalf("seed opportunity: %v", err)
	}
	alice, err := store.CreateContact(ctx, store.Pool, ContactParams{
		Name: "Alice Example", CompanyID: &companyID,
	})
	if err != nil {
		t.Fatalf("seed contact: %v", err)
	}
	bob, err := store.CreateContact(ctx, store.Pool, ContactParams{
		Name: "Bob Example", CompanyID: &companyID,
	})
	if err != nil {
		t.Fatalf("seed contact: %v", err)
	}

	// Attach the same contact under two relationships, plus a second
	// contact, to exercise the composite PK.
	if err := store.AttachOpportunityContact(ctx, store.Pool, opp.ID, alice.ID, "recruiter"); err != nil {
		t.Fatalf("attach alice recruiter: %v", err)
	}
	if err := store.AttachOpportunityContact(ctx, store.Pool, opp.ID, alice.ID, "interviewer"); err != nil {
		t.Fatalf("attach alice interviewer: %v", err)
	}
	if err := store.AttachOpportunityContact(ctx, store.Pool, opp.ID, bob.ID, "hiring_manager"); err != nil {
		t.Fatalf("attach bob hiring_manager: %v", err)
	}

	rows, err := store.ListOpportunityContacts(ctx, opp.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("list len = %d, want 3: %+v", len(rows), rows)
	}
	for _, r := range rows {
		if r.OpportunityID != opp.ID {
			t.Fatalf("row.OpportunityID = %q, want %q", r.OpportunityID, opp.ID)
		}
		if r.CreatedAt.IsZero() {
			t.Fatalf("row.CreatedAt zero: %+v", r)
		}
	}
	// Sort is deterministic: contact name (case-insensitive) then
	// relationship — so alice/interviewer, alice/recruiter, bob/hiring_manager.
	want := []struct{ Name, Rel string }{
		{"Alice Example", "interviewer"},
		{"Alice Example", "recruiter"},
		{"Bob Example", "hiring_manager"},
	}
	for i, w := range want {
		if rows[i].ContactName != w.Name || rows[i].Relationship != w.Rel {
			t.Fatalf("row[%d] = (%q, %q), want (%q, %q)", i, rows[i].ContactName, rows[i].Relationship, w.Name, w.Rel)
		}
	}

	// Duplicate attach (same triple) is ErrConflict.
	if err := store.AttachOpportunityContact(ctx, store.Pool, opp.ID, alice.ID, "recruiter"); !errors.Is(err, ErrConflict) {
		t.Fatalf("dup attach: want ErrConflict, got %v", err)
	}

	// Detaching the recruiter relationship leaves the interviewer row.
	if err := store.DetachOpportunityContact(ctx, store.Pool, opp.ID, alice.ID, "recruiter"); err != nil {
		t.Fatalf("detach alice recruiter: %v", err)
	}
	rows, err = store.ListOpportunityContacts(ctx, opp.ID)
	if err != nil {
		t.Fatalf("list after detach: %v", err)
	}
	wantAfter := []model.OpportunityContact{
		{OpportunityID: opp.ID, ContactID: alice.ID, ContactName: "Alice Example", Relationship: "interviewer", CreatedAt: rows[0].CreatedAt},
		{OpportunityID: opp.ID, ContactID: bob.ID, ContactName: "Bob Example", Relationship: "hiring_manager", CreatedAt: rows[1].CreatedAt},
	}
	if !cmp.Equal(wantAfter, rows) {
		t.Fatalf("rows after detach (-want +got):\n%s", cmp.Diff(wantAfter, rows))
	}

	// Detaching a missing triple is ErrNotFound.
	if err := store.DetachOpportunityContact(ctx, store.Pool, opp.ID, alice.ID, "recruiter"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("detach missing: want ErrNotFound, got %v", err)
	}

	// Detach with an unknown opportunity id is also ErrNotFound — no row
	// matched, regardless of why.
	const missingOpp = "00000000-0000-0000-0000-000000000000"
	if err := store.DetachOpportunityContact(ctx, store.Pool, missingOpp, alice.ID, "interviewer"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("detach unknown opp: want ErrNotFound, got %v", err)
	}

	// Listing under an unknown opportunity returns no rows, no error —
	// matches the empty-list contract of List* across the package.
	if rows, err := store.ListOpportunityContacts(ctx, missingOpp); err != nil || len(rows) != 0 {
		t.Fatalf("list missing opp: rows=%v err=%v", rows, err)
	}
}
