package config_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/hackebrot/opportunities/internal/config"
)

func TestLoadFromDefaultsWhenFileMissing(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "missing.toml")
	want := config.Defaults()

	got, err := config.LoadFrom(missing, nil)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Config mismatch (-want +got):\n%s", diff)
	}
}

func TestLoadFromFileOverridesDefaults(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	body := `locale = "de-DE"
timezone = "Europe/Berlin"

[database]
url = "postgres://from-file/db"

[http]
port = 9090

[compensation]
reporting_currency = "USD"

[compensation.fx_rates]
EUR = 1.05
USD = 1.00

[dashboard]
active_application_stale_after = "21d"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	want := config.Defaults()
	want.Locale = "de-DE"
	want.Timezone = "Europe/Berlin"
	want.Database.URL = "postgres://from-file/db"
	want.HTTP.Port = 9090
	want.Compensation.ReportingCurrency = "USD"
	want.Compensation.FXRates["EUR"] = 1.05
	want.Compensation.FXRates["USD"] = 1.00
	want.Dashboard.ActiveApplicationStaleAfter = "21d"

	got, err := config.LoadFrom(path, nil)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Config mismatch (-want +got):\n%s", diff)
	}
}

// Keys absent from the file should retain their default values rather
// than wipe the rest of the map — verifies BurntSushi/toml's merge
// (not replace) semantics for map fields.
func TestLoadFromPartialMapsMergeWithDefaults(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	body := `[compensation.fx_rates]
EUR = 1.05

[ranking.source_weights]
referral = 0.95
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	want := config.Defaults()
	want.Compensation.FXRates["EUR"] = 1.05
	want.Ranking.SourceWeights["referral"] = 0.95

	got, err := config.LoadFrom(path, nil)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if diff := cmp.Diff(want.Compensation.FXRates, got.Compensation.FXRates); diff != "" {
		t.Errorf("FXRates mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(want.Ranking.SourceWeights, got.Ranking.SourceWeights); diff != "" {
		t.Errorf("SourceWeights mismatch (-want +got):\n%s", diff)
	}
}

func TestLoadFromEnvOverridesFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	body := `locale = "de-DE"
timezone = "Europe/Berlin"

[database]
url = "postgres://from-file/db"

[http]
port = 9090
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	env := map[string]string{
		"OPPS_LOCALE":       "en-US",
		"OPPS_TIMEZONE":     "UTC",
		"OPPS_DATABASE_URL": "postgres://from-env/db",
		"OPPS_HTTP_PORT":    "1234",
		"OPPS_LOG_LEVEL":    "debug",
	}

	want := config.Defaults()
	want.Locale = "en-US"
	want.Timezone = "UTC"
	want.Database.URL = "postgres://from-env/db"
	want.HTTP.Port = 1234
	want.LogLevel = "debug"

	got, err := config.LoadFrom(path, func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Config mismatch (-want +got):\n%s", diff)
	}
}

func TestLoadFromInvalidTOML(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("not = valid = toml"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := config.LoadFrom(path, nil)
	if err == nil {
		t.Fatal("LoadFrom: expected error for invalid TOML, got nil")
	}

	if diff := cmp.Diff(config.Config{}, got); diff != "" {
		t.Errorf("expected zero Config on error (-want +got):\n%s", diff)
	}
}

func TestLoadFromInvalidHTTPPortEnv(t *testing.T) {
	t.Parallel()

	env := map[string]string{"OPPS_HTTP_PORT": "not-a-number"}

	got, err := config.LoadFrom("", func(k string) string { return env[k] })
	if err == nil {
		t.Fatal("LoadFrom: expected error for non-numeric OPPS_HTTP_PORT, got nil")
	}
	if !strings.Contains(err.Error(), "OPPS_HTTP_PORT") {
		t.Errorf("error should attribute the source env var, got: %v", err)
	}

	if diff := cmp.Diff(config.Config{}, got); diff != "" {
		t.Errorf("expected zero Config on error (-want +got):\n%s", diff)
	}
}

func TestLoadFromHTTPPortOutOfRangeEnv(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"below range": "0",
		"above range": "70000",
		"negative":    "-1",
	}

	for name, port := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			env := map[string]string{"OPPS_HTTP_PORT": port}

			got, err := config.LoadFrom("", func(k string) string { return env[k] })
			if err == nil {
				t.Fatal("LoadFrom: expected error for out-of-range port, got nil")
			}
			if !strings.Contains(err.Error(), "OPPS_HTTP_PORT") {
				t.Errorf("error should attribute the source env var, got: %v", err)
			}
			if diff := cmp.Diff(config.Config{}, got); diff != "" {
				t.Errorf("expected zero Config on error (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLoadFromHTTPPortOutOfRangeFile(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"below range": "[http]\nport = 0\n",
		"above range": "[http]\nport = 70000\n",
		"negative":    "[http]\nport = -1\n",
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			got, err := config.LoadFrom(path, nil)
			if err == nil {
				t.Fatal("LoadFrom: expected error for out-of-range port, got nil")
			}
			if diff := cmp.Diff(config.Config{}, got); diff != "" {
				t.Errorf("expected zero Config on error (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLoadFromHTTPPortBoundaries(t *testing.T) {
	t.Parallel()

	tests := map[string]int{
		"min": 1,
		"max": 65535,
	}

	for name, port := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			env := map[string]string{"OPPS_HTTP_PORT": strconv.Itoa(port)}

			got, err := config.LoadFrom("", func(k string) string { return env[k] })
			if err != nil {
				t.Fatalf("LoadFrom: %v", err)
			}
			if got.HTTP.Port != port {
				t.Errorf("HTTP.Port: got %d, want %d", got.HTTP.Port, port)
			}
		})
	}
}

// Per the XDG Base Directory spec, a non-absolute XDG_CONFIG_HOME is
// invalid and must be ignored; we fall back to $HOME/.config rather
// than resolving the relative path against cwd.
func TestDefaultPath(t *testing.T) {
	tests := map[string]struct {
		xdg  string
		home string
		want string
	}{
		"honors absolute XDG_CONFIG_HOME": {
			xdg:  "/tmp/xdg",
			home: "/home/user",
			want: filepath.Join("/tmp/xdg", "opportunities", "config.toml"),
		},
		"falls back to home when XDG unset": {
			xdg:  "",
			home: "/home/user",
			want: filepath.Join("/home/user", ".config", "opportunities", "config.toml"),
		},
		"ignores relative XDG_CONFIG_HOME": {
			xdg:  "relative/dir",
			home: "/home/user",
			want: filepath.Join("/home/user", ".config", "opportunities", "config.toml"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", tc.xdg)
			t.Setenv("HOME", tc.home)

			got, err := config.DefaultPath()
			if err != nil {
				t.Fatalf("DefaultPath: %v", err)
			}
			if got != tc.want {
				t.Errorf("DefaultPath: got %q, want %q", got, tc.want)
			}
		})
	}
}
