//go:build integration

package store

import (
	"context"
	"testing"
)

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
