package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hackebrot/opportunities/internal/cli"
)

func TestVersionSubcommandPrintsVersion(t *testing.T) {
	t.Parallel()

	const want = "v1.2.3-test"

	var stdout bytes.Buffer
	root := cli.NewRoot(want)
	root.SetOut(&stdout)
	root.SetErr(&stdout)
	root.SetArgs([]string{"version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	got := strings.TrimSpace(stdout.String())
	if got != want {
		t.Fatalf("version output: got %q, want %q", got, want)
	}
}

func TestVersionSubcommandRejectsExtraArgs(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	root := cli.NewRoot("v1.2.3-test")
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"version", "extra"})

	if err := root.Execute(); err == nil {
		t.Fatal("Execute: expected error for extra args, got nil")
	}
}
