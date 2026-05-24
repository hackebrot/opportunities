//go:build integration

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/hackebrot/opportunities/internal/model"
)

func TestIntegrationContactsCRUD(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store := startPostgresStore(ctx, t)
	if err := store.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	company, err := store.CreateCompany(ctx, CompanyParams{Name: "Acme Corp", Slug: "acmecorp"})
	if err != nil {
		t.Fatalf("create company: %v", err)
	}

	params := ContactParams{
		Name:      "Alice Example",
		Email:     "alice@example.test",
		LinkedIn:  "in/alice",
		Role:      "Hiring Manager",
		CompanyID: &company.ID,
		Notes:     "met at meetup",
	}

	created, err := store.CreateContact(ctx, params)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("create: empty id")
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("create: zero timestamps: %+v", created)
	}
	// The JOIN must resolve the company name on create.
	if created.CompanyName == nil || *created.CompanyName != "Acme Corp" {
		t.Fatalf("create: company name = %v, want Acme Corp", created.CompanyName)
	}
	wantCreated := model.Contact{
		ID:          created.ID,
		Name:        params.Name,
		Email:       params.Email,
		LinkedIn:    params.LinkedIn,
		Role:        params.Role,
		CompanyID:   &company.ID,
		CompanyName: created.CompanyName,
		Notes:       params.Notes,
		CreatedAt:   created.CreatedAt,
		UpdatedAt:   created.UpdatedAt,
	}
	if diff := cmp.Diff(wantCreated, created); diff != "" {
		t.Fatalf("create row (-want +got):\n%s", diff)
	}

	got, err := store.GetContact(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if diff := cmp.Diff(created, got); diff != "" {
		t.Fatalf("get round-trip (-want +got):\n%s", diff)
	}

	// Second contact, no company, to exercise NULL FK handling and List
	// ordering.
	second, err := store.CreateContact(ctx, ContactParams{Name: "Bob Example"})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if second.CompanyID != nil {
		t.Fatalf("second.CompanyID: want nil, got %q", *second.CompanyID)
	}
	if second.CompanyName != nil {
		t.Fatalf("second.CompanyName: want nil, got %q", *second.CompanyName)
	}

	list, err := store.ListContacts(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// Ordered by lower(name): "Alice" before "Bob".
	wantList := []model.Contact{created, second}
	if diff := cmp.Diff(wantList, list); diff != "" {
		t.Fatalf("list (-want +got):\n%s", diff)
	}

	// Update: clear the company association and rename.
	updateParams := params
	updateParams.Name = "Alice E."
	updateParams.CompanyID = nil
	updated, err := store.UpdateContact(ctx, created.ID, updateParams)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "Alice E." {
		t.Fatalf("update: name not written: %+v", updated)
	}
	if updated.CompanyID != nil || updated.CompanyName != nil {
		t.Fatalf("update: company not cleared: id=%v name=%v", updated.CompanyID, updated.CompanyName)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("update: updated_at not bumped (was %s, now %s)", created.UpdatedAt, updated.UpdatedAt)
	}
	if !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Fatalf("update: created_at changed (was %s, now %s)", created.CreatedAt, updated.CreatedAt)
	}

	if err := store.DeleteContact(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.GetContact(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete: want ErrNotFound, got %v", err)
	}
	if err := store.DeleteContact(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: want ErrNotFound, got %v", err)
	}
}

func TestIntegrationContactsCompanyFKBehavior(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store := startPostgresStore(ctx, t)
	if err := store.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	// A company_id referencing no company surfaces as ErrNotFound.
	ghost := "00000000-0000-0000-0000-000000000000"
	if _, err := store.CreateContact(ctx, ContactParams{Name: "Carol Example", CompanyID: &ghost}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("create with dangling company_id: want ErrNotFound, got %v", err)
	}

	// ON DELETE SET NULL: deleting the company nulls the contact's FK.
	company, err := store.CreateCompany(ctx, CompanyParams{Name: "Acme", Slug: "acme"})
	if err != nil {
		t.Fatalf("create company: %v", err)
	}
	contact, err := store.CreateContact(ctx, ContactParams{Name: "Carol Example", CompanyID: &company.ID})
	if err != nil {
		t.Fatalf("create contact: %v", err)
	}
	if err := store.DeleteCompany(ctx, company.ID); err != nil {
		t.Fatalf("delete company: %v", err)
	}
	got, err := store.GetContact(ctx, contact.ID)
	if err != nil {
		t.Fatalf("get contact after company delete: %v", err)
	}
	if got.CompanyID != nil || got.CompanyName != nil {
		t.Fatalf("company delete must null the FK: id=%v name=%v", got.CompanyID, got.CompanyName)
	}
}
