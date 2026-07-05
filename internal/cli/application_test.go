package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/cobra"

	"github.com/hackebrot/opportunities/internal/model"
	"github.com/hackebrot/opportunities/internal/service"
)

// TestApplicationSubcommandFlags asserts every documented flag is wired
// so future refactors that drop a flag fail loudly here instead of at
// the user.
func TestApplicationSubcommandFlags(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		cmd  func() *cobra.Command
		want []string
	}{
		"application create":    {newApplicationCreateCmd, []string{"opportunity", "applied-at", "applied-with-email", "notes", "json"}},
		"application list":      {newApplicationListCmd, []string{"json"}},
		"application show":      {newApplicationShowCmd, []string{"json"}},
		"application update":    {newApplicationUpdateCmd, []string{"applied-at", "applied-with-email", "notes", "json"}},
		"application rm":        {newApplicationRmCmd, []string{"yes"}},
		"application follow-up": {newApplicationFollowUpCmd, []string{"blocked", "done", "json"}},
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

// TestFilterFollowUpCandidates pins which apps land in the picker for
// each mode: stamp and block see active-and-unblocked; done sees
// active-and-blocked; terminal status is always filtered out.
func TestFilterFollowUpCandidates(t *testing.T) {
	t.Parallel()

	apps := []model.Application{
		{ID: "active-unblocked", Status: "applied"},
		{ID: "active-blocked", Status: "in_progress", FollowUpBlocked: true},
		{ID: "offer-unblocked", Status: "offer"},
		{ID: "rejected", Status: "rejected"},
		{ID: "accepted", Status: "accepted"},
	}

	want := map[service.FollowUpMode][]string{
		service.FollowUpStamp: {"active-unblocked", "offer-unblocked"},
		service.FollowUpBlock: {"active-unblocked", "offer-unblocked"},
		service.FollowUpDone:  {"active-blocked"},
	}

	for mode, wantIDs := range want {
		got := filterFollowUpCandidates(apps, mode)
		var gotIDs []string
		for _, a := range got {
			gotIDs = append(gotIDs, a.ID)
		}
		if diff := cmp.Diff(wantIDs, gotIDs); diff != "" {
			t.Errorf("filterFollowUpCandidates(mode=%v) (-want +got):\n%s", mode, diff)
		}
	}
}

// TestPrintApplicationDetailTable covers the table branch of the composed
// show renderer: it must stack the application's own fields, then the
// stages section, then the compensations section — the wiring the
// JSON-only integration test doesn't exercise.
func TestPrintApplicationDetailTable(t *testing.T) {
	t.Parallel()

	applied := time.Date(2026, 6, 20, 9, 0, 0, 0, time.UTC)
	app := model.Application{
		ID:            "00000000-0000-0000-0000-000000000001",
		OpportunityID: "00000000-0000-0000-0000-0000000000aa",
		AppliedAt:     &applied,
		Status:        "offer",
		Notes:         "onsite went well",
		CreatedAt:     applied,
		UpdatedAt:     applied,
	}
	stages := []model.ApplicationStage{{OrderIndex: 1, Label: "Recruiter screen"}}
	comps := []model.Compensation{{Kind: "offered", BaseMinor: 300, Currency: "EUR"}}

	var buf bytes.Buffer
	if err := printApplicationDetail(&buf, app, stages, comps, false); err != nil {
		t.Fatalf("printApplicationDetail: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"Status", "offer", // the application's own fields
		"Stages:", "Recruiter screen", // stages section
		"Compensations:", "offered", "EUR", // compensations section
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// Section order: application fields, then Stages, then Compensations.
	stagesAt := strings.Index(out, "Stages:")
	compsAt := strings.Index(out, "Compensations:")
	if stagesAt < 0 || compsAt < 0 || stagesAt > compsAt {
		t.Fatalf("want Stages before Compensations, got indexes %d and %d:\n%s", stagesAt, compsAt, out)
	}
}
