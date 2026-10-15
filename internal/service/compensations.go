package service

import (
	"context"

	"github.com/hackebrot/opportunities/internal/model"
)

// ListOpportunityCompensations returns the compensation rows linked to an
// opportunity (target/listed figures), for `opps opportunity show`.
//
// This is a thin read-through: it adds no logic over the store call. The
// read still routes through the service so the CLI never touches the store
// directly, honoring the architecture's Boundaries rule.
func (s *Service) ListOpportunityCompensations(ctx context.Context, oppID string) ([]model.Compensation, error) {
	return s.store.ListOpportunityCompensations(ctx, oppID)
}

// ListApplicationCompensations returns the compensation rows linked to an
// application (offered/counter/accepted figures), for `opps application
// show`. Thin read-through; see ListOpportunityCompensations.
func (s *Service) ListApplicationCompensations(ctx context.Context, appID string) ([]model.Compensation, error) {
	return s.store.ListApplicationCompensations(ctx, appID)
}
