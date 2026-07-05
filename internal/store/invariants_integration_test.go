//go:build integration

package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestIntegrationSchemaInvariants exercises the CHECK constraints and
// partial unique index from 00001_init.sql by attempting violating
// inserts and asserting that Postgres rejects them with the right
// SQLSTATE.
//
// Subtests share one Postgres container and an unrolled-back schema —
// each subtest namespaces its inserts by company slug ("acme",
// "becme", ...) instead of truncating between runs. New subtests must
// either follow that pattern or query with a WHERE clause scoped to
// their own opportunity_id; an unscoped SELECT would see prior rows.
func TestIntegrationSchemaInvariants(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store := startPostgresStore(ctx, t)
	if err := store.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	pool := store.Pool

	t.Run("office_days_per_week rejects 6", func(t *testing.T) {
		companyID := insertCompany(ctx, t, pool, "acme")
		_, err := pool.Exec(ctx, `
			INSERT INTO opportunities (company_id, office_days_per_week,
				source, priority, latest_status)
			VALUES ($1, 6, 'outbound', 'normal', 'watching')
		`, companyID)
		assertCheckViolation(t, err, "opportunities_office_days_chk")
	})

	t.Run("currency rejects GBP", func(t *testing.T) {
		companyID := insertCompany(ctx, t, pool, "becme")
		oppID := insertOpportunity(ctx, t, pool, companyID)
		_, err := pool.Exec(ctx, `
			INSERT INTO compensations (opportunity_id, kind, base_minor, currency)
			VALUES ($1, 'target', 1000000, 'GBP')
		`, oppID)
		assertCheckViolation(t, err, "compensations_currency_chk")
	})

	t.Run("compensations XOR rejects both targets set", func(t *testing.T) {
		companyID := insertCompany(ctx, t, pool, "cecme")
		oppID := insertOpportunity(ctx, t, pool, companyID)
		appID := insertApplication(ctx, t, pool, oppID, "applied")
		_, err := pool.Exec(ctx, `
			INSERT INTO compensations (opportunity_id, application_id,
				kind, base_minor, currency)
			VALUES ($1, $2, 'target', 1000000, 'EUR')
		`, oppID, appID)
		assertCheckViolation(t, err, "compensations_target_xor_chk")
	})

	t.Run("events.label required for custom, forbidden otherwise", func(t *testing.T) {
		companyID := insertCompany(ctx, t, pool, "decme")
		oppID := insertOpportunity(ctx, t, pool, companyID)

		_, err := pool.Exec(ctx, `
			INSERT INTO events (opportunity_id, kind, occurred_at, label)
			VALUES ($1, 'note', now(), 'stray label')
		`, oppID)
		assertCheckViolation(t, err, "events_label_only_for_custom_chk")

		_, err = pool.Exec(ctx, `
			INSERT INTO events (opportunity_id, kind, occurred_at)
			VALUES ($1, 'custom', now())
		`, oppID)
		assertCheckViolation(t, err, "events_label_only_for_custom_chk")
	})

	t.Run("archive_reason_category must match status", func(t *testing.T) {
		companyID := insertCompany(ctx, t, pool, "eecme")
		oppID := insertOpportunity(ctx, t, pool, companyID)

		_, err := pool.Exec(ctx, `
			INSERT INTO applications (opportunity_id, status, archive_reason_category)
			VALUES ($1, 'applied', 'other')
		`, oppID)
		assertCheckViolation(t, err, "applications_archive_reason_chk")

		_, err = pool.Exec(ctx, `
			INSERT INTO applications (opportunity_id, status, archive_reason_category)
			VALUES ($1, 'rejected', 'compensation')
		`, oppID)
		assertCheckViolation(t, err, "applications_archive_reason_chk")
	})

	t.Run("partial unique index blocks two active applications", func(t *testing.T) {
		companyID := insertCompany(ctx, t, pool, "fecme")
		oppID := insertOpportunity(ctx, t, pool, companyID)

		insertApplication(ctx, t, pool, oppID, "applied")

		_, err := pool.Exec(ctx, `
			INSERT INTO applications (opportunity_id, status)
			VALUES ($1, 'in_progress')
		`, oppID)
		assertUniqueViolation(t, err, "uq_active_app_per_opportunity")

		// Once the first row reaches a terminal status it falls out of
		// the partial index and a new active application is allowed.
		if _, err := pool.Exec(ctx, `
			UPDATE applications
			SET status = 'rejected',
			    archive_reason_category = 'process_ended',
			    archived_at = now()
			WHERE opportunity_id = $1 AND status = 'applied'
		`, oppID); err != nil {
			t.Fatalf("retire first application: %v", err)
		}
		insertApplication(ctx, t, pool, oppID, "applied")
	})

	t.Run("events composite FK rejects mismatched opportunity", func(t *testing.T) {
		companyID := insertCompany(ctx, t, pool, "gecme")
		oppA := insertOpportunity(ctx, t, pool, companyID)
		oppB := insertOpportunity(ctx, t, pool, companyID)
		appOnA := insertApplication(ctx, t, pool, oppA, "applied")

		_, err := pool.Exec(ctx, `
			INSERT INTO events (opportunity_id, application_id, kind, occurred_at)
			VALUES ($1, $2, 'note', now())
		`, oppB, appOnA)
		assertForeignKeyViolation(t, err, "events_application_belongs_to_opportunity_fk")
	})

	t.Run("stage_id requires application_id", func(t *testing.T) {
		companyID := insertCompany(ctx, t, pool, "hecme")
		oppID := insertOpportunity(ctx, t, pool, companyID)

		// stage_id set while application_id is NULL trips the CHECK; the
		// composite FK short-circuits here (MATCH SIMPLE skips NULL columns).
		_, err := pool.Exec(ctx, `
			INSERT INTO events (opportunity_id, stage_id, kind, occurred_at)
			VALUES ($1, gen_random_uuid(), 'stage_scheduled', now())
		`, oppID)
		assertCheckViolation(t, err, "events_stage_requires_app_chk")
	})

	t.Run("stage_id must belong to the event's application", func(t *testing.T) {
		companyID := insertCompany(ctx, t, pool, "iecme")
		oppA := insertOpportunity(ctx, t, pool, companyID)
		appA := insertApplication(ctx, t, pool, oppA, "applied")
		oppB := insertOpportunity(ctx, t, pool, companyID)
		appB := insertApplication(ctx, t, pool, oppB, "applied")
		stageOnA := insertApplicationStage(ctx, t, pool, appA)

		// An event on appB pointing at appA's stage: (appB, stageOnA) has no
		// match in application_stages(application_id, id).
		_, err := pool.Exec(ctx, `
			INSERT INTO events (opportunity_id, application_id, stage_id, kind, occurred_at)
			VALUES ($1, $2, $3, 'stage_scheduled', now())
		`, oppB, appB, stageOnA)
		assertForeignKeyViolation(t, err, "events_stage_belongs_to_app_fk")
	})

	t.Run("stage-linked event for its own application is allowed", func(t *testing.T) {
		companyID := insertCompany(ctx, t, pool, "jecme")
		oppID := insertOpportunity(ctx, t, pool, companyID)
		appID := insertApplication(ctx, t, pool, oppID, "applied")
		stageID := insertApplicationStage(ctx, t, pool, appID)

		if _, err := pool.Exec(ctx, `
			INSERT INTO events (opportunity_id, application_id, stage_id, kind, occurred_at)
			VALUES ($1, $2, $3, 'stage_scheduled', now())
		`, oppID, appID, stageID); err != nil {
			t.Fatalf("valid stage-linked event rejected: %v", err)
		}
	})
}

