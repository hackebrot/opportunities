package cli_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/cobra"

	"github.com/hackebrot/opportunities/internal/cli"
	"github.com/hackebrot/opportunities/internal/prompt"
)

// TestNonInteractiveFlagPropagatesToContext verifies the persistent
// --non-interactive flag is wired through to the command context so
// prompt helpers can honor it. The "shadowed" case also guards against
// accidental removal of cobra.EnableTraverseRunHooks: a child defining
// its own PersistentPreRunE must not suppress the root's flag wiring.
func TestNonInteractiveFlagPropagatesToContext(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		args        []string
		childPreRun bool
		want        bool
	}{
		{"default", []string{"probe"}, false, false},
		{"flag set", []string{"--non-interactive", "probe"}, false, true},
		{"shadowed by child PreRun", []string{"--non-interactive", "probe"}, true, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got bool
			probe := &cobra.Command{
				Use: "probe",
				RunE: func(cmd *cobra.Command, _ []string) error {
					got = prompt.IsNonInteractive(cmd.Context())
					return nil
				},
			}
			if tc.childPreRun {
				probe.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
					return nil
				}
			}

			root := cli.NewRoot("test")
			root.AddCommand(probe)

			var buf bytes.Buffer
			root.SetOut(&buf)
			root.SetErr(&buf)
			root.SetArgs(tc.args)

			if err := root.ExecuteContext(context.Background()); err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if got != tc.want {
				t.Fatalf("non-interactive: got %v, want %v", got, tc.want)
			}
		})
	}
}
