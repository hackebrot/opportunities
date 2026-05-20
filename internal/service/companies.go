package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/hackebrot/opportunities/internal/model"
	"github.com/hackebrot/opportunities/internal/store"
)

// CompanyInput is the caller-supplied subset of a company. Slug is
// derived from Name by the service; ID and timestamps are owned by the
// store.
type CompanyInput struct {
	Name           string
	Website        string
	CareersURL     string
	PreferredEmail string
	Notes          string
}

// Validate reports whether the input is acceptable. Name must be
// non-empty after trim AND must yield a non-empty slug. Validate is
// the single pre-flight gate — if it returns nil, Create/Update will
// not reject the input on validation grounds.
func (in CompanyInput) Validate() error {
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrValidation)
	}
	if _, err := Slug(in.Name); err != nil {
		return err
	}
	return nil
}

// Slug renders a company name into the canonical slug form: lowercase,
// ASCII alphanumerics only. Returns ErrValidation if the result would
// be empty (e.g. whitespace-only, all-punctuation, non-ASCII script).
func Slug(name string) (string, error) {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	s := b.String()
	if s == "" {
		return "", fmt.Errorf("%w: name %q produces empty slug", ErrValidation, name)
	}
	return s, nil
}

// CreateCompany validates input, generates a slug, and persists.
func (s *Service) CreateCompany(ctx context.Context, in CompanyInput) (model.Company, error) {
	if err := in.Validate(); err != nil {
		return model.Company{}, err
	}
	slug, err := Slug(in.Name)
	if err != nil {
		return model.Company{}, err
	}
	return s.store.CreateCompany(ctx, store.CompanyParams{
		Name:           strings.TrimSpace(in.Name),
		Slug:           slug,
		Website:        in.Website,
		CareersURL:     in.CareersURL,
		PreferredEmail: nullableString(in.PreferredEmail),
		Notes:          in.Notes,
	})
}

// UpdateCompany re-derives the slug from the new name and persists.
func (s *Service) UpdateCompany(ctx context.Context, id string, in CompanyInput) (model.Company, error) {
	if err := in.Validate(); err != nil {
		return model.Company{}, err
	}
	slug, err := Slug(in.Name)
	if err != nil {
		return model.Company{}, err
	}
	return s.store.UpdateCompany(ctx, id, store.CompanyParams{
		Name:           strings.TrimSpace(in.Name),
		Slug:           slug,
		Website:        in.Website,
		CareersURL:     in.CareersURL,
		PreferredEmail: nullableString(in.PreferredEmail),
		Notes:          in.Notes,
	})
}

// GetCompany returns a company by id.
func (s *Service) GetCompany(ctx context.Context, id string) (model.Company, error) {
	return s.store.GetCompany(ctx, id)
}

// ListCompanies returns all companies.
func (s *Service) ListCompanies(ctx context.Context) ([]model.Company, error) {
	return s.store.ListCompanies(ctx)
}

// DeleteCompany removes a company by id.
func (s *Service) DeleteCompany(ctx context.Context, id string) error {
	return s.store.DeleteCompany(ctx, id)
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
