//go:build integration

package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

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

	svc := service.New(st, testClock)

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
