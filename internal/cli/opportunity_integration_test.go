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
	"github.com/hackebrot/opportunities/internal/store"
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

// TestIntegrationOpportunityContactAttachDetachViaCLI exercises the
// secondary attach/detach path through the CLI: create an opportunity
// without a contact, attach one via `opps opportunity contact attach`,
// then detach with `--as` to remove that specific relationship while
// leaving any others intact.
func TestIntegrationOpportunityContactAttachDetachViaCLI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	dsn := st.Pool.Config().ConnString()
	t.Setenv("OPPS_DATABASE_URL", dsn)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	companyOut := runCmd(ctx, t, "--non-interactive", "company", "create",
		"--name", "Acme Corp", "--json")
	companyID := decodeCompanyJSON(t, companyOut).ID

	contactOut := runCmd(ctx, t, "--non-interactive", "contact", "create",
		"--name", "Alice Example", "--company", companyID, "--json")
	contactID := decodeContactJSON(t, contactOut).ID

	createOut := runCmd(
		ctx, t,
		"--non-interactive", "opportunity", "create",
		"--company", companyID,
		"--role-title", "Staff Engineer",
		"--office-days-per-week", "0",
		"--source", "outbound",
		"--json",
	)
	oppID := decodeOpportunityJSON(t, createOut).ID

	// Attach the same contact under two relationships.
	if _, err := tryRun(ctx, t, "--non-interactive",
		"opportunity", "contact", "attach", contactID,
		"--opportunity", oppID, "--as", "recruiter"); err != nil {
		t.Fatalf("attach recruiter: %v", err)
	}
	if _, err := tryRun(ctx, t, "--non-interactive",
		"opportunity", "contact", "attach", contactID,
		"--opportunity", oppID, "--as", "interviewer"); err != nil {
		t.Fatalf("attach interviewer: %v", err)
	}

	countRows := func(t *testing.T) int {
		t.Helper()
		var n int
		if err := st.Pool.QueryRow(
			ctx,
			`SELECT count(*) FROM opportunity_contacts WHERE opportunity_id = $1`, oppID,
		).Scan(&n); err != nil {
			t.Fatalf("count rows: %v", err)
		}
		return n
	}

	if got := countRows(t); got != 2 {
		t.Fatalf("after attach: rows = %d, want 2", got)
	}

	// Detach without --as in non-interactive mode must fail (PK is the
	// triple — refusing to guess is the safe default).
	if _, err := tryRun(ctx, t, "--non-interactive",
		"opportunity", "contact", "detach", contactID,
		"--opportunity", oppID); err == nil {
		t.Fatal("detach without --as: expected error")
	}

	// Detach the recruiter relationship; interviewer must remain.
	if _, err := tryRun(ctx, t, "--non-interactive",
		"opportunity", "contact", "detach", contactID,
		"--opportunity", oppID, "--as", "recruiter"); err != nil {
		t.Fatalf("detach recruiter: %v", err)
	}
	var remaining string
	if err := st.Pool.QueryRow(
		ctx,
		`SELECT relationship FROM opportunity_contacts WHERE opportunity_id = $1`, oppID,
	).Scan(&remaining); err != nil {
		t.Fatalf("query remaining: %v", err)
	}
	if remaining != "interviewer" {
		t.Fatalf("remaining = %q, want interviewer", remaining)
	}
	if got := countRows(t); got != 1 {
		t.Fatalf("after detach: rows = %d, want 1", got)
	}

	// Detaching a row that isn't there surfaces as a runtime error
	// (store.ErrNotFound under the hood). The CLI propagates it.
	if _, err := tryRun(ctx, t, "--non-interactive",
		"opportunity", "contact", "detach", contactID,
		"--opportunity", oppID, "--as", "recruiter"); err == nil {
		t.Fatal("detach already-detached row: expected error")
	}
}

// TestIntegrationOpportunityApplyViaCLI proves `opps opportunity apply
// [<id>]` creates an application against the picked opportunity, flips
// latest_status, and that a subsequent attempt to apply again fails
// with ErrActiveExists — the partial unique index sees one slot taken.
func TestIntegrationOpportunityApplyViaCLI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	dsn := st.Pool.Config().ConnString()
	t.Setenv("OPPS_DATABASE_URL", dsn)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	companyOut := runCmd(ctx, t, "--non-interactive", "company", "create",
		"--name", "Acme Corp", "--json")
	companyID := decodeCompanyJSON(t, companyOut).ID

	createOut := runCmd(ctx, t, "--non-interactive", "opportunity", "create",
		"--company", companyID,
		"--role-title", "Staff Engineer",
		"--office-days-per-week", "0",
		"--source", "outbound",
		"--json")
	oppID := decodeOpportunityJSON(t, createOut).ID

	applyOut := runCmd(ctx, t, "--non-interactive", "opportunity", "apply", oppID,
		"--applied-with-email", "me@example.test",
		"--notes", "applied via careers page",
		"--json")
	app := decodeApplicationJSON(t, applyOut)
	if app.Status != "applied" {
		t.Fatalf("apply status = %q, want applied", app.Status)
	}
	if app.OpportunityID != oppID {
		t.Fatalf("apply opportunity_id = %q, want %q", app.OpportunityID, oppID)
	}

	// latest_status flips to applied.
	showOut := runCmd(ctx, t, "opportunity", "show", oppID, "--json")
	if got := decodeOpportunityJSON(t, showOut).LatestStatus; got != "applied" {
		t.Fatalf("latest_status after apply = %q, want applied", got)
	}

	// A second apply is rejected by the partial unique index. Pin the
	// concrete error so a future regression that swallows it into a
	// generic "internal error" still fails here.
	if _, err := tryRun(ctx, t, "--non-interactive", "opportunity", "apply", oppID); !errors.Is(err, store.ErrActiveExists) {
		t.Fatalf("second apply: err=%v, want store.ErrActiveExists", err)
	}
}

