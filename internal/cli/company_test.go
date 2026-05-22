package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/cobra"

	"github.com/hackebrot/opportunities/internal/model"
)

// TestCompanySubcommandFlags asserts every documented flag is wired so
// future refactors that drop a flag fail loudly here instead of at the
// user.
func TestCompanySubcommandFlags(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		cmd  func() *cobra.Command
		want []string
	}{
		"company create": {newCompanyCreateCmd, []string{"name", "website", "careers-url", "preferred-email", "notes", "json"}},
		"company list":   {newCompanyListCmd, []string{"json"}},
		"company show":   {newCompanyShowCmd, []string{"json"}},
		"company update": {newCompanyUpdateCmd, []string{"name", "website", "careers-url", "preferred-email", "notes", "json"}},
		"company rm":     {newCompanyRmCmd, []string{"yes"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cmd := tc.cmd()
			for _, f := range tc.want {
				if cmd.Flags().Lookup(f) == nil {
					t.Errorf("%s: flag --%s missing", name, f)
				}
			}
		})
	}
}

func TestPrintCompanyJSON(t *testing.T) {
	t.Parallel()

	email := "applicant+acme@example.test"
	c := model.Company{
		ID:             "00000000-0000-0000-0000-000000000001",
		Name:           "Acme Corp",
		Slug:           "acmecorp",
		Website:        "https://acme.test",
		CareersURL:     "https://acme.test/careers",
		PreferredEmail: &email,
		Notes:          "first contact via meetup",
		CreatedAt:      time.Date(2026, 5, 20, 10, 30, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC),
	}

	var buf bytes.Buffer
	if err := printCompany(&buf, c, true); err != nil {
		t.Fatalf("printCompany: %v", err)
	}
	var got companyJSON
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal: %v\n%s", err, buf.String())
	}
	want := companyJSON{
		ID:             c.ID,
		Name:           c.Name,
		Slug:           c.Slug,
		Website:        c.Website,
		CareersURL:     c.CareersURL,
		PreferredEmail: &email,
		Notes:          c.Notes,
		CreatedAt:      "2026-05-20T10:30:00Z",
		UpdatedAt:      "2026-05-20T11:00:00Z",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("companyJSON mismatch (-want +got):\n%s", diff)
	}
}

func TestPrintCompaniesTableHeader(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := printCompanies(&buf, nil, false); err != nil {
		t.Fatalf("printCompanies: %v", err)
	}
	if !strings.Contains(buf.String(), "ID") || !strings.Contains(buf.String(), "SLUG") {
		t.Fatalf("table header missing columns: %q", buf.String())
	}
}

func TestOneline(t *testing.T) {
	t.Parallel()

	got := oneline("a\nb\tc\rd")
	if want := "a b c d"; got != want {
		t.Fatalf("oneline: got %q, want %q", got, want)
	}
}

func TestCompanyInputFromModelDereferencesEmail(t *testing.T) {
	t.Parallel()

	email := "x@example.com"
	in := companyInputFromModel(model.Company{Name: "N", PreferredEmail: &email})
	if in.PreferredEmail != email {
		t.Fatalf("PreferredEmail: got %q, want %q", in.PreferredEmail, email)
	}

	in = companyInputFromModel(model.Company{Name: "N"})
	if in.PreferredEmail != "" {
		t.Fatalf("PreferredEmail: got %q, want empty", in.PreferredEmail)
	}
}
