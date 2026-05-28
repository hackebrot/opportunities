package service

import (
	"errors"
	"testing"
)

func TestOpportunityInputNormalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		in            OpportunityInput
		wantRoleTitle *string
		wantPriority  string
		wantErr       bool
	}{
		{
			name:         "valid minimal defaults priority to normal",
			in:           OpportunityInput{CompanyID: "c1", Source: "outbound"},
			wantPriority: "normal",
		},
		{
			name: "valid full",
			in: OpportunityInput{
				CompanyID:         "c1",
				RoleTitle:         "  Staff Engineer  ",
				Location:          "Berlin",
				OfficeDaysPerWeek: 3,
				Source:            "referral",
				SourceDetail:      "former colleague",
				Priority:          "high",
				Notes:             "promising",
			},
			wantRoleTitle: new("Staff Engineer"),
			wantPriority:  "high",
		},
		{
			name:          "blank role title normalizes to nil",
			in:            OpportunityInput{CompanyID: "c1", Source: "outbound", RoleTitle: "   "},
			wantRoleTitle: nil,
			wantPriority:  "normal",
		},
		{"missing company id", OpportunityInput{Source: "outbound"}, nil, "", true},
		{"whitespace company id", OpportunityInput{CompanyID: "  ", Source: "outbound"}, nil, "", true},
		{"missing source", OpportunityInput{CompanyID: "c1"}, nil, "", true},
		{"invalid source", OpportunityInput{CompanyID: "c1", Source: "carrier pigeon"}, nil, "", true},
		{"invalid priority", OpportunityInput{CompanyID: "c1", Source: "outbound", Priority: "urgent"}, nil, "", true},
		{"office days too high", OpportunityInput{CompanyID: "c1", Source: "outbound", OfficeDaysPerWeek: 6}, nil, "", true},
		{"office days negative", OpportunityInput{CompanyID: "c1", Source: "outbound", OfficeDaysPerWeek: -1}, nil, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p, err := tt.in.normalize()
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
			if tt.wantRoleTitle == nil {
				if p.RoleTitle != nil {
					t.Fatalf("RoleTitle = %q, want nil", *p.RoleTitle)
				}
			} else {
				if p.RoleTitle == nil || *p.RoleTitle != *tt.wantRoleTitle {
					t.Fatalf("RoleTitle = %v, want %q", p.RoleTitle, *tt.wantRoleTitle)
				}
			}
			if p.Priority != tt.wantPriority {
				t.Fatalf("Priority = %q, want %q", p.Priority, tt.wantPriority)
			}
		})
	}
}
