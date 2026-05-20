package service

import (
	"errors"

	"github.com/hackebrot/opportunities/internal/store"
)

// ErrValidation is returned when caller-supplied input fails service-layer
// validation (e.g. empty name, slug that reduces to the empty string).
// Wrapped with %w so callers can errors.Is(err, ErrValidation).
var ErrValidation = errors.New("service: invalid input")

// Service wraps the persistence layer with business logic. Construct
// with New.
type Service struct {
	store *store.Store
}

// New returns a Service backed by s. Panics if s is nil — passing nil
// is a programmer error caught at startup, not a runtime condition.
func New(s *store.Store) *Service {
	if s == nil {
		panic("service: New called with nil *store.Store")
	}
	return &Service{store: s}
}
