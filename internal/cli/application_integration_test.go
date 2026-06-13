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

// followUpJSONShape is the subset of applicationJSON the follow-up
// integration tests probe.
type followUpJSONShape struct {
	ID               string  `json:"id"`
	Status           string  `json:"status"`
	FollowUpBlocked  bool    `json:"follow_up_blocked"`
	LastFollowedUpAt *string `json:"last_followed_up_at"`
}

func decodeFollowUpJSON(t *testing.T, s string) followUpJSONShape {
	t.Helper()
	var a followUpJSONShape
	if err := json.Unmarshal([]byte(s), &a); err != nil {
		t.Fatalf("decode follow-up JSON: %v\n%s", err, s)
	}
	return a
}

// TestIntegrationApplicationFollowUpViaCLI walks the three modes
// end-to-end via cobra: stamp writes the timestamp, --blocked sets the
// suppression flag, --done clears the block and stamps again.
func TestIntegrationApplicationFollowUpViaCLI(t *testing.T) {
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

	applyOut := runCmd(ctx, t, "--non-interactive", "opportunity", "apply", oppID, "--json")
	appID := decodeApplicationJSON(t, applyOut).ID
	if appID == "" {
		t.Fatalf("apply returned empty id: %q", applyOut)
	}

	// Stamp via canonical noun-first form.
	stampOut := runCmd(ctx, t, "--non-interactive", "application", "follow-up", appID, "--json")
	stamp := decodeFollowUpJSON(t, stampOut)
	if stamp.FollowUpBlocked {
		t.Fatalf("after stamp: FollowUpBlocked = true, want false")
	}
	if stamp.LastFollowedUpAt == nil || *stamp.LastFollowedUpAt == "" {
		t.Fatalf("after stamp: LastFollowedUpAt empty: %+v", stamp)
	}

	// One follow_up event recorded against the application.
	var n int
	if err := st.Pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE opportunity_id = $1 AND kind = 'follow_up' AND application_id = $2`,
		oppID, appID).Scan(&n); err != nil {
		t.Fatalf("count follow_up: %v", err)
	}
	if n != 1 {
		t.Fatalf("follow_up events after stamp = %d, want 1", n)
	}

	// --blocked sets the flag without emitting another event.
	blockOut := runCmd(ctx, t, "--non-interactive", "application", "follow-up", appID, "--blocked", "--json")
	if !decodeFollowUpJSON(t, blockOut).FollowUpBlocked {
		t.Fatalf("after --blocked: FollowUpBlocked = false, want true")
	}
	if err := st.Pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE opportunity_id = $1 AND kind = 'follow_up'`,
		oppID).Scan(&n); err != nil {
		t.Fatalf("count follow_up after block: %v", err)
	}
	if n != 1 {
		t.Fatalf("follow_up events after --blocked = %d, want 1 (block does not emit)", n)
	}

	// --done clears the block and adds an event.
	doneOut := runCmd(ctx, t, "--non-interactive", "application", "follow-up", appID, "--done", "--json")
	done := decodeFollowUpJSON(t, doneOut)
	if done.FollowUpBlocked {
		t.Fatalf("after --done: FollowUpBlocked = true, want false")
	}
	if done.LastFollowedUpAt == nil || *done.LastFollowedUpAt == "" {
		t.Fatalf("after --done: LastFollowedUpAt empty: %+v", done)
	}
	if err := st.Pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE opportunity_id = $1 AND kind = 'follow_up'`,
		oppID).Scan(&n); err != nil {
		t.Fatalf("count follow_up after done: %v", err)
	}
	if n != 2 {
		t.Fatalf("follow_up events after --done = %d, want 2", n)
	}

	// --blocked + --done together is rejected before any DB write.
	if _, err := tryRun(ctx, t, "--non-interactive", "application", "follow-up", appID, "--blocked", "--done"); err == nil {
		t.Fatal("--blocked --done together: expected error")
	}
}

// TestIntegrationFollowUpAliasViaCLI proves the top-level `opps
// follow-up <id>` shortcut reaches the same RunE as the canonical
// noun-first `opps application follow-up <id>` — a thin wiring check
// that catches a missing AddCommand or a divergent factory.
func TestIntegrationFollowUpAliasViaCLI(t *testing.T) {
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

	applyOut := runCmd(ctx, t, "--non-interactive", "opportunity", "apply", oppID, "--json")
	appID := decodeApplicationJSON(t, applyOut).ID

	aliasOut := runCmd(ctx, t, "--non-interactive", "follow-up", appID, "--json")
	stamp := decodeFollowUpJSON(t, aliasOut)
	if stamp.LastFollowedUpAt == nil || *stamp.LastFollowedUpAt == "" {
		t.Fatalf("alias follow-up: LastFollowedUpAt empty: %+v", stamp)
	}
}