// TestIntegrationApplyAliasViaCLI proves the top-level `opps apply
// <id>` shortcut reaches the same RunE as the canonical noun-first
// `opps opportunity apply <id>` — a thin wiring check that catches a
// missing AddCommand or a divergent factory.
func TestIntegrationApplyAliasViaCLI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	dsn := st.Pool.Config().ConnString()
	t.Setenv("OPPS_DATABASE_URL", dsn)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	companyOut := runCmd(ctx, t, "--non-interactive", "company", "create",
		"--name", "Acme Corp", "--json")
	companyID := decodeCompanyJSON(t, companyOut).ID

	createOut := runCmd(ctx, t, "--non-interactive", "opportunity", "create",
		"--company", companyID,
		"--source", "outbound",
		"--office-days-per-week", "0",
		"--json")
	oppID := decodeOpportunityJSON(t, createOut).ID

	aliasOut := runCmd(ctx, t, "--non-interactive", "apply", oppID, "--json")
	app := decodeApplicationJSON(t, aliasOut)
	if app.Status != "applied" {
		t.Fatalf("alias apply status = %q, want applied", app.Status)
	}
	if app.OpportunityID != oppID {
		t.Fatalf("alias apply opp = %q, want %q", app.OpportunityID, oppID)
	}
}

// TestIntegrationOpportunityEventCreateRejectViaCLI proves that moving
// an active application to rejected via `opps opportunity event create`
// — with the archive_reason_category supplied as a flag — archives the
// application row and flips latest_status to dormant (any-app, none
// active).
func TestIntegrationOpportunityEventCreateRejectViaCLI(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	dsn := st.Pool.Config().ConnString()
	t.Setenv("OPPS_DATABASE_URL", dsn)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	companyOut := runCmd(ctx, t, "--non-interactive", "company", "create",
		"--name", "Acme Corp", "--json")
	companyID := decodeCompanyJSON(t, companyOut).ID

	createOut := runCmd(ctx, t, "--non-interactive", "opportunity", "create",
		"--company", companyID,
		"--role-title", "Staff Engineer",
		"--office-days-per-week", "0",
		"--source", "outbound",
		"--json")
	oppID := decodeOpportunityJSON(t, createOut).ID

	if _, err := tryRun(ctx, t, "--non-interactive", "opportunity", "apply", oppID); err != nil {
		t.Fatalf("apply: %v", err)
	}

	// Reject without the category must fail before SQL.
	if _, err := tryRun(ctx, t, "--non-interactive",
		"opportunity", "event", "create", oppID,
		"--kind", "rejected"); err == nil {
		t.Fatal("rejected without --archive-reason-category: expected error")
	}

	// Reject with category lands and archives the app.
	eventOut := runCmd(ctx, t, "--non-interactive",
		"opportunity", "event", "create", oppID,
		"--kind", "rejected",
		"--archive-reason-category", "process_ended",
		"--json")
	var ev struct {
		Kind          string  `json:"kind"`
		ApplicationID *string `json:"application_id"`
	}
	if err := json.Unmarshal([]byte(eventOut), &ev); err != nil {
		t.Fatalf("event unmarshal: %v\n%s", err, eventOut)
	}
	if ev.Kind != "rejected" {
		t.Fatalf("event kind = %q, want rejected", ev.Kind)
	}
	if ev.ApplicationID == nil || *ev.ApplicationID == "" {
		t.Fatalf("event application_id missing: %s", eventOut)
	}

	// Application row is archived with the supplied category.
	var (
		status         string
		archivedAt     *time.Time
		reasonCategory *string
	)
	if err := st.Pool.QueryRow(
		ctx, `
		SELECT status, archived_at, archive_reason_category
		FROM applications
		WHERE opportunity_id = $1`, oppID,
	).Scan(&status, &archivedAt, &reasonCategory); err != nil {
		t.Fatalf("query application: %v", err)
	}
	if status != "rejected" {
		t.Fatalf("application status = %q, want rejected", status)
	}
	if archivedAt == nil {
		t.Fatal("application archived_at = nil, want non-nil")
	}
	if reasonCategory == nil || *reasonCategory != "process_ended" {
		t.Fatalf("archive_reason_category = %v, want process_ended", reasonCategory)
	}

	// latest_status flips to dormant: any app exists, none active.
	showOut := runCmd(ctx, t, "opportunity", "show", oppID, "--json")
	if got := decodeOpportunityJSON(t, showOut).LatestStatus; got != "dormant" {
		t.Fatalf("latest_status after rejected = %q, want dormant", got)
	}

	// --kind omitted in non-interactive mode now surfaces ErrNonInteractive.
	if _, err := tryRun(ctx, t, "--non-interactive",
		"opportunity", "event", "create", oppID); !errors.Is(err, prompt.ErrNonInteractive) {
		t.Fatalf("event create without --kind: err=%v, want ErrNonInteractive", err)
	}
}

type applicationJSONShape struct {
	ID                    string  `json:"id"`
	OpportunityID         string  `json:"opportunity_id"`
	Status                string  `json:"status"`
	Notes                 string  `json:"notes"`
	ArchiveReasonCategory *string `json:"archive_reason_category"`
}

func decodeApplicationJSON(t *testing.T, s string) applicationJSONShape {
	t.Helper()
	var a applicationJSONShape
	if err := json.Unmarshal([]byte(s), &a); err != nil {
		t.Fatalf("decode application JSON: %v\n%s", err, s)
	}
	return a
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
