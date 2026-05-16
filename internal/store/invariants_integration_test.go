//go:build integration

package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// TestIntegrationSchemaInvariants exercises the CHECK constraints and
// partial unique index from 00001_init.sql by attempting violating
// inserts and asserting that Postgres rejects them with the right
// SQLSTATE. The migration is applied once for the whole subtest tree.
func TestIntegrationSchemaInvariants(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	sqlDB := startPostgres(ctx, t)
	if err := migrateUp(ctx, sqlDB); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	// Each subtest gets its own freshly seeded company + opportunity so
	// failures are isolated from one another.
	t.Run("office_days_per_week rejects 6", func(t *testing.T) {
		companyID := insertCompany(ctx, t, sqlDB, "acme")
		_, err := sqlDB.ExecContext(ctx, `
			INSERT INTO opportunities (company_id, office_days_per_week,
				source, priority, latest_status)
			VALUES ($1, 6, 'outbound', 'normal', 'watching')
		`, companyID)
		assertCheckViolation(t, err, "opportunities_office_days_chk")
	})

	t.Run("currency rejects GBP", func(t *testing.T) {
		companyID := insertCompany(ctx, t, sqlDB, "becme")
		oppID := insertOpportunity(ctx, t, sqlDB, companyID)
		_, err := sqlDB.ExecContext(ctx, `
			INSERT INTO compensations (opportunity_id, kind, base_minor, currency)
			VALUES ($1, 'target', 1000000, 'GBP')
		`, oppID)
		assertCheckViolation(t, err, "compensations_currency_chk")
	})

	t.Run("compensations XOR rejects both targets set", func(t *testing.T) {
		companyID := insertCompany(ctx, t, sqlDB, "cecme")
		oppID := insertOpportunity(ctx, t, sqlDB, companyID)
		appID := insertApplication(ctx, t, sqlDB, oppID, "applied")
		_, err := sqlDB.ExecContext(ctx, `
			INSERT INTO compensations (opportunity_id, application_id,
				kind, base_minor, currency)
			VALUES ($1, $2, 'target', 1000000, 'EUR')
		`, oppID, appID)
		assertCheckViolation(t, err, "compensations_target_xor_chk")
	})

	t.Run("events.label required for custom, forbidden otherwise", func(t *testing.T) {
		companyID := insertCompany(ctx, t, sqlDB, "decme")
		oppID := insertOpportunity(ctx, t, sqlDB, companyID)

		// Non-custom event with a label → rejected.
		_, err := sqlDB.ExecContext(ctx, `
			INSERT INTO events (opportunity_id, kind, occurred_at, label)
			VALUES ($1, 'note', now(), 'stray label')
		`, oppID)
		assertCheckViolation(t, err, "events_label_only_for_custom_chk")

		// Custom event without a label → rejected.
		_, err = sqlDB.ExecContext(ctx, `
			INSERT INTO events (opportunity_id, kind, occurred_at)
			VALUES ($1, 'custom', now())
		`, oppID)
		assertCheckViolation(t, err, "events_label_only_for_custom_chk")
	})

	t.Run("archive_reason_category must match status", func(t *testing.T) {
		companyID := insertCompany(ctx, t, sqlDB, "eecme")
		oppID := insertOpportunity(ctx, t, sqlDB, companyID)

		// applied + a category set → rejected (only terminal statuses get one).
		_, err := sqlDB.ExecContext(ctx, `
			INSERT INTO applications (opportunity_id, status, archive_reason_category)
			VALUES ($1, 'applied', 'other')
		`, oppID)
		assertCheckViolation(t, err, "applications_archive_reason_chk")

		// rejected status + a 'compensation' category (declined-only value) → rejected.
		_, err = sqlDB.ExecContext(ctx, `
			INSERT INTO applications (opportunity_id, status, archive_reason_category)
			VALUES ($1, 'rejected', 'compensation')
		`, oppID)
		assertCheckViolation(t, err, "applications_archive_reason_chk")
	})

	t.Run("partial unique index blocks two active applications", func(t *testing.T) {
		companyID := insertCompany(ctx, t, sqlDB, "fecme")
		oppID := insertOpportunity(ctx, t, sqlDB, companyID)

		// First active application is fine.
		insertApplication(ctx, t, sqlDB, oppID, "applied")

		// Second active application on the same opportunity is rejected by
		// the partial unique index (SQLSTATE 23505).
		_, err := sqlDB.ExecContext(ctx, `
			INSERT INTO applications (opportunity_id, status)
			VALUES ($1, 'in_progress')
		`, oppID)
		assertUniqueViolation(t, err, "uq_active_app_per_opportunity")

		// Once the first row moves to a terminal status it leaves the
		// partial index, and a new active application is allowed.
		if _, err := sqlDB.ExecContext(ctx, `
			UPDATE applications
			SET status = 'rejected',
			    archive_reason_category = 'process_ended',
			    archived_at = now()
			WHERE opportunity_id = $1 AND status = 'applied'
		`, oppID); err != nil {
			t.Fatalf("retire first application: %v", err)
		}
		insertApplication(ctx, t, sqlDB, oppID, "applied")
	})

	t.Run("events composite FK rejects mismatched opportunity", func(t *testing.T) {
		companyID := insertCompany(ctx, t, sqlDB, "gecme")
		oppA := insertOpportunity(ctx, t, sqlDB, companyID)
		oppB := insertOpportunity(ctx, t, sqlDB, companyID)
		appOnA := insertApplication(ctx, t, sqlDB, oppA, "applied")

		// Attaching an application from opportunity A to an event whose
		// opportunity_id is B violates the composite FK (SQLSTATE 23503).
		_, err := sqlDB.ExecContext(ctx, `
			INSERT INTO events (opportunity_id, application_id, kind, occurred_at)
			VALUES ($1, $2, 'note', now())
		`, oppB, appOnA)
		assertForeignKeyViolation(t, err, "events_application_belongs_to_opportunity_fk")
	})
}

