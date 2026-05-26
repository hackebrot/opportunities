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

func TestContactSubcommandFlags(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		cmd  func() *cobra.Command
		want []string
	}{
		"contact create": {newContactCreateCmd, []string{"name", "email", "linkedin", "role", "company", "notes", "json"}},
		"contact list":   {newContactListCmd, []string{"json"}},
		"contact show":   {newContactShowCmd, []string{"json"}},
		"contact update": {newContactUpdateCmd, []string{"name", "email", "linkedin", "role", "company", "notes", "json"}},
		"contact rm":     {newContactRmCmd, []string{"yes"}},
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

func TestPrintContactJSON(t *testing.T) {
	t.Parallel()

	companyID := "00000000-0000-0000-0000-0000000000aa"
	companyName := "Acme Corp"
	c := model.Contact{
		ID:          "00000000-0000-0000-0000-000000000001",
		Name:        "Alice Example",
		Email:       "alice@example.test",
		LinkedIn:    "in/ada",
		Role:        "Hiring Manager",
		CompanyID:   &companyID,
		CompanyName: &companyName,
		Notes:       "met at conf",
		CreatedAt:   time.Date(2026, 5, 20, 10, 30, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC),
	}

	var buf bytes.Buffer
	if err := printContact(&buf, c, true); err != nil {
		t.Fatalf("printContact: %v", err)
	}
	var got contactJSON
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal: %v\n%s", err, buf.String())
	}
	want := contactJSON{
		ID:        c.ID,
		Name:      c.Name,
		Email:     c.Email,
		LinkedIn:  c.LinkedIn,
		Role:      c.Role,
		Company:   &contactCompanyJSON{ID: companyID, Name: companyName},
		Notes:     c.Notes,
		CreatedAt: "2026-05-20T10:30:00Z",
		UpdatedAt: "2026-05-20T11:00:00Z",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("contactJSON mismatch (-want +got):\n%s", diff)
	}
}

func TestPrintContactJSONNoCompany(t *testing.T) {
	t.Parallel()

	c := model.Contact{ID: "k1", Name: "Solo"}
	var buf bytes.Buffer
	if err := printContact(&buf, c, true); err != nil {
		t.Fatalf("printContact: %v", err)
	}
	// company key must be omitted entirely when there's no association.
	if strings.Contains(buf.String(), "company") {
		t.Fatalf("unattached contact JSON must omit company: %s", buf.String())
	}
}

func TestPrintContactsTableHeader(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := printContacts(&buf, nil, false); err != nil {
		t.Fatalf("printContacts: %v", err)
	}
	for _, col := range []string{"ID", "NAME", "ROLE", "COMPANY"} {
		if !strings.Contains(buf.String(), col) {
			t.Fatalf("table header missing %q: %q", col, buf.String())
		}
	}
}

func TestContactInputFromModelCarriesCompanyID(t *testing.T) {
	t.Parallel()

	companyID := "co1"
	in := contactInputFromModel(model.Contact{Name: "Alice", CompanyID: &companyID})
	if in.CompanyID == nil || *in.CompanyID != companyID {
		t.Fatalf("CompanyID = %v, want %q", in.CompanyID, companyID)
	}

	in = contactInputFromModel(model.Contact{Name: "Alice"})
	if in.CompanyID != nil {
		t.Fatalf("CompanyID = %q, want nil", *in.CompanyID)
	}
}

func TestContactPickLabel(t *testing.T) {
	t.Parallel()

	company := "Acme Corp"
	empty := ""
	cases := map[string]struct {
		contact model.Contact
		want    string
	}{
		"with company":  {model.Contact{Name: "Alice", CompanyName: &company}, "Alice (Acme Corp)"},
		"no company":    {model.Contact{Name: "Alice"}, "Alice"},
		"empty company": {model.Contact{Name: "Alice", CompanyName: &empty}, "Alice"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := contactPickLabel(tc.contact); got != tc.want {
				t.Fatalf("contactPickLabel = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNullableFlag(t *testing.T) {
	t.Parallel()

	if got := nullableFlag(""); got != nil {
		t.Fatalf("nullableFlag(\"\") = %q, want nil", *got)
	}
	if got := nullableFlag("x"); got == nil || *got != "x" {
		t.Fatalf("nullableFlag(\"x\") = %v, want \"x\"", got)
	}
}
