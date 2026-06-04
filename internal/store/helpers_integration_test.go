//go:build integration

package store

import (
	"context"
	"testing"

	"github.com/hackebrot/opportunities/internal/testutil"
)

// startPostgresStore opens a *Store against an ephemeral Postgres
// container (see testutil.StartPostgres). The container and pool are
// released on test cleanup.
func startPostgresStore(ctx context.Context, t *testing.T) *Store {
	t.Helper()

	s, err := Open(ctx, testutil.StartPostgres(ctx, t))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(s.Close)

	if err := s.Pool.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return s
}

// seedCompany inserts a company and returns its id, for opportunity FKs.
func seedCompany(ctx context.Context, t *testing.T, s *Store, name, slug string) string {
	t.Helper()
	c, err := s.CreateCompany(ctx, s.Pool, CompanyParams{Name: name, Slug: slug})
	if err != nil {
		t.Fatalf("seed company: %v", err)
	}
	return c.ID
}

// seedOpportunity is the boilerplate every test that needs an existing
// opportunity reaches for: a fresh company plus a freshly inserted
// opportunity (latest_status "watching", no events).
func seedOpportunity(ctx context.Context, t *testing.T, s *Store, name, slug string) string {
	t.Helper()
	companyID := seedCompany(ctx, t, s, name, slug)
	opp, err := s.InsertOpportunity(ctx, s.Pool, OpportunityParams{
		CompanyID: companyID, Source: "outbound", Priority: "normal",
	}, "watching")
	if err != nil {
		t.Fatalf("seed opportunity: %v", err)
	}
	return opp.ID
}
