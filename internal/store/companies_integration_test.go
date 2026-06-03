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

func TestIntegrationCompaniesCRUD(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store := startPostgresStore(ctx, t)
	if err := store.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	email := "me+acmecorp@example.com"
	params := CompanyParams{
		Name:           "Acme Corp",
		Slug:           "acmecorp",
		Website:        "https://acme.test",
		CareersURL:     "https://acme.test/careers",
		PreferredEmail: &email,
		Notes:          "first contact via meetup",
	}

	created, err := store.CreateCompany(ctx, store.Pool, params)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("create: empty id")
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("create: zero timestamps: %+v", created)
	}
	wantCreated := model.Company{
		ID:             created.ID,
		Name:           params.Name,
		Slug:           params.Slug,
		Website:        params.Website,
		CareersURL:     params.CareersURL,
		PreferredEmail: &email,
		Notes:          params.Notes,
		CreatedAt:      created.CreatedAt,
		UpdatedAt:      created.UpdatedAt,
	}
	if !cmp.Equal(wantCreated, created) {
		t.Fatalf("create row (-want +got):\n%s", cmp.Diff(wantCreated, created))
	}

	got, err := store.GetCompany(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !cmp.Equal(created, got) {
		t.Fatalf("get round-trip (-want +got):\n%s", cmp.Diff(created, got))
	}

	// Second company, no preferred_email, to exercise NULL handling
	// and List ordering.
	second, err := store.CreateCompany(ctx, store.Pool, CompanyParams{
		Name: "Example Corp",
		Slug: "examplecorp",
	})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if second.PreferredEmail != nil {
		t.Fatalf("second.PreferredEmail: want nil, got %q", *second.PreferredEmail)
	}

	list, err := store.ListCompanies(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	wantList := []model.Company{created, second}
	if !cmp.Equal(wantList, list) {
		t.Fatalf("list (-want +got):\n%s", cmp.Diff(wantList, list))
	}

	updateParams := params
	updateParams.Name = "Acme Corporation"
	updateParams.Notes = "renamed after merger"
	updateParams.PreferredEmail = nil
	updated, err := store.UpdateCompany(ctx, created.ID, updateParams)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "Acme Corporation" || updated.Notes != "renamed after merger" {
		t.Fatalf("update: fields not written: %+v", updated)
	}
	if updated.PreferredEmail != nil {
		t.Fatalf("update: preferred_email not cleared: %q", *updated.PreferredEmail)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatalf("update: updated_at not bumped (was %s, now %s)",
			created.UpdatedAt, updated.UpdatedAt)
	}
	if !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Fatalf("update: created_at changed (was %s, now %s)",
			created.CreatedAt, updated.CreatedAt)
	}

	if err := store.DeleteCompany(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.GetCompany(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete: want ErrNotFound, got %v", err)
	}
	if err := store.DeleteCompany(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: want ErrNotFound, got %v", err)
	}
}

func TestIntegrationCompaniesDuplicateSlug(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store := startPostgresStore(ctx, t)
	if err := store.MigrateUp(ctx); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	if _, err := store.CreateCompany(ctx, store.Pool, CompanyParams{
		Name: "Acme Corp",
		Slug: "acmecorp",
	}); err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err := store.CreateCompany(ctx, store.Pool, CompanyParams{
		Name: "Acme Corp (second branch)",
		Slug: "acmecorp",
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("second create: want ErrConflict, got %v", err)
	}

	// Update into an existing slug also conflicts.
	other, err := store.CreateCompany(ctx, store.Pool, CompanyParams{
		Name: "Other Co",
		Slug: "otherco",
	})
	if err != nil {
		t.Fatalf("create other: %v", err)
	}
	_, err = store.UpdateCompany(ctx, other.ID, CompanyParams{
		Name: "Other Co",
		Slug: "acmecorp",
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("update to dup slug: want ErrConflict, got %v", err)
	}

	// Update of a missing id returns ErrNotFound.
	_, err = store.UpdateCompany(ctx, "00000000-0000-0000-0000-000000000000",
		CompanyParams{Name: "Ghost", Slug: "ghost"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing: want ErrNotFound, got %v", err)
	}
}
