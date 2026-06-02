//go:build integration

package cli_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hackebrot/opportunities/internal/prompt"
)

// TestIntegrationCompanyCRUDViaCLI drives every company subcommand
// through the cobra entry point against a real Postgres. This is the
// single end-to-end proof that flags → service → DB is wired correctly.
func TestIntegrationCompanyCRUDViaCLI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	dsn := st.Pool.Config().ConnString()
	t.Setenv("OPPS_DATABASE_URL", dsn)
	// Force config.Load to a tempdir so the user's real config.toml is
	// not read during the test.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// add company (non-interactive — every value supplied via flags)
	addOut := runCmd(
		ctx, t,
		"--non-interactive", "company", "create",
		"--name", "Acme Corp",
		"--website", "https://acme.test",
		"--careers-url", "https://acme.test/careers",
		"--preferred-email", "applicant+acme@example.test",
		"--notes", "first contact",
		"--json",
	)
	id := decodeCompanyJSON(t, addOut).ID
	if id == "" {
		t.Fatalf("add company returned empty id: %q", addOut)
	}

	// list companies --json
	listOut := runCmd(ctx, t, "company", "list", "--json")
	var listed []map[string]any
	if err := json.Unmarshal([]byte(listOut), &listed); err != nil {
		t.Fatalf("list json: %v\n%s", err, listOut)
	}
	if len(listed) != 1 || listed[0]["id"] != id {
		t.Fatalf("list companies: want one row with id=%s, got %v", id, listed)
	}

	// show company <id> --json
	showOut := runCmd(ctx, t, "company", "show", id, "--json")
	if decodeCompanyJSON(t, showOut).Name != "Acme Corp" {
		t.Fatalf("show company: unexpected payload: %s", showOut)
	}

	// update company — rename
	updateOut := runCmd(
		ctx, t,
		"company", "update", id,
		"--name", "Acme Corporation",
		"--json",
	)
	updated := decodeCompanyJSON(t, updateOut)
	if updated.Name != "Acme Corporation" {
		t.Fatalf("update name: got %q", updated.Name)
	}
	// Slug must re-derive from the new name.
	if updated.Slug != "acmecorporation" {
		t.Fatalf("update slug: got %q, want acmecorporation", updated.Slug)
	}
	// Unspecified fields preserved.
	if updated.Website != "https://acme.test" {
		t.Fatalf("update preserved website: got %q", updated.Website)
	}
	if updated.PreferredEmail == nil || *updated.PreferredEmail != "applicant+acme@example.test" {
		t.Fatalf("update preserved preferred_email: got %v", updated.PreferredEmail)
	}

	// Clearing preferred_email via an explicit empty flag — the only
	// path that distinguishes "leave alone" from "overwrite with empty".
	clearedOut := runCmd(
		ctx, t,
		"company", "update", id,
		"--preferred-email", "",
		"--json",
	)
	cleared := decodeCompanyJSON(t, clearedOut)
	if cleared.PreferredEmail != nil {
		t.Fatalf("clear preferred_email: got %q, want nil", *cleared.PreferredEmail)
	}
	if cleared.Name != "Acme Corporation" {
		t.Fatalf("clear preferred_email: unrelated rename overwritten: got %q", cleared.Name)
	}

	// rm without --yes must error in non-interactive mode: the
	// confirmation prompt returns ErrNonInteractive rather than silently
	// dropping rows. Checked while the company still exists, so the error
	// is the confirmation path and not a not-found.
	if _, err := tryRun(ctx, t, "--non-interactive", "company", "rm", id); !errors.Is(err, prompt.ErrNonInteractive) {
		t.Fatalf("rm without --yes in non-interactive: err=%v, want ErrNonInteractive", err)
	}

	// rm company --yes
	if _, err := tryRun(ctx, t, "company", "rm", id, "--yes"); err != nil {
		t.Fatalf("rm company: %v", err)
	}
	// list is empty again
	emptyOut := runCmd(ctx, t, "company", "list", "--json")
	emptyOut = strings.TrimSpace(emptyOut)
	if emptyOut != "null" && emptyOut != "[]" {
		t.Fatalf("list after rm: want null/[], got %q", emptyOut)
	}

	// add company with no name and no prompts → must error.
	if _, err := tryRun(ctx, t, "--non-interactive", "company", "create"); !errors.Is(err, prompt.ErrNonInteractive) {
		t.Fatalf("add without --name in non-interactive: err=%v, want ErrNonInteractive", err)
	}

	// add company with a malformed website → service validation rejects
	// it through the CLI rather than persisting garbage.
	if _, err := tryRun(ctx, t, "--non-interactive", "company", "create", "--name", "Bad URL Co", "--website", "not-a-url"); err == nil {
		t.Fatal("create with malformed website: expected error")
	}
}

type companyJSONShape struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Slug           string  `json:"slug"`
	Website        string  `json:"website"`
	PreferredEmail *string `json:"preferred_email"`
}

func decodeCompanyJSON(t *testing.T, s string) companyJSONShape {
	t.Helper()
	var c companyJSONShape
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		t.Fatalf("decode company JSON: %v\n%s", err, s)
	}
	return c
}
