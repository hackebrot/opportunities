//go:build integration

package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hackebrot/opportunities/internal/service"
	"github.com/hackebrot/opportunities/internal/store"
)

// TestIntegrationAddApplication proves AddApplication writes the
// application (status "applied") and an `applied` event linked to the
// new application_id in one tx, flips the opportunity's latest_status
// to "applied", and pins the event timestamp via the injected clock.
func TestIntegrationAddApplication(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)
	oppID := seedOpportunity(ctx, t, svc)

	appliedAt := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	app, err := svc.AddApplication(ctx, service.ApplicationCreationInput{
		Application: service.ApplicationInput{
			OpportunityID:    oppID,
			AppliedAt:        &appliedAt,
			AppliedWithEmail: "me@example.test",
			Notes:            "applied via careers page",
		},
	})
	if err != nil {
		t.Fatalf("add application: %v", err)
	}
	if app.Status != "applied" {
		t.Fatalf("application status = %q, want applied", app.Status)
	}
	if app.OpportunityID != oppID {
		t.Fatalf("application opportunity_id = %q, want %q", app.OpportunityID, oppID)
	}

	// latest_status flips to applied.
	opp, err := st.GetOpportunity(ctx, oppID)
	if err != nil {
		t.Fatalf("get opportunity: %v", err)
	}
	if opp.LatestStatus != "applied" {
		t.Fatalf("latest_status = %q, want applied", opp.LatestStatus)
	}

	// Exactly one `applied` event with application_id set to the new app
	// and occurred_at pinned to the injected clock.
	var (
		count      int
		kind       string
		occurredAt time.Time
		eventAppID *string
	)
	err = st.Pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE kind = 'applied') OVER (), kind, occurred_at, application_id
		FROM events
		WHERE opportunity_id = $1 AND kind = 'applied'`, oppID).
		Scan(&count, &kind, &occurredAt, &eventAppID)
	if err != nil {
		t.Fatalf("query applied event: %v", err)
	}
	if count != 1 {
		t.Fatalf("applied event count = %d, want 1", count)
	}
	if eventAppID == nil || *eventAppID != app.ID {
		t.Fatalf("applied event application_id = %v, want %q", eventAppID, app.ID)
	}
	if !occurredAt.Equal(testClock.t) {
		t.Fatalf("applied event occurred_at = %s, want %s", occurredAt, testClock.t)
	}
}

// TestIntegrationAddApplicationInlineGraph proves the inline-create chain
// lands the whole graph in one transaction: a company, an opportunity,
// its "added" event, the application (status "applied"), and the matching
// "applied" event — with the opportunity's latest_status flipped to
// "applied". This is the SPEC "[+ New …] … one transaction" promise,
// exercised through AddApplication rather than AddOpportunity.
func TestIntegrationAddApplicationInlineGraph(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)

	app, err := svc.AddApplication(ctx, service.ApplicationCreationInput{
		Application: service.ApplicationInput{AppliedWithEmail: "me@example.test"},
		Opportunity: &service.OpportunityCreationInput{
			Company: service.OpportunityCompanyChoice{
				New: &service.CompanyInput{Name: "Acme Corp"},
			},
			Opportunity: service.OpportunityInput{
				RoleTitle: "Member of Technical Staff",
				Source:    "inbound_founder",
			},
		},
	})
	if err != nil {
		t.Fatalf("AddApplication: %v", err)
	}
	if app.Status != "applied" {
		t.Fatalf("application status = %q, want applied", app.Status)
	}

	// One company, one opportunity, one application — all from the single
	// inline call.
	for table, want := range map[string]int{
		"companies":     1,
		"opportunities": 1,
		"applications":  1,
	} {
		var n int
		if err := st.Pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != want {
			t.Fatalf("%s = %d, want %d", table, n, want)
		}
	}

	// The application points at the inline-created opportunity, and that
	// opportunity's latest_status has flipped to applied.
	opp, err := st.GetOpportunity(ctx, app.OpportunityID)
	if err != nil {
		t.Fatalf("get opportunity: %v", err)
	}
	if opp.CompanyName != "Acme Corp" {
		t.Fatalf("company name = %q, want Acme Corp", opp.CompanyName)
	}
	if opp.LatestStatus != "applied" {
		t.Fatalf("latest_status = %q, want applied", opp.LatestStatus)
	}

	// Exactly one "added" event (from the opportunity insert) and one
	// "applied" event linked to the new application.
	var added, applied int
	if err := st.Pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE kind = 'added'),
			count(*) FILTER (WHERE kind = 'applied' AND application_id = $2)
		FROM events WHERE opportunity_id = $1`, app.OpportunityID, app.ID).
		Scan(&added, &applied); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if added != 1 {
		t.Fatalf("added events = %d, want 1", added)
	}
	if applied != 1 {
		t.Fatalf("applied events = %d, want 1", applied)
	}
}

