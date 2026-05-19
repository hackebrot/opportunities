package cli_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hackebrot/opportunities/internal/cli"
)

func TestConfigPathPrintsResolvedPath(t *testing.T) {
	tests := map[string]struct {
		xdg  string
		home string
		want string
	}{
		"honors XDG_CONFIG_HOME": {
			xdg:  "/tmp/xdg",
			home: "/home/user",
			want: filepath.Join("/tmp/xdg", "opportunities", "config.toml"),
		},
		"falls back to home": {
			xdg:  "",
			home: "/home/user",
			want: filepath.Join("/home/user", ".config", "opportunities", "config.toml"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", tc.xdg)
			t.Setenv("HOME", tc.home)

			var stdout bytes.Buffer
			root := cli.NewRoot("v0.0.0-test")
			root.SetOut(&stdout)
			root.SetErr(&stdout)
			root.SetArgs([]string{"config", "path"})

			if err := root.Execute(); err != nil {
				t.Fatalf("Execute: %v", err)
			}

			got := strings.TrimSpace(stdout.String())
			if got != tc.want {
				t.Fatalf("config path output: got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConfigPathRejectsExtraArgs(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	t.Setenv("HOME", "/home/user")

	var buf bytes.Buffer
	root := cli.NewRoot("v0.0.0-test")
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"config", "path", "extra"})

	if err := root.Execute(); err == nil {
		t.Fatal("Execute: expected error for extra args, got nil")
	}
}
