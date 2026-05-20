package service

import (
	"errors"

	"github.com/hackebrot/opportunities/internal/store"
)

// ErrValidation is returned when caller-supplied input fails service-layer
// validation (e.g. empty name, slug that reduces to the empty string).
// Wrapped with %w so callers can errors.Is(err, ErrValidation).
var ErrValidation = errors.New("service: invalid input")

// Service wraps the persistence layer with business logic. A nil pointer
// is not valid; construct with New.
type Service struct {
	store *store.Store
}

// New returns a Service backed by s.
func New(s *store.Store) *Service {
	return &Service{store: s}
}
