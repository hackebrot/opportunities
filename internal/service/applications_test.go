package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestApplicationInputNormalize(t *testing.T) {
	t.Parallel()

	oppID := "00000000-0000-0000-0000-000000000001"
	appliedAt := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		in        ApplicationInput
		wantOppID string
		wantErr   bool
	}{
		{
			name:      "valid minimal",
			in:        ApplicationInput{OpportunityID: oppID},
			wantOppID: oppID,
		},
		{
			name:      "trims opportunity id",
			in:        ApplicationInput{OpportunityID: "  " + oppID + "  "},
			wantOppID: oppID,
		},
		{
			name: "carries optional fields",
			in: ApplicationInput{
				OpportunityID:    oppID,
				AppliedAt:        &appliedAt,
				AppliedWithEmail: "me@example.test",
				Notes:            "applied via careers page",
			},
			wantOppID: oppID,
		},
		{"empty opportunity id", ApplicationInput{}, "", true},
		{"whitespace opportunity id", ApplicationInput{OpportunityID: "   "}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			params, err := tt.in.normalize()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalize(%+v): want error", tt.in)
				}
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("normalize(%+v): err=%v, want ErrValidation", tt.in, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalize(%+v): unexpected error: %v", tt.in, err)
			}
			if params.OpportunityID != tt.wantOppID {
				t.Fatalf("normalize OpportunityID = %q, want %q", params.OpportunityID, tt.wantOppID)
			}
			if tt.in.AppliedAt != nil && (params.AppliedAt == nil || !params.AppliedAt.Equal(*tt.in.AppliedAt)) {
				t.Fatalf("normalize AppliedAt = %v, want %v", params.AppliedAt, tt.in.AppliedAt)
			}
			if params.AppliedWithEmail != tt.in.AppliedWithEmail {
				t.Fatalf("normalize AppliedWithEmail = %q, want %q", params.AppliedWithEmail, tt.in.AppliedWithEmail)
			}
			if params.Notes != tt.in.Notes {
				t.Fatalf("normalize Notes = %q, want %q", params.Notes, tt.in.Notes)
			}
		})
	}
}

// TestFollowUpApplicationModeValidation pins the pre-DB validation:
// unknown modes are rejected as ErrValidation, missing ids the same.
// The DB-touching cases are covered by the integration tests because
// FollowUpApplication queries the application status before deciding
// what to write.
func TestFollowUpApplicationModeValidation(t *testing.T) {
	t.Parallel()

	svc := &Service{}
	tests := []struct {
		name string
		id   string
		mode FollowUpMode
	}{
		{"zero mode", "id", FollowUpMode(0)},
		{"bogus mode", "id", FollowUpMode(99)},
		{"blank id", "   ", FollowUpStamp},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := svc.FollowUpApplication(context.Background(), tt.id, tt.mode); !errors.Is(err, ErrValidation) {
				t.Fatalf("err=%v, want ErrValidation", err)
			}
		})
	}
}
