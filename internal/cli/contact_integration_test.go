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

// TestIntegrationContactCRUDViaCLI drives every contact subcommand
// through the cobra entry point against a real Postgres, including the
// embedded-company JOIN on read.
func TestIntegrationContactCRUDViaCLI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	dsn := st.Pool.Config().ConnString()
	t.Setenv("OPPS_DATABASE_URL", dsn)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Seed a company to attach the contact to.
	companyOut := runCmd(ctx, t, "--non-interactive", "company", "create",
		"--name", "Acme Corp", "--json")
	companyID := decodeCompanyJSON(t, companyOut).ID
	if companyID == "" {
		t.Fatalf("seed company returned empty id: %q", companyOut)
	}

	// create contact attached to the company (non-interactive)
	addOut := runCmd(
		ctx, t,
		"--non-interactive", "contact", "create",
		"--name", "Alice Example",
		"--email", "alice@example.test",
		"--linkedin", "in/ada",
		"--role", "Hiring Manager",
		"--company", companyID,
		"--notes", "first contact",
		"--json",
	)
	created := decodeContactJSON(t, addOut)
	id := created.ID
	if id == "" {
		t.Fatalf("add contact returned empty id: %q", addOut)
	}
	if created.Company == nil || created.Company.ID != companyID || created.Company.Name != "Acme Corp" {
		t.Fatalf("create: embedded company = %v, want Acme Corp/%s", created.Company, companyID)
	}

	// list contacts --json
	listOut := runCmd(ctx, t, "contact", "list", "--json")
	var listed []map[string]any
	if err := json.Unmarshal([]byte(listOut), &listed); err != nil {
		t.Fatalf("list json: %v\n%s", err, listOut)
	}
	if len(listed) != 1 || listed[0]["id"] != id {
		t.Fatalf("list contacts: want one row with id=%s, got %v", id, listed)
	}

	// show contact <id> --json — embedded company present
	showOut := runCmd(ctx, t, "contact", "show", id, "--json")
	shown := decodeContactJSON(t, showOut)
	if shown.Name != "Alice Example" {
		t.Fatalf("show contact: unexpected payload: %s", showOut)
	}
	if shown.Company == nil || shown.Company.Name != "Acme Corp" {
		t.Fatalf("show contact: embedded company missing: %s", showOut)
	}

	// update contact — rename
	updateOut := runCmd(ctx, t, "contact", "update", id, "--name", "Alice E.", "--json")
	updated := decodeContactJSON(t, updateOut)
	if updated.Name != "Alice E." {
		t.Fatalf("update name: got %q", updated.Name)
	}
	// Unspecified company association preserved.
	if updated.Company == nil || updated.Company.ID != companyID {
		t.Fatalf("update preserved company: got %v", updated.Company)
	}

	// Clearing the company via an explicit empty flag.
	clearedOut := runCmd(ctx, t, "contact", "update", id, "--company", "", "--json")
	cleared := decodeContactJSON(t, clearedOut)
	if cleared.Company != nil {
		t.Fatalf("clear company: got %v, want nil", cleared.Company)
	}
	if cleared.Name != "Alice E." {
		t.Fatalf("clear company: unrelated rename overwritten: got %q", cleared.Name)
	}

	// rm without --yes must error in non-interactive mode.
	if _, err := tryRun(ctx, t, "--non-interactive", "contact", "rm", id); !errors.Is(err, prompt.ErrNonInteractive) {
		t.Fatalf("rm without --yes in non-interactive: err=%v, want ErrNonInteractive", err)
	}

	// rm contact --yes
	if _, err := tryRun(ctx, t, "contact", "rm", id, "--yes"); err != nil {
		t.Fatalf("rm contact: %v", err)
	}
	emptyOut := strings.TrimSpace(runCmd(ctx, t, "contact", "list", "--json"))
	if emptyOut != "null" && emptyOut != "[]" {
		t.Fatalf("list after rm: want null/[], got %q", emptyOut)
	}

	// create with no name and no prompts → must error.
	if _, err := tryRun(ctx, t, "--non-interactive", "contact", "create"); !errors.Is(err, prompt.ErrNonInteractive) {
		t.Fatalf("create without --name in non-interactive: err=%v, want ErrNonInteractive", err)
	}

	// create with a dangling company id → FK rejected through the CLI.
	if _, err := tryRun(ctx, t, "--non-interactive", "contact", "create",
		"--name", "Ghost", "--company", "00000000-0000-0000-0000-000000000000"); err == nil {
		t.Fatal("create with dangling company id: expected error")
	}
}

type contactCompanyShape struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type contactJSONShape struct {
	ID      string               `json:"id"`
	Name    string               `json:"name"`
	Email   string               `json:"email"`
	Company *contactCompanyShape `json:"company"`
}

func decodeContactJSON(t *testing.T, s string) contactJSONShape {
	t.Helper()
	var c contactJSONShape
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		t.Fatalf("decode contact JSON: %v\n%s", err, s)
	}
	return c
}
