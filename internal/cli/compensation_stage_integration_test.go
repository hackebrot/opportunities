//go:build integration

package cli_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hackebrot/opportunities/internal/store"
)

// compensationShowShape is the subset of an embedded compensation payload
// the show tests assert on.
type compensationShowShape struct {
	Kind      string `json:"kind"`
	BaseMinor int64  `json:"base_minor"`
	Currency  string `json:"currency"`
}

// stageShowShape is the subset of an embedded stage payload the show tests
// assert on.
type stageShowShape struct {
	OrderIndex int    `json:"order_index"`
	Label      string `json:"label"`
}

// TestIntegrationOpportunityShowEmbedsCompensations proves `opps
// opportunity show --json` surfaces compensation rows linked to the
// opportunity. Rows are seeded through the store; show must read them
// without error, and an opportunity with none must still render an empty
// `compensations` array rather than failing.
func TestIntegrationOpportunityShowEmbedsCompensations(t *testing.T) {
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

	// Before any compensation exists, show still renders an empty array.
	var empty struct {
		Compensations []compensationShowShape `json:"compensations"`
	}
	emptyOut := runCmd(ctx, t, "opportunity", "show", oppID, "--json")
	if err := json.Unmarshal([]byte(emptyOut), &empty); err != nil {
		t.Fatalf("show (no comps) unmarshal: %v\n%s", err, emptyOut)
	}
	if empty.Compensations == nil {
		t.Fatalf("compensations: want [], got null\n%s", emptyOut)
	}
	if len(empty.Compensations) != 0 {
		t.Fatalf("compensations: want empty, got %+v", empty.Compensations)
	}

	// Seed a target compensation on the opportunity through the store, then
	// assert `opportunity show` surfaces it — the store is the seam the
	// fixture uses to place a row the read path must then embed.
	if _, err := st.InsertCompensation(ctx, st.Pool, store.CompensationParams{
		OpportunityID: &oppID,
		Kind:          "target",
		BaseMinor:     100,
		BonusMinor:    new(int64(50)),
		Currency:      "EUR",
		Notes:         "target placeholder",
	}); err != nil {
		t.Fatalf("insert compensation: %v", err)
	}

	var got struct {
		ID            string                  `json:"id"`
		Compensations []compensationShowShape `json:"compensations"`
	}
	showOut := runCmd(ctx, t, "opportunity", "show", oppID, "--json")
	if err := json.Unmarshal([]byte(showOut), &got); err != nil {
		t.Fatalf("show unmarshal: %v\n%s", err, showOut)
	}
	if got.ID != oppID {
		t.Fatalf("show id = %q, want %q", got.ID, oppID)
	}
	if len(got.Compensations) != 1 {
		t.Fatalf("compensations: want 1 row, got %+v", got.Compensations)
	}
	want := compensationShowShape{Kind: "target", BaseMinor: 100, Currency: "EUR"}
	if got.Compensations[0] != want {
		t.Fatalf("compensation = %+v, want %+v", got.Compensations[0], want)
	}
}

// TestIntegrationApplicationShowEmbedsStagesAndCompensations proves `opps
// application show --json` surfaces both the interview-pipeline stages and
// the application-linked compensations. Both are seeded through the store;
// show must read them without error.
func TestIntegrationApplicationShowEmbedsStagesAndCompensations(t *testing.T) {
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
		"--office-days-per-week", "0",
		"--source", "outbound",
		"--json")
	oppID := decodeOpportunityJSON(t, createOut).ID

	applyOut := runCmd(ctx, t, "--non-interactive", "opportunity", "apply", oppID, "--json")
	appID := decodeApplicationJSON(t, applyOut).ID

	// Seed a stage and an application-linked compensation through the store,
	// the seam the fixtures use to place rows the read path must embed.
	if _, err := st.InsertApplicationStage(ctx, st.Pool, store.ApplicationStageParams{
		ApplicationID:    appID,
		OrderIndex:       1,
		Label:            "Recruiter screen",
		ExpectationNotes: "30 min intro",
	}); err != nil {
		t.Fatalf("insert stage: %v", err)
	}
	if _, err := st.InsertCompensation(ctx, st.Pool, store.CompensationParams{
		ApplicationID: &appID,
		Kind:          "offered",
		BaseMinor:     300,
		Currency:      "EUR",
	}); err != nil {
		t.Fatalf("insert compensation: %v", err)
	}

	var got struct {
		ID            string                  `json:"id"`
		Stages        []stageShowShape        `json:"stages"`
		Compensations []compensationShowShape `json:"compensations"`
	}
	showOut := runCmd(ctx, t, "application", "show", appID, "--json")
	if err := json.Unmarshal([]byte(showOut), &got); err != nil {
		t.Fatalf("show unmarshal: %v\n%s", err, showOut)
	}
	if got.ID != appID {
		t.Fatalf("show id = %q, want %q", got.ID, appID)
	}
	if len(got.Stages) != 1 {
		t.Fatalf("stages: want 1 row, got %+v", got.Stages)
	}
	if wantStage := (stageShowShape{OrderIndex: 1, Label: "Recruiter screen"}); got.Stages[0] != wantStage {
		t.Fatalf("stage = %+v, want %+v", got.Stages[0], wantStage)
	}
	if len(got.Compensations) != 1 {
		t.Fatalf("compensations: want 1 row, got %+v", got.Compensations)
	}
	if wantComp := (compensationShowShape{Kind: "offered", BaseMinor: 300, Currency: "EUR"}); got.Compensations[0] != wantComp {
		t.Fatalf("compensation = %+v, want %+v", got.Compensations[0], wantComp)
	}
}