// TestIntegrationAddApplicationRollsBackOnFailure proves the cross-entity
// atomicity guarantee: when a later step of an inline AddApplication fails
// (here, a dangling existing-contact id in the embedded opportunity graph,
// reached only after the company and opportunity rows are already
// inserted), the whole transaction rolls back. No orphan opportunity (or
// company, event, application) survives — the regression the SPEC's "one
// transaction" promise guards against.
func TestIntegrationAddApplicationRollsBackOnFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)

	_, err := svc.AddApplication(ctx, service.ApplicationCreationInput{
		Opportunity: &service.OpportunityCreationInput{
			Company: service.OpportunityCompanyChoice{
				New: &service.CompanyInput{Name: "Example Corp"},
			},
			Opportunity: service.OpportunityInput{Source: "outbound"},
			Contact: &service.OpportunityContactChoice{
				ID:           "00000000-0000-0000-0000-000000000000",
				Relationship: "recruiter",
			},
		},
	})
	if err == nil {
		t.Fatal("expected error on dangling contact id")
	}

	for _, table := range []string{"companies", "opportunities", "events", "contacts", "opportunity_contacts", "applications"} {
		var rowCount int
		if err := st.Pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&rowCount); err != nil {
			t.Errorf("row count in %s: %v", table, err)
			continue
		}
		if rowCount != 0 {
			t.Errorf("row count in %s after rollback = %d, want 0", table, rowCount)
		}
	}
}