// TestIntegrationSchemaColumnTypes pins the storage-representation
// invariants that the CHECK-focused test above doesn't touch: every
// timestamp column is timestamptz (a bare timestamp would silently drop
// the zone and corrupt the stored instant), compensation money is bigint
// minor units, and every id is a uuid defaulted from gen_random_uuid().
// These introspect information_schema, so a migration that changed a
// column's declared type would fail here.
func TestIntegrationSchemaColumnTypes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store := startPostgresStore(ctx, t)
	if err := store.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	pool := store.Pool

	t.Run("every timestamp column is timestamptz", func(t *testing.T) {
		rows, err := pool.Query(ctx, `
			SELECT table_name, column_name, data_type
			FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name <> 'goose_db_version'  -- goose's own bookkeeping table, not our schema
			  AND data_type LIKE 'timestamp%'
			ORDER BY table_name, column_name
		`)
		if err != nil {
			t.Fatalf("query timestamp columns: %v", err)
		}
		defer rows.Close()

		found := 0
		for rows.Next() {
			var table, column, dataType string
			if err := rows.Scan(&table, &column, &dataType); err != nil {
				t.Fatalf("scan: %v", err)
			}
			found++
			if dataType != "timestamp with time zone" {
				t.Errorf("%s.%s is %q, want \"timestamp with time zone\"", table, column, dataType)
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("iterate: %v", err)
		}
		if found == 0 {
			t.Fatal("found no timestamp columns — the query is wrong or the schema is empty")
		}
	})

	t.Run("compensation money columns are bigint", func(t *testing.T) {
		for _, column := range []string{"base_minor", "bonus_minor"} {
			var dataType string
			err := pool.QueryRow(ctx, `
				SELECT data_type FROM information_schema.columns
				WHERE table_schema = 'public'
				  AND table_name = 'compensations' AND column_name = $1
			`, column).Scan(&dataType)
			if err != nil {
				t.Fatalf("query compensations.%s: %v", column, err)
			}
			if dataType != "bigint" {
				t.Errorf("compensations.%s is %q, want \"bigint\"", column, dataType)
			}
		}
	})

	t.Run("every id column is uuid from gen_random_uuid", func(t *testing.T) {
		rows, err := pool.Query(ctx, `
			SELECT table_name, data_type, column_default
			FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name <> 'goose_db_version'  -- goose's own bookkeeping table, not our schema
			  AND column_name = 'id'
			ORDER BY table_name
		`)
		if err != nil {
			t.Fatalf("query id columns: %v", err)
		}
		defer rows.Close()

		found := 0
		for rows.Next() {
			var table, dataType string
			var columnDefault *string
			if err := rows.Scan(&table, &dataType, &columnDefault); err != nil {
				t.Fatalf("scan: %v", err)
			}
			found++
			if dataType != "uuid" {
				t.Errorf("%s.id is %q, want \"uuid\"", table, dataType)
			}
			if columnDefault == nil || !strings.Contains(*columnDefault, "gen_random_uuid()") {
				def := "NULL"
				if columnDefault != nil {
					def = *columnDefault
				}
				t.Errorf("%s.id default is %q, want it to call gen_random_uuid()", table, def)
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("iterate: %v", err)
		}
		if found == 0 {
			t.Fatal("found no id columns — the query is wrong or the schema is empty")
		}
	})

	t.Run("timestamptz column persists the instant, not the naive clock time", func(t *testing.T) {
		companyID := insertCompany(ctx, t, pool, "acme")
		oppID := insertOpportunity(ctx, t, pool, companyID)
		if _, err := pool.Exec(ctx, `
			INSERT INTO events (opportunity_id, kind, occurred_at)
			VALUES ($1, 'note', '2026-06-01 14:00:00+02:00')
		`, oppID); err != nil {
			t.Fatalf("insert event: %v", err)
		}
		var got time.Time
		if err := pool.QueryRow(ctx, `
			SELECT occurred_at FROM events WHERE opportunity_id = $1
		`, oppID).Scan(&got); err != nil {
			t.Fatalf("read occurred_at: %v", err)
		}
		want := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("occurred_at round-tripped to %s, want the same instant as %s "+
				"(a naive timestamp column would have dropped the +02:00 offset)",
				got.UTC().Format(time.RFC3339), want.Format(time.RFC3339))
		}
	})
}