// --- helpers ---------------------------------------------------------------

func insertCompany(ctx context.Context, t *testing.T, sqlDB *sql.DB, slug string) string {
	t.Helper()
	var id string
	err := sqlDB.QueryRowContext(ctx, `
		INSERT INTO companies (name, slug) VALUES ($1, $1) RETURNING id
	`, slug).Scan(&id)
	if err != nil {
		t.Fatalf("insert company %q: %v", slug, err)
	}
	return id
}

func insertOpportunity(ctx context.Context, t *testing.T, sqlDB *sql.DB, companyID string) string {
	t.Helper()
	var id string
	err := sqlDB.QueryRowContext(ctx, `
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

func insertApplication(ctx context.Context, t *testing.T, sqlDB *sql.DB, oppID, status string) string {
	t.Helper()
	var id string
	err := sqlDB.QueryRowContext(ctx, `
		INSERT INTO applications (opportunity_id, status)
		VALUES ($1, $2)
		RETURNING id
	`, oppID, status).Scan(&id)
	if err != nil {
		t.Fatalf("insert application: %v", err)
	}
	return id
}

// SQLSTATE codes from postgres. Inlined here rather than pulling in a
// dependency just for four constants.
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
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected *pgconn.PgError, got %T: %v", err, err)
	}
	if pgErr.Code != wantSQLSTATE {
		t.Fatalf("want SQLSTATE %s, got %s (message: %s)", wantSQLSTATE, pgErr.Code, pgErr.Message)
	}
	// pgErr.ConstraintName is the most direct check; fall back to
	// substring on the message for indexes (Postgres reports the index
	// name in the message but not always in ConstraintName).
	if pgErr.ConstraintName == wantConstraint {
		return
	}
	if strings.Contains(pgErr.Message, wantConstraint) {
		return
	}
	t.Fatalf("want constraint %q, got constraint=%q message=%q",
		wantConstraint, pgErr.ConstraintName, pgErr.Message)
}
