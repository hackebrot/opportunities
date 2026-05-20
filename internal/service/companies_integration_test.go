//go:build integration

package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/hackebrot/opportunities/internal/service"
	"github.com/hackebrot/opportunities/internal/store"
)

// TestIntegrationCreateCompanyUniqueSlug proves the full pipeline:
// the service derives the same slug from two equal names ("Foo Corp")
// and the second insert is rejected by the DB unique constraint and
// surfaces as store.ErrConflict.
func TestIntegrationCreateCompanyUniqueSlug(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	svc := service.New(st)

	first, err := svc.CreateCompany(ctx, service.CompanyInput{Name: "Foo Corp"})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	if first.Slug != "foocorp" {
		t.Fatalf("first.Slug = %q, want foocorp", first.Slug)
	}

	if _, err := svc.CreateCompany(ctx, service.CompanyInput{Name: "Foo Corp"}); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("second create: err=%v, want store.ErrConflict", err)
	}

	// A different spelling that slugs to the same string must also collide.
	if _, err := svc.CreateCompany(ctx, service.CompanyInput{Name: "  foo   corp  "}); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("third create (whitespace variant): err=%v, want store.ErrConflict", err)
	}
}

// startPostgresStore mirrors the helper in the store package. Duplicated
// here so this test file does not import that package's unexported
// helper (and to keep the service package free of any non-test reach
// into store internals).
func startPostgresStore(ctx context.Context, t *testing.T) *store.Store {
	t.Helper()

	pg, err := tcpg.Run(
		ctx, "postgres:16-alpine",
		tcpg.WithDatabase("opps_test"),
		tcpg.WithUsername("opps"),
		tcpg.WithPassword("opps"),
		tcpg.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := testcontainers.TerminateContainer(pg); err != nil {
			t.Logf("terminate container: %v", err)
		}
	})

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}

	s, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(s.Close)

	if err := s.Pool.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return s
}