// --- helpers ---------------------------------------------------------------

func insertCompany(ctx context.Context, t *testing.T, pool *pgxpool.Pool, slug string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO companies (name, slug) VALUES ($1, $1) RETURNING id
	`, slug).Scan(&id)
	if err != nil {
		t.Fatalf("insert company %q: %v", slug, err)
	}
	return id
}

func insertOpportunity(ctx context.Context, t *testing.T, pool *pgxpool.Pool, companyID string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO opportunities (company_id, office_days_per_week,
			source, priority, latest_status)
		VALUES ($1, 0, 'outbound', 'normal', 'watching')
		RETURNING id
	`, companyID).Scan(&id)
	if err != nil {
		t.Fatalf("insert opportunity: %v", err)
	}
	return id
}

func insertApplication(ctx context.Context, t *testing.T, pool *pgxpool.Pool, oppID, status string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO applications (opportunity_id, status)
		VALUES ($1, $2)
		RETURNING id
	`, oppID, status).Scan(&id)
	if err != nil {
		t.Fatalf("insert application: %v", err)
	}
	return id
}

func insertApplicationStage(ctx context.Context, t *testing.T, pool *pgxpool.Pool, appID string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO application_stages (application_id, order_index, label)
		VALUES ($1, 0, 'Screen')
		RETURNING id
	`, appID).Scan(&id)
	if err != nil {
		t.Fatalf("insert application stage: %v", err)
	}
	return id
}

// SQLSTATE codes — inlined rather than pulling pgerrcode for four constants.
const (
	sqlstateCheckViolation      = "23514"
	sqlstateUniqueViolation     = "23505"
	sqlstateForeignKeyViolation = "23503"
)

func assertCheckViolation(t *testing.T, err error, constraintName string) {
	t.Helper()
	assertPGError(t, err, sqlstateCheckViolation, constraintName)
}

func assertUniqueViolation(t *testing.T, err error, constraintName string) {
	t.Helper()
	assertPGError(t, err, sqlstateUniqueViolation, constraintName)
}

func assertForeignKeyViolation(t *testing.T, err error, constraintName string) {
	t.Helper()
	assertPGError(t, err, sqlstateForeignKeyViolation, constraintName)
}

func assertPGError(t *testing.T, err error, wantSQLSTATE, wantConstraint string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with SQLSTATE %s on %q, got nil", wantSQLSTATE, wantConstraint)
	}
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	if !ok {
		t.Fatalf("expected *pgconn.PgError, got %T: %v", err, err)
	}
	if pgErr.Code != wantSQLSTATE {
		t.Fatalf("want SQLSTATE %s, got %s (message: %s)", wantSQLSTATE, pgErr.Code, pgErr.Message)
	}
	if pgErr.ConstraintName == wantConstraint {
		return
	}
	if strings.Contains(pgErr.Message, wantConstraint) {
		return
	}
	t.Fatalf("want constraint %q, got constraint=%q message=%q",
		wantConstraint, pgErr.ConstraintName, pgErr.Message)
}