// TestIntegrationApplicationCRUDViaCLI drives `opps application
// {create,list,show,update,rm}` end-to-end against a real Postgres.
func TestIntegrationApplicationCRUDViaCLI(t *testing.T) {
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

	oppOut := runCmd(ctx, t, "--non-interactive", "opportunity", "create",
		"--company", companyID,
		"--role-title", "Staff Engineer",
		"--source", "outbound",
		"--office-days-per-week", "0",
		"--json")
	oppID := decodeOpportunityJSON(t, oppOut).ID

	// Create application via the canonical noun-first form, with all
	// fields supplied as flags.
	appliedAt := "2026-06-10T12:00:00Z"
	createOut := runCmd(ctx, t, "--non-interactive", "application", "create",
		"--opportunity", oppID,
		"--applied-at", appliedAt,
		"--applied-with-email", "me@example.test",
		"--notes", "applied via careers page",
		"--json")
	created := decodeApplicationJSON(t, createOut)
	if created.ID == "" {
		t.Fatalf("create returned empty id: %q", createOut)
	}
	if created.Status != "applied" {
		t.Fatalf("create status = %q, want applied", created.Status)
	}
	if created.OpportunityID != oppID {
		t.Fatalf("create opportunity_id = %q, want %q", created.OpportunityID, oppID)
	}

	// latest_status on the opportunity should be applied now.
	showOpp := runCmd(ctx, t, "opportunity", "show", oppID, "--json")
	if got := decodeOpportunityJSON(t, showOpp).LatestStatus; got != "applied" {
		t.Fatalf("opportunity latest_status = %q, want applied", got)
	}

	// list applications --json should contain exactly one row.
	listOut := runCmd(ctx, t, "application", "list", "--json")
	var listed []map[string]any
	if err := json.Unmarshal([]byte(listOut), &listed); err != nil {
		t.Fatalf("list json: %v\n%s", err, listOut)
	}
	if len(listed) != 1 || listed[0]["id"] != created.ID {
		t.Fatalf("list: want one row with id=%s, got %v", created.ID, listed)
	}

	// show <id> --json round-trips.
	showOut := runCmd(ctx, t, "application", "show", created.ID, "--json")
	if shown := decodeApplicationJSON(t, showOut); shown.ID != created.ID {
		t.Fatalf("show id = %q, want %q", shown.ID, created.ID)
	}

	// update notes; status must not change (status-machine columns are
	// off-limits to UpdateApplication).
	wantNotes := "recruiter replied — phone screen scheduled"
	updateOut := runCmd(ctx, t, "application", "update", created.ID,
		"--notes", wantNotes, "--json")
	updated := decodeApplicationJSON(t, updateOut)
	if updated.Status != "applied" {
		t.Fatalf("update status = %q, want applied (unchanged)", updated.Status)
	}
	if updated.Notes != wantNotes {
		t.Fatalf("update notes = %q, want %q", updated.Notes, wantNotes)
	}

	// rm without --yes must fail in non-interactive mode (no confirm
	// possible).
	if _, err := tryRun(ctx, t, "--non-interactive", "application", "rm", created.ID); !errors.Is(err, prompt.ErrNonInteractive) {
		t.Fatalf("rm without --yes in non-interactive: err=%v, want ErrNonInteractive", err)
	}

	// rm --yes removes the row.
	if _, err := tryRun(ctx, t, "application", "rm", created.ID, "--yes"); err != nil {
		t.Fatalf("rm: %v", err)
	}
	emptyOut := strings.TrimSpace(runCmd(ctx, t, "application", "list", "--json"))
	if emptyOut != "null" && emptyOut != "[]" {
		t.Fatalf("list after rm: want null/[], got %q", emptyOut)
	}

	// create without --opportunity in non-interactive mode must fail —
	// applications are permanent associations, so the picker refuses to
	// auto-select even when one opportunity exists.
	if _, err := tryRun(ctx, t, "--non-interactive", "application", "create"); !errors.Is(err, prompt.ErrNonInteractive) {
		t.Fatalf("create without --opportunity: err=%v, want ErrNonInteractive", err)
	}
}
