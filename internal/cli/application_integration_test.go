//go:build integration

package cli_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"
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
