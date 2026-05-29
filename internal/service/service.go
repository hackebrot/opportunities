package service

import (
	"errors"
	"time"

	"github.com/hackebrot/opportunities/internal/store"
)

// ErrValidation is returned when caller-supplied input fails service-layer
// validation (e.g. empty name, slug that reduces to the empty string).
// Wrapped with %w so callers can errors.Is(err, ErrValidation).
var ErrValidation = errors.New("service: invalid input")

// Clock supplies the current time to business logic. The spec forbids
// time.Now() in the service layer so event timestamps can be pinned in
// tests; production seeds a real clock from cmd/opps.
type Clock interface {
	Now() time.Time
}

// Service wraps the persistence layer with business logic. Construct
// with New.
type Service struct {
	store *store.Store
	clock Clock
}

// New returns a Service backed by s and clock. Panics if either is nil —
// passing nil is a programmer error caught at startup, not a runtime
// condition.
func New(s *store.Store, clock Clock) *Service {
	if s == nil {
		panic("service: New called with nil *store.Store")
	}
	if clock == nil {
		panic("service: New called with nil Clock")
	}
	return &Service{store: s, clock: clock}
}
