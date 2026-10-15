package service

import (
	"context"

	"github.com/hackebrot/opportunities/internal/model"
)

// ListApplicationStages returns the interview-pipeline stages of an
// application in display order, for `opps application show`.
//
// This is a thin read-through: it adds no logic over the store call. The
// read still routes through the service so the CLI never touches the store
// directly, honoring the architecture's Boundaries rule.
func (s *Service) ListApplicationStages(ctx context.Context, appID string) ([]model.ApplicationStage, error) {
	return s.store.ListApplicationStages(ctx, appID)
}
