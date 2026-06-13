package cli

import (
	"testing"

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
