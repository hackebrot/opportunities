package service

import (
	"errors"
	"testing"
)

func TestContactNormalize(t *testing.T) {
	t.Parallel()

	companyID := "00000000-0000-0000-0000-000000000001"
	blank := "   "

	tests := []struct {
		name          string
		in            ContactInput
		wantName      string
		wantCompanyID *string
		wantErr       bool
	}{
		{
			name:     "valid minimal",
			in:       ContactInput{Name: "Alice Example"},
			wantName: "Alice Example",
		},
		{
			name:          "trims name and keeps company",
			in:            ContactInput{Name: "  Alice  ", CompanyID: &companyID},
			wantName:      "Alice",
			wantCompanyID: &companyID,
		},
		{
			name:     "blank company id normalizes to nil",
			in:       ContactInput{Name: "Alice", CompanyID: &blank},
			wantName: "Alice",
		},
		{
			name:     "valid email",
			in:       ContactInput{Name: "Alice", Email: "alice@example.test"},
			wantName: "Alice",
		},
		{"empty name", ContactInput{Name: ""}, "", nil, true},
		{"whitespace name", ContactInput{Name: "   "}, "", nil, true},
		{"invalid email", ContactInput{Name: "Alice", Email: "not an email"}, "", nil, true},
		{"email with display name", ContactInput{Name: "Alice", Email: "Alice <alice@b.test>"}, "", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			name, companyID, err := tt.in.normalize()
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
			if name != tt.wantName {
				t.Fatalf("normalize name = %q, want %q", name, tt.wantName)
			}
			switch {
			case tt.wantCompanyID == nil && companyID != nil:
				t.Fatalf("normalize companyID = %q, want nil", *companyID)
			case tt.wantCompanyID != nil && companyID == nil:
				t.Fatalf("normalize companyID = nil, want %q", *tt.wantCompanyID)
			case tt.wantCompanyID != nil && *companyID != *tt.wantCompanyID:
				t.Fatalf("normalize companyID = %q, want %q", *companyID, *tt.wantCompanyID)
			}
		})
	}
}