// TestIntegrationAddApplicationActiveExists proves back-to-back
// AddApplication calls on the same opportunity collapse to one win plus
// store.ErrActiveExists — the partial unique index is the contract.
func TestIntegrationAddApplicationActiveExists(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)
	oppID := seedOpportunity(ctx, t, svc)

	if _, err := svc.AddApplication(ctx, service.ApplicationCreationInput{Application: service.ApplicationInput{OpportunityID: oppID}}); err != nil {
		t.Fatalf("first add: %v", err)
	}
	_, err := svc.AddApplication(ctx, service.ApplicationCreationInput{Application: service.ApplicationInput{OpportunityID: oppID}})
	if !errors.Is(err, store.ErrActiveExists) {
		t.Fatalf("second add: want store.ErrActiveExists, got %v", err)
	}

	// And nothing in events for the second attempt: still exactly one
	// `applied` event, the one from the first add.
	var n int
	if err := st.Pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE opportunity_id = $1 AND kind = 'applied'`, oppID).
		Scan(&n); err != nil {
		t.Fatalf("count applied events: %v", err)
	}
	if n != 1 {
		t.Fatalf("applied events after conflict = %d, want 1", n)
	}
}

// TestIntegrationAddApplicationReapply proves the re-application path:
// once the first application terminates (status flipped out of the
// active set), AddApplication succeeds again on the same opportunity.
func TestIntegrationAddApplicationReapply(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)
	oppID := seedOpportunity(ctx, t, svc)

	first, err := svc.AddApplication(ctx, service.ApplicationCreationInput{Application: service.ApplicationInput{OpportunityID: oppID}})
	if err != nil {
		t.Fatalf("first add: %v", err)
	}

	// Terminate the first app through the events engine: a rejected
	// event flips the application to a terminal status, freeing the
	// partial-index slot for the re-application below.
	if _, err := svc.AppendEvent(ctx, service.EventInput{
		OpportunityID:         oppID,
		Kind:                  "rejected",
		ArchiveReasonCategory: "process_ended",
	}); err != nil {
		t.Fatalf("reject first: %v", err)
	}

	second, err := svc.AddApplication(ctx, service.ApplicationCreationInput{Application: service.ApplicationInput{OpportunityID: oppID}})
	if err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	if second.ID == first.ID {
		t.Fatalf("re-apply returned the same row id; want a fresh application")
	}
	if second.Status != "applied" {
		t.Fatalf("re-apply status = %q, want applied", second.Status)
	}
	if got := readOpportunityLatestStatus(ctx, t, st, oppID); got != "applied" {
		t.Fatalf("opportunity latest_status after re-apply = %q, want applied", got)
	}
}

// TestIntegrationAddApplicationConcurrent is the partial-index race
// regression: two goroutines calling AddApplication on the same
// opportunity collapse to exactly one win and one store.ErrActiveExists.
// The service-layer "any active app?" check is best-effort; the index
// is the authority.
func TestIntegrationAddApplicationConcurrent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)
	oppID := seedOpportunity(ctx, t, svc)

	var (
		wg      sync.WaitGroup
		results = make([]error, 2)
	)
	start := make(chan struct{})
	for i := range results {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			_, err := svc.AddApplication(ctx, service.ApplicationCreationInput{Application: service.ApplicationInput{OpportunityID: oppID}})
			results[idx] = err
		}(i)
	}
	close(start)
	wg.Wait()

	var wins, losses int
	for _, err := range results {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, store.ErrActiveExists):
			losses++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if wins != 1 || losses != 1 {
		t.Fatalf("race: wins=%d losses=%d, want 1/1 (results=%v)", wins, losses, results)
	}

	// Exactly one row in applications, exactly one `applied` event.
	var nApps int
	if err := st.Pool.QueryRow(ctx,
		`SELECT count(*) FROM applications WHERE opportunity_id = $1`, oppID).
		Scan(&nApps); err != nil {
		t.Fatalf("count applications: %v", err)
	}
	if nApps != 1 {
		t.Fatalf("applications after race = %d, want 1", nApps)
	}
	var nEvents int
	if err := st.Pool.QueryRow(ctx,
		`SELECT count(*) FROM events WHERE opportunity_id = $1 AND kind = 'applied'`, oppID).
		Scan(&nEvents); err != nil {
		t.Fatalf("count applied events: %v", err)
	}
	if nEvents != 1 {
		t.Fatalf("applied events after race = %d, want 1", nEvents)
	}
}

// TestIntegrationAddApplicationUnknownOpportunity proves AddApplication
// rejects an unknown opportunity id with store.ErrNotFound and leaves
// no application or event behind.
func TestIntegrationAddApplicationUnknownOpportunity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)

	_, err := svc.AddApplication(ctx, service.ApplicationCreationInput{
		Application: service.ApplicationInput{
			OpportunityID: "00000000-0000-0000-0000-000000000000",
		},
	})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown opportunity: want store.ErrNotFound, got %v", err)
	}

	for _, table := range []string{"applications", "events"} {
		var n int
		if err := st.Pool.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != 0 {
			t.Fatalf("%s after failed add = %d, want 0", table, n)
		}
	}
}

// TestIntegrationAddApplicationArchivedOpportunity proves AddApplication
// rejects an archived opportunity with ErrPrecondition.
func TestIntegrationAddApplicationArchivedOpportunity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)
	oppID := seedOpportunity(ctx, t, svc)

	if _, err := svc.ArchiveOpportunity(ctx, oppID, "delisted"); err != nil {
		t.Fatalf("archive: %v", err)
	}

	_, err := svc.AddApplication(ctx, service.ApplicationCreationInput{Application: service.ApplicationInput{OpportunityID: oppID}})
	if !errors.Is(err, service.ErrPrecondition) {
		t.Fatalf("apply to archived: want ErrPrecondition, got %v", err)
	}

	// The tx must roll back: no application row landed.
	var n int
	if err := st.Pool.QueryRow(ctx,
		`SELECT count(*) FROM applications WHERE opportunity_id = $1`, oppID).
		Scan(&n); err != nil {
		t.Fatalf("count applications: %v", err)
	}
	if n != 0 {
		t.Fatalf("applications after archived-opp reject = %d, want 0", n)
	}
}

// TestIntegrationAddApplicationValidation proves invalid input is
// rejected before any DB write.
func TestIntegrationAddApplicationValidation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)

	if _, err := svc.AddApplication(ctx, service.ApplicationCreationInput{}); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("missing opportunity: want ErrValidation, got %v", err)
	}

	var n int
	if err := st.Pool.QueryRow(ctx, `SELECT count(*) FROM applications`).Scan(&n); err != nil {
		t.Fatalf("count applications: %v", err)
	}
	if n != 0 {
		t.Fatalf("applications after validation failure = %d, want 0", n)
	}
}

// TestIntegrationUpdateApplicationRejectsReparent pins the immutability
// invariant on UpdateApplication: handing in a different opportunity_id
// must surface ErrPrecondition without touching the row. Re-parenting
// would orphan the events already written against the original
// opportunity, so the service rejects rather than the CLI surface alone.
func TestIntegrationUpdateApplicationRejectsReparent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	svc := service.New(st, testClock)

	oppID := seedOpportunity(ctx, t, svc)
	app, err := svc.AddApplication(ctx, service.ApplicationCreationInput{Application: service.ApplicationInput{OpportunityID: oppID}})
	if err != nil {
		t.Fatalf("add application: %v", err)
	}

	// A second opportunity on the same company is the target of the
	// attempted re-parent.
	opp, err := st.GetOpportunity(ctx, oppID)
	if err != nil {
		t.Fatalf("get opportunity: %v", err)
	}
	otherOpp, err := svc.AddOpportunity(ctx, service.OpportunityCreationInput{
		Company:     service.OpportunityCompanyChoice{ID: opp.CompanyID},
		Opportunity: service.OpportunityInput{Source: "outbound"},
	})
	if err != nil {
		t.Fatalf("add second opportunity: %v", err)
	}

	_, err = svc.UpdateApplication(ctx, app.ID, service.ApplicationInput{
		OpportunityID: otherOpp.ID,
		Notes:         "attempted re-parent",
	})
	if !errors.Is(err, service.ErrPrecondition) {
		t.Fatalf("re-parent: err=%v, want ErrPrecondition", err)
	}

	// The row stays put.
	after, err := st.GetApplication(ctx, app.ID)
	if err != nil {
		t.Fatalf("get application: %v", err)
	}
	if after.OpportunityID != oppID {
		t.Fatalf("opportunity_id after rejected update = %q, want %q", after.OpportunityID, oppID)
	}
	if after.Notes != "" {
		t.Fatalf("notes after rejected update = %q, want empty", after.Notes)
	}

	// Same OpportunityID on the input is a no-op write, not a rejection.
	updated, err := svc.UpdateApplication(ctx, app.ID, service.ApplicationInput{
		OpportunityID: oppID,
		Notes:         "ok",
	})
	if err != nil {
		t.Fatalf("preserving opportunity_id: %v", err)
	}
	if updated.Notes != "ok" {
		t.Fatalf("notes after no-op opportunity update = %q, want %q", updated.Notes, "ok")
	}
}
