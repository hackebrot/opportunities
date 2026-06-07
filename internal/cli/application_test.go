package cli

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/hackebrot/opportunities/internal/model"
	"github.com/hackebrot/opportunities/internal/service"
)

func TestApplicationFollowUpFlags(t *testing.T) {
	t.Parallel()

	cmd := newApplicationFollowUpCmd()
	for _, f := range []string{"blocked", "done", "json"} {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("application follow-up: flag --%s missing", f)
		}
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
