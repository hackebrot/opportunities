package cli

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/cobra"

	"github.com/hackebrot/opportunities/internal/model"
	"github.com/hackebrot/opportunities/internal/prompt"
)

func TestOpportunityContactSubcommandFlags(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		cmd  func() *cobra.Command
		want []string
	}{
		"opportunity contact attach": {newOpportunityContactAttachCmd, []string{"opportunity", "as"}},
		"opportunity contact detach": {newOpportunityContactDetachCmd, []string{"opportunity", "as"}},
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

func TestUniqueAttachedContacts(t *testing.T) {
	t.Parallel()

	rows := []model.OpportunityContact{
		{ContactID: "alice", ContactName: "Alice", Relationship: "interviewer"},
		{ContactID: "alice", ContactName: "Alice", Relationship: "recruiter"},
		{ContactID: "bob", ContactName: "Bob", Relationship: "hiring_manager"},
	}
	got := uniqueAttachedContacts(rows)
	wantIDs := []string{"alice", "bob"}
	if len(got) != len(wantIDs) {
		t.Fatalf("len = %d, want %d", len(got), len(wantIDs))
	}
	for i, id := range wantIDs {
		if got[i].ContactID != id {
			t.Errorf("got[%d].ContactID = %q, want %q", i, got[i].ContactID, id)
		}
	}
}

func TestRelationshipsForContact(t *testing.T) {
	t.Parallel()

	rows := []model.OpportunityContact{
		{ContactID: "alice", Relationship: "interviewer"},
		{ContactID: "alice", Relationship: "recruiter"},
		{ContactID: "bob", Relationship: "hiring_manager"},
	}
	want := []prompt.Option{
		{Key: "interviewer", Label: "interviewer"},
		{Key: "recruiter", Label: "recruiter"},
	}
	if diff := cmp.Diff(want, relationshipsForContact(rows, "alice")); diff != "" {
		t.Errorf("relationshipsForContact (-want +got):\n%s", diff)
	}
	if got := relationshipsForContact(rows, "carol"); len(got) != 0 {
		t.Errorf("unknown contact: got %v, want empty", got)
	}
}
