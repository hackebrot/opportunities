//go:build integration

package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hackebrot/opportunities/internal/service"
)

// TestIntegrationCreateContactWithCompany proves the full pipeline:
// the service persists a contact tied to a company and the read path
// resolves the company name via the LEFT JOIN.
func TestIntegrationCreateContactWithCompany(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	st := startPostgresStore(ctx, t)
	if err := st.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	svc := service.New(st)

	company, err := svc.CreateCompany(ctx, service.CompanyInput{Name: "Foo Corp"})
	if err != nil {
		t.Fatalf("create company: %v", err)
	}

	contact, err := svc.CreateContact(ctx, service.ContactInput{
		Name:      "Alice Example",
		Email:     "alice@example.test",
		CompanyID: &company.ID,
	})
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}
	if contact.CompanyName == nil || *contact.CompanyName != "Foo Corp" {
		t.Fatalf("contact company name = %v, want Foo Corp", contact.CompanyName)
	}

	// A blank company id normalizes to no association rather than failing
	// the FK.
	blank := "  "
	solo, err := svc.CreateContact(ctx, service.ContactInput{Name: "Bob", CompanyID: &blank})
	if err != nil {
		t.Fatalf("create solo contact: %v", err)
	}
	if solo.CompanyID != nil {
		t.Fatalf("blank company id must persist as NULL, got %q", *solo.CompanyID)
	}

	// Invalid email is rejected before reaching the DB.
	if _, err := svc.CreateContact(ctx, service.ContactInput{Name: "Bad", Email: "nope"}); !errors.Is(err, service.ErrValidation) {
		t.Fatalf("create with bad email: err=%v, want ErrValidation", err)
	}
}
