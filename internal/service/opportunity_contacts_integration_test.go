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

// TestIntegrationOpportunityContactsAttachDetachList exercises the
// secondary attach/detach path end-to-end through the service layer.
// The PK is (opportunity_id, contact_id, relationship), so the same
// contact may be attached under multiple relationships and detached one
// row at a time.
func TestIntegrationOpportunityContactsAttachDetachList(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)

	company, err := svc.CreateCompany(ctx, service.CompanyInput{Name: "Acme Corp"})
	if err != nil {
		t.Fatalf("create company: %v", err)
	}
	opp, err := svc.AddOpportunity(ctx, service.OpportunityCreationInput{
		Company:     service.OpportunityCompanyChoice{ID: company.ID},
		Opportunity: service.OpportunityInput{Source: "outbound"},
	})
	if err != nil {
		t.Fatalf("add opportunity: %v", err)
	}
	alice, err := svc.CreateContact(ctx, service.ContactInput{Name: "Alice Example", CompanyID: &company.ID})
	if err != nil {
		t.Fatalf("create alice: %v", err)
	}

	// Empty before any attach.
	rows, err := svc.ListOpportunityContacts(ctx, opp.ID)
	if err != nil {
		t.Fatalf("list pre-attach: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("list pre-attach = %d, want 0", len(rows))
	}

	if err := svc.AttachOpportunityContact(ctx, opp.ID, alice.ID, "recruiter"); err != nil {
		t.Fatalf("attach recruiter: %v", err)
	}
	if err := svc.AttachOpportunityContact(ctx, opp.ID, alice.ID, "interviewer"); err != nil {
		t.Fatalf("attach interviewer: %v", err)
	}

	rows, err = svc.ListOpportunityContacts(ctx, opp.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("list = %d rows, want 2: %+v", len(rows), rows)
	}

	// Detach the recruiter row, leave interviewer.
	if err := svc.DetachOpportunityContact(ctx, opp.ID, alice.ID, "recruiter"); err != nil {
		t.Fatalf("detach recruiter: %v", err)
	}
	rows, err = svc.ListOpportunityContacts(ctx, opp.ID)
	if err != nil {
		t.Fatalf("list after detach: %v", err)
	}
	if len(rows) != 1 || rows[0].Relationship != "interviewer" {
		t.Fatalf("after detach = %+v, want one interviewer row", rows)
	}

	// Detaching a missing row is ErrNotFound; the same is true for attach
	// with an unknown opportunity.
	if err := svc.DetachOpportunityContact(ctx, opp.ID, alice.ID, "recruiter"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("detach already-detached: want ErrNotFound, got %v", err)
	}
	const missing = "00000000-0000-0000-0000-000000000000"
	if err := svc.AttachOpportunityContact(ctx, missing, alice.ID, "recruiter"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("attach unknown opp: want ErrNotFound, got %v", err)
	}

	// Unknown relationship is service-layer ErrValidation.
	if err := svc.AttachOpportunityContact(ctx, opp.ID, alice.ID, "mentor"); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("attach unknown relationship: want ErrValidation, got %v", err)
	}
	if err := svc.DetachOpportunityContact(ctx, opp.ID, alice.ID, ""); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("detach empty relationship: want ErrValidation, got %v", err)
	}
}
