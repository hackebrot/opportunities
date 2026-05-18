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
// prompt helpers can honor it.
func TestNonInteractiveFlagPropagatesToContext(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"default", []string{"probe"}, false},
		{"flag set", []string{"--non-interactive", "probe"}, true},
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

// TestWireGlobalsRecoversFromShadowedPreRun verifies a subcommand
// defining its own PersistentPreRunE can still propagate
// --non-interactive by calling cli.WireGlobals — guarding the documented
// contract for future commands that need their own PreRun logic.
func TestWireGlobalsRecoversFromShadowedPreRun(t *testing.T) {
	t.Parallel()

	var got bool
	shadowed := &cobra.Command{
		Use: "shadowed",
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return cli.WireGlobals(cmd)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			got = prompt.IsNonInteractive(cmd.Context())
			return nil
		},
	}

	root := cli.NewRoot("test")
	root.AddCommand(shadowed)

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"--non-interactive", "shadowed"})

	if err := root.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !got {
		t.Fatal("non-interactive: shadowed PersistentPreRunE failed to propagate flag")
	}
}
