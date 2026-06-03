package service

import (
	"errors"
	"testing"
)

// TestAttachOpportunityContactValidation locks in the service-layer
// guard: an unknown relationship must be ErrValidation, not pass through
// to a CHECK violation at the store. Same contract as the inline-create
// flow in AddOpportunity.
func TestAttachOpportunityContactValidation(t *testing.T) {
	t.Parallel()

	// A zero *Service is intentional: validation runs before the store,
	// so the nil pool is never dialed. If validation ever regresses to
	// "reach the store first," this nil-dereferences loudly.
	var s Service

	tests := []struct {
		name         string
		oppID        string
		contactID    string
		relationship string
		wantErr      bool
	}{
		{"valid", "opp", "contact", "recruiter", false},
		{"empty opportunity id", "", "contact", "recruiter", true},
		{"empty contact id", "opp", "", "recruiter", true},
		{"unknown relationship", "opp", "contact", "mentor", true},
		{"empty relationship", "opp", "contact", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := s.validateOpportunityContact(tt.oppID, tt.contactID, tt.relationship)
			if tt.wantErr {
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("err = %v, want ErrValidation", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
