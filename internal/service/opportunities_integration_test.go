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

// TestIntegrationAddOpportunity proves AddOpportunity writes the
// opportunity (latest_status "watching") and an "added" event in one
// transaction: both rows exist afterwards, with the event timestamp pinned
// by the injected clock.
func TestIntegrationAddOpportunity(t *testing.T) {
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
		Company: service.OpportunityCompanyChoice{ID: company.ID},
		Opportunity: service.OpportunityInput{
			RoleTitle: "Member of Technical Staff",
			Location:  "Berlin",
			Source:    "referral",
		},
	})
	if err != nil {
		t.Fatalf("add opportunity: %v", err)
	}
	if opp.LatestStatus != "watching" {
		t.Fatalf("latest_status = %q, want watching", opp.LatestStatus)
	}
	if opp.CompanyName != "Acme Corp" {
		t.Fatalf("company name = %q, want Acme Corp", opp.CompanyName)
	}

	// Exactly one event, kind "added", at the clock instant.
	var (
		count      int
		kind       string
		occurredAt time.Time
		appID      *string
	)
	err = st.Pool.QueryRow(ctx, `
		SELECT count(*) OVER (), kind, occurred_at, application_id
		FROM events WHERE opportunity_id = $1`, opp.ID).
		Scan(&count, &kind, &occurredAt, &appID)
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	if count != 1 {
		t.Fatalf("event count = %d, want 1", count)
	}
	if kind != "added" {
		t.Fatalf("event kind = %q, want added", kind)
	}
	if appID != nil {
		t.Fatalf("event application_id = %q, want nil", *appID)
	}
	if !occurredAt.Equal(testClock.t) {
		t.Fatalf("event occurred_at = %s, want %s", occurredAt, testClock.t)
	}
}

// TestIntegrationAddOpportunityUnknownCompany proves a bad company FK is
// rejected and leaves no opportunity behind.
func TestIntegrationAddOpportunityUnknownCompany(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)

	_, err := svc.AddOpportunity(ctx, service.OpportunityCreationInput{
		Company:     service.OpportunityCompanyChoice{ID: "00000000-0000-0000-0000-000000000000"},
		Opportunity: service.OpportunityInput{Source: "outbound"},
	})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("add with unknown company: want store.ErrNotFound, got %v", err)
	}

	var n int
	if err := st.Pool.QueryRow(ctx, `SELECT count(*) FROM opportunities`).Scan(&n); err != nil {
		t.Fatalf("count opportunities: %v", err)
	}
	if n != 0 {
		t.Fatalf("opportunities after failed add = %d, want 0", n)
	}
}

// TestIntegrationAddOpportunityInlineGraph proves the inline flow lands
// a company, opportunity, "added" event, and one opportunity_contacts
// row in one transaction.
func TestIntegrationAddOpportunityInlineGraph(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)

	opp, err := svc.AddOpportunity(ctx, service.OpportunityCreationInput{
		Company: service.OpportunityCompanyChoice{
			New: &service.CompanyInput{Name: "Acme Corp"},
		},
		Opportunity: service.OpportunityInput{
			RoleTitle: "Member of Technical Staff",
			Source:    "inbound_founder",
		},
		Contact: &service.OpportunityContactChoice{
			New:          &service.ContactInput{Name: "Alice Example"},
			Relationship: "hiring_manager",
		},
	})
	if err != nil {
		t.Fatalf("AddOpportunity: %v", err)
	}
	if opp.CompanyName != "Acme Corp" {
		t.Fatalf("company name = %q, want Acme Corp", opp.CompanyName)
	}

	// One company, one contact (with company id set), one opp_contacts row.
	var n int
	if err := st.Pool.QueryRow(ctx, `SELECT count(*) FROM companies`).Scan(&n); err != nil {
		t.Fatalf("count companies: %v", err)
	}
	if n != 1 {
		t.Fatalf("companies = %d, want 1", n)
	}
	var contactCompany *string
	if err := st.Pool.QueryRow(ctx, `SELECT company_id FROM contacts`).Scan(&contactCompany); err != nil {
		t.Fatalf("contact: %v", err)
	}
	if contactCompany == nil || *contactCompany != opp.CompanyID {
		t.Fatalf("contact company_id = %v, want %s", contactCompany, opp.CompanyID)
	}
	var rel string
	if err := st.Pool.QueryRow(
		ctx,
		`SELECT relationship FROM opportunity_contacts WHERE opportunity_id = $1`, opp.ID,
	).Scan(&rel); err != nil {
		t.Fatalf("opp_contacts: %v", err)
	}
	if rel != "hiring_manager" {
		t.Fatalf("relationship = %q, want hiring_manager", rel)
	}
}

// TestIntegrationAddOpportunityRollsBackOnFailure proves the
// "inline-create transaction" risk mitigation: when a later step fails
// (here, an unknown existing contact id passed after the company was
// created in the same tx), the company row is rolled back and the
// caller does not end up with an orphan.
func TestIntegrationAddOpportunityRollsBackOnFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)

	_, err := svc.AddOpportunity(ctx, service.OpportunityCreationInput{
		Company: service.OpportunityCompanyChoice{
			New: &service.CompanyInput{Name: "Example Corp"},
		},
		Opportunity: service.OpportunityInput{Source: "outbound"},
		Contact: &service.OpportunityContactChoice{
			ID:           "00000000-0000-0000-0000-000000000000",
			Relationship: "recruiter",
		},
	})
	if err == nil {
		t.Fatal("expected error on dangling contact id")
	}

	for _, table := range []string{"companies", "opportunities", "events", "contacts", "opportunity_contacts"} {
		var rowCount int
		if err := st.Pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&rowCount); err != nil {
			t.Errorf("row count in %s: %v", table, err)
			continue
		}
		if rowCount != 0 {
			t.Errorf("row count in %s after rollback = %d, want 0", table, rowCount)
		}
	}
}

// TestIntegrationAddOpportunityValidation proves invalid input is rejected
// before any DB write.
func TestIntegrationAddOpportunityValidation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)

	if _, err := svc.AddOpportunity(ctx, service.OpportunityCreationInput{
		Opportunity: service.OpportunityInput{Source: "outbound"},
	}); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("missing company: want ErrValidation, got %v", err)
	}

	// Rejection must happen before any write: the table stays empty.
	var n int
	if err := st.Pool.QueryRow(ctx, `SELECT count(*) FROM opportunities`).Scan(&n); err != nil {
		t.Fatalf("count opportunities: %v", err)
	}
	if n != 0 {
		t.Fatalf("opportunities after validation failure = %d, want 0", n)
	}
}
