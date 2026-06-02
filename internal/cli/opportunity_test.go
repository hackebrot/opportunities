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

// TestOpportunitySubcommandFlags asserts every documented flag is wired
// so future refactors that drop a flag fail loudly here instead of at
// the user.
func TestOpportunitySubcommandFlags(t *testing.T) {
	t.Parallel()

	createFlags := []string{
		"company", "role-title", "location", "office-days-per-week",
		"source", "source-detail", "priority", "notes",
		"contact", "relationship", "json",
	}
	updateFlags := []string{
		"company", "role-title", "location", "office-days-per-week",
		"source", "source-detail", "priority", "notes", "json",
	}

	cases := map[string]struct {
		cmd  func() *cobra.Command
		want []string
	}{
		"opportunity create":       {newOpportunityCreateCmd, createFlags},
		"opportunity list":         {newOpportunityListCmd, []string{"json", "sort"}},
		"opportunity show":         {newOpportunityShowCmd, []string{"json"}},
		"opportunity update":       {newOpportunityUpdateCmd, updateFlags},
		"opportunity rm":           {newOpportunityRmCmd, []string{"yes"}},
		"opportunity archive":      {newOpportunityArchiveCmd, []string{"reason", "json"}},
		"opportunity note":         {newOpportunityNoteCmd, []string{"json"}},
		"opportunity event create": {newOpportunityEventCreateCmd, []string{"kind", "label", "notes", "json"}},
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

func TestPrintOpportunityJSON(t *testing.T) {
	t.Parallel()

	role := "Staff Engineer"
	reason := "team disbanded"
	archived := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	o := model.Opportunity{
		ID:                "00000000-0000-0000-0000-000000000001",
		CompanyID:         "00000000-0000-0000-0000-0000000000aa",
		CompanyName:       "Acme Corp",
		RoleTitle:         &role,
		Location:          "Berlin",
		OfficeDaysPerWeek: 3,
		Source:            "outbound",
		SourceDetail:      "",
		Priority:          "normal",
		LatestStatus:      "watching",
		ArchivedAt:        &archived,
		ArchiveReason:     &reason,
		Notes:             "promising",
		CreatedAt:         time.Date(2026, 5, 20, 10, 30, 0, 0, time.UTC),
		UpdatedAt:         time.Date(2026, 5, 20, 11, 0, 0, 0, time.UTC),
	}

	var buf bytes.Buffer
	if err := printOpportunity(&buf, o, true); err != nil {
		t.Fatalf("printOpportunity: %v", err)
	}
	var got opportunityJSON
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal: %v\n%s", err, buf.String())
	}
	archivedStr := "2026-05-21T10:00:00Z"
	want := opportunityJSON{
		ID:                o.ID,
		CompanyID:         o.CompanyID,
		CompanyName:       o.CompanyName,
		RoleTitle:         o.RoleTitle,
		Location:          o.Location,
		OfficeDaysPerWeek: o.OfficeDaysPerWeek,
		Source:            o.Source,
		Priority:          o.Priority,
		LatestStatus:      o.LatestStatus,
		ArchivedAt:        &archivedStr,
		ArchiveReason:     o.ArchiveReason,
		Notes:             o.Notes,
		CreatedAt:         "2026-05-20T10:30:00Z",
		UpdatedAt:         "2026-05-20T11:00:00Z",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("opportunityJSON mismatch (-want +got):\n%s", diff)
	}
}

func TestPrintOpportunitiesTableHeader(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := printOpportunities(&buf, nil, false); err != nil {
		t.Fatalf("printOpportunities: %v", err)
	}
	for _, col := range []string{"ID", "COMPANY", "ROLE", "STATUS", "SOURCE", "PRIORITY"} {
		if !strings.Contains(buf.String(), col) {
			t.Fatalf("table header missing %q: %q", col, buf.String())
		}
	}
}

func TestSortOpportunities(t *testing.T) {
	t.Parallel()

	t1 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)

	items := []model.Opportunity{
		{ID: "a", LatestStatus: "watching", CreatedAt: t3},
		{ID: "b", LatestStatus: "applied", CreatedAt: t1},
		{ID: "c", LatestStatus: "applied", CreatedAt: t2},
	}
	if err := sortOpportunities(items, "status"); err != nil {
		t.Fatalf("sort: %v", err)
	}
	// Alphabetical by status, then newer first within a status.
	wantIDs := []string{"c", "b", "a"}
	for i, want := range wantIDs {
		if items[i].ID != want {
			t.Fatalf("sorted[%d].ID = %q, want %q (full: %+v)", i, items[i].ID, want, items)
		}
	}

	if err := sortOpportunities(items, "bogus"); err == nil {
		t.Fatal("sort bogus: want error")
	}
}

func TestOpportunityPickLabel(t *testing.T) {
	t.Parallel()
	role := "Staff Engineer"
	if got := opportunityPickLabel(model.Opportunity{
		CompanyName: "Acme", RoleTitle: &role, LatestStatus: "watching",
	}); got != "Acme — Staff Engineer [watching]" {
		t.Fatalf("got %q", got)
	}
	if got := opportunityPickLabel(model.Opportunity{
		CompanyName: "Acme", LatestStatus: "watching",
	}); got != "Acme — (no role) [watching]" {
		t.Fatalf("got %q", got)
	}
}

func TestSplitNoteArgs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		args     []string
		wantOpp  []string
		wantNote string
	}{
		{nil, nil, ""},
		{[]string{"hello world"}, nil, "hello world"},
		{[]string{"oppid", "hello world"}, []string{"oppid"}, "hello world"},
	}
	for _, tt := range tests {
		oppArgs, note := splitNoteArgs(tt.args)
		if diff := cmp.Diff(tt.wantOpp, oppArgs); diff != "" || note != tt.wantNote {
			t.Errorf("splitNoteArgs(%v) note=%q want %q; oppArgs diff (-want +got):\n%s",
				tt.args, note, tt.wantNote, diff)
		}
	}
}
