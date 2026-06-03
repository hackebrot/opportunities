//go:build integration

package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/hackebrot/opportunities/internal/service"
	"github.com/hackebrot/opportunities/internal/store"
	"github.com/hackebrot/opportunities/internal/testutil"
)

// fixedClock is a service.Clock that always returns the same instant, so
// tests can assert on event timestamps deterministically.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// testClock is a fixed instant used across the service integration tests.
var testClock = fixedClock{t: time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)}

// startPostgresStore opens a *store.Store against an ephemeral Postgres
// container (see testutil.StartPostgres). Released on test cleanup.
func startPostgresStore(ctx context.Context, t *testing.T) *store.Store {
	t.Helper()

	s, err := store.Open(ctx, testutil.StartPostgres(ctx, t))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(s.Close)

	if err := s.Pool.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return s
}

// seedOpportunity is the boilerplate every test that needs an existing
// opportunity reaches for: a company and a freshly-added opportunity
// (latest_status "watching", one "added" event already written by
// AddOpportunity).
func seedOpportunity(ctx context.Context, t *testing.T, svc *service.Service) string {
	t.Helper()
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
	return opp.ID
}

// readOpportunityLatestStatus is the post-state probe used by every
// happy-path assertion that needs to verify a latest_status flip.
func readOpportunityLatestStatus(ctx context.Context, t *testing.T, st *store.Store, oppID string) string {
	t.Helper()
	opp, err := st.GetOpportunity(ctx, oppID)
	if err != nil {
		t.Fatalf("get opportunity: %v", err)
	}
	return opp.LatestStatus
}
