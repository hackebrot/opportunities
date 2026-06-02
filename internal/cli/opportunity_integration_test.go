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

// TestIntegrationOpportunityCRUDViaCLI drives every opportunity
// subcommand through the cobra entry point against a real Postgres.
// The non-interactive path mirrors the canonical "recruiter messaged me"
// scenario: company, opportunity, contact, and relationship captured in
// one command.
func TestIntegrationOpportunityCRUDViaCLI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	dsn := st.Pool.Config().ConnString()
	t.Setenv("OPPS_DATABASE_URL", dsn)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Seed a company so the opportunity can attach via --company.
	companyOut := runCmd(ctx, t, "--non-interactive", "company", "create",
		"--name", "Acme Corp", "--json")
	companyID := decodeCompanyJSON(t, companyOut).ID
	if companyID == "" {
		t.Fatalf("seed company returned empty id: %q", companyOut)
	}

	// Seed an existing contact under the same company so --contact /
	// --relationship can attach it inline.
	contactOut := runCmd(ctx, t, "--non-interactive", "contact", "create",
		"--name", "Alice Example", "--company", companyID, "--json")
	contactID := decodeContactJSON(t, contactOut).ID
	if contactID == "" {
		t.Fatalf("seed contact returned empty id: %q", contactOut)
	}

	// Create opportunity in one command, attaching the existing contact.
	createOut := runCmd(
		ctx, t,
		"--non-interactive", "opportunity", "create",
		"--company", companyID,
		"--role-title", "Member of Technical Staff",
		"--location", "Berlin",
		"--office-days-per-week", "3",
		"--source", "inbound_inhouse_recruiter",
		"--source-detail", "warm intro",
		"--priority", "high",
		"--notes", "promising",
		"--contact", contactID,
		"--relationship", "recruiter",
		"--json",
	)
	created := decodeOpportunityJSON(t, createOut)
	if created.ID == "" {
		t.Fatalf("create returned empty id: %q", createOut)
	}
	if created.CompanyName != "Acme Corp" {
		t.Fatalf("company_name = %q, want Acme Corp", created.CompanyName)
	}
	if created.LatestStatus != "watching" {
		t.Fatalf("latest_status = %q, want watching", created.LatestStatus)
	}

	// Confirm the opportunity_contacts row landed.
	var rel string
	if err := st.Pool.QueryRow(
		ctx,
		`SELECT relationship FROM opportunity_contacts WHERE opportunity_id = $1`, created.ID,
	).Scan(&rel); err != nil {
		t.Fatalf("query opportunity_contacts: %v", err)
	}
	if rel != "recruiter" {
		t.Fatalf("relationship = %q, want recruiter", rel)
	}

	// list opportunities --json
	listOut := runCmd(ctx, t, "opportunity", "list", "--json")
	var listed []map[string]any
	if err := json.Unmarshal([]byte(listOut), &listed); err != nil {
		t.Fatalf("list json: %v\n%s", err, listOut)
	}
	if len(listed) != 1 || listed[0]["id"] != created.ID {
		t.Fatalf("list opportunities: want one row with id=%s, got %v", created.ID, listed)
	}

	// show opportunity <id> --json
	showOut := runCmd(ctx, t, "opportunity", "show", created.ID, "--json")
	if decodeOpportunityJSON(t, showOut).RoleTitle == nil ||
		*decodeOpportunityJSON(t, showOut).RoleTitle != "Member of Technical Staff" {
		t.Fatalf("show opportunity: unexpected payload: %s", showOut)
	}

	// note <id> <text>
	noteOut := runCmd(ctx, t, "opportunity", "note", created.ID, "great call with hiring manager", "--json")
	var noteEv struct {
		Kind  string `json:"kind"`
		Notes string `json:"notes"`
	}
	if err := json.Unmarshal([]byte(noteOut), &noteEv); err != nil {
		t.Fatalf("note unmarshal: %v\n%s", err, noteOut)
	}
	if noteEv.Kind != "note" || noteEv.Notes != "great call with hiring manager" {
		t.Fatalf("note event = %+v, want kind=note notes=...", noteEv)
	}

	// event create --kind=exploring
	eventOut := runCmd(ctx, t, "opportunity", "event", "create", created.ID, "--kind", "exploring", "--json")
	var ev struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal([]byte(eventOut), &ev); err != nil {
		t.Fatalf("event unmarshal: %v\n%s", err, eventOut)
	}
	if ev.Kind != "exploring" {
		t.Fatalf("event kind = %q, want exploring", ev.Kind)
	}
	// latest_status flips to exploring.
	statusOut := runCmd(ctx, t, "opportunity", "show", created.ID, "--json")
	if got := decodeOpportunityJSON(t, statusOut).LatestStatus; got != "exploring" {
		t.Fatalf("latest_status after exploring = %q, want exploring", got)
	}

	// update — rename role.
	updateOut := runCmd(ctx, t, "opportunity", "update", created.ID,
		"--role-title", "Senior Engineer", "--json")
	updated := decodeOpportunityJSON(t, updateOut)
	if updated.RoleTitle == nil || *updated.RoleTitle != "Senior Engineer" {
		t.Fatalf("update role: got %v", updated.RoleTitle)
	}

	// archive --reason
	archiveOut := runCmd(ctx, t, "opportunity", "archive", created.ID,
		"--reason", "team disbanded", "--json")
	archived := decodeOpportunityJSON(t, archiveOut)
	if archived.LatestStatus != "archived" {
		t.Fatalf("archive: latest_status = %q, want archived", archived.LatestStatus)
	}

	// rm without --yes must error in non-interactive mode.
	if _, err := tryRun(ctx, t, "--non-interactive", "opportunity", "rm", created.ID); !errors.Is(err, prompt.ErrNonInteractive) {
		t.Fatalf("rm without --yes in non-interactive: err=%v, want ErrNonInteractive", err)
	}

	// rm --yes
	if _, err := tryRun(ctx, t, "opportunity", "rm", created.ID, "--yes"); err != nil {
		t.Fatalf("rm: %v", err)
	}
	emptyOut := strings.TrimSpace(runCmd(ctx, t, "opportunity", "list", "--json"))
	if emptyOut != "null" && emptyOut != "[]" {
		t.Fatalf("list after rm: want null/[], got %q", emptyOut)
	}

	// create without source non-interactively must fail.
	if _, err := tryRun(ctx, t, "--non-interactive", "opportunity", "create",
		"--company", companyID); err == nil {
		t.Fatal("create without --source: expected error")
	}

	// --contact without --relationship must fail.
	if _, err := tryRun(ctx, t, "--non-interactive", "opportunity", "create",
		"--company", companyID, "--source", "outbound",
		"--contact", contactID); err == nil {
		t.Fatal("--contact without --relationship: expected error")
	}

	// --relationship without --contact must also fail (symmetric).
	if _, err := tryRun(ctx, t, "--non-interactive", "opportunity", "create",
		"--company", companyID, "--source", "outbound",
		"--relationship", "recruiter"); err == nil {
		t.Fatal("--relationship without --contact: expected error")
	}
}

type opportunityJSONShape struct {
	ID           string  `json:"id"`
	CompanyID    string  `json:"company_id"`
	CompanyName  string  `json:"company_name"`
	RoleTitle    *string `json:"role_title"`
	LatestStatus string  `json:"latest_status"`
}

func decodeOpportunityJSON(t *testing.T, s string) opportunityJSONShape {
	t.Helper()
	var o opportunityJSONShape
	if err := json.Unmarshal([]byte(s), &o); err != nil {
		t.Fatalf("decode opportunity JSON: %v\n%s", err, s)
	}
	return o
}
