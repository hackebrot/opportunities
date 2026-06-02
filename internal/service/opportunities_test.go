package service

import (
	"errors"
	"testing"
)

func TestOpportunityCreationInputNormalize(t *testing.T) {
	t.Parallel()

	companyInput := &CompanyInput{Name: "Foo Corp"}
	contactInput := &ContactInput{Name: "Alice"}

	tests := []struct {
		name    string
		in      OpportunityCreationInput
		wantErr bool
	}{
		{
			name: "existing company only",
			in: OpportunityCreationInput{
				Company: OpportunityCompanyChoice{ID: "c1"},
			},
		},
		{
			name: "new company only",
			in: OpportunityCreationInput{
				Company: OpportunityCompanyChoice{New: companyInput},
			},
		},
		{
			name:    "neither company id nor new",
			in:      OpportunityCreationInput{},
			wantErr: true,
		},
		{
			name: "whitespace-only company id is treated as missing",
			in: OpportunityCreationInput{
				Company: OpportunityCompanyChoice{ID: "   "},
			},
			wantErr: true,
		},
		{
			name: "both company id and new",
			in: OpportunityCreationInput{
				Company: OpportunityCompanyChoice{ID: "c1", New: companyInput},
			},
			wantErr: true,
		},
		{
			name: "with existing contact",
			in: OpportunityCreationInput{
				Company: OpportunityCompanyChoice{ID: "c1"},
				Contact: &OpportunityContactChoice{ID: "k1", Relationship: "recruiter"},
			},
		},
		{
			name: "with new contact",
			in: OpportunityCreationInput{
				Company: OpportunityCompanyChoice{ID: "c1"},
				Contact: &OpportunityContactChoice{New: contactInput, Relationship: "hiring_manager"},
			},
		},
		{
			name: "contact with no id and no new",
			in: OpportunityCreationInput{
				Company: OpportunityCompanyChoice{ID: "c1"},
				Contact: &OpportunityContactChoice{Relationship: "recruiter"},
			},
			wantErr: true,
		},
		{
			name: "contact with both id and new",
			in: OpportunityCreationInput{
				Company: OpportunityCompanyChoice{ID: "c1"},
				Contact: &OpportunityContactChoice{ID: "k1", New: contactInput, Relationship: "recruiter"},
			},
			wantErr: true,
		},
		{
			name: "unknown relationship",
			in: OpportunityCreationInput{
				Company: OpportunityCompanyChoice{ID: "c1"},
				Contact: &OpportunityContactChoice{ID: "k1", Relationship: "mentor"},
			},
			wantErr: true,
		},
		{
			name: "empty relationship",
			in: OpportunityCreationInput{
				Company: OpportunityCompanyChoice{ID: "c1"},
				Contact: &OpportunityContactChoice{ID: "k1"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := tt.in.normalize()
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
		})
	}
}

// TestOpportunityCreationInputNormalizeTrimsAndPreservesCaller asserts
// the two contracts that distinguish normalize from a simple validate:
// the returned struct has trimmed IDs, and the caller's input — including
// a heap-allocated Contact — is left untouched.
func TestOpportunityCreationInputNormalizeTrimsAndPreservesCaller(t *testing.T) {
	t.Parallel()

	in := OpportunityCreationInput{
		Company: OpportunityCompanyChoice{ID: "  c1  "},
		Contact: &OpportunityContactChoice{ID: "  k1  ", Relationship: "recruiter"},
	}
	got, err := in.normalize()
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if got.Company.ID != "c1" {
		t.Fatalf("returned Company.ID = %q, want %q (trimmed)", got.Company.ID, "c1")
	}
	if got.Contact.ID != "k1" {
		t.Fatalf("returned Contact.ID = %q, want %q (trimmed)", got.Contact.ID, "k1")
	}
	if in.Company.ID != "  c1  " {
		t.Fatalf("caller Company.ID was mutated: got %q", in.Company.ID)
	}
	if in.Contact.ID != "  k1  " {
		t.Fatalf("caller Contact.ID was mutated through pointer: got %q", in.Contact.ID)
	}
}

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
