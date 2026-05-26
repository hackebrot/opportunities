package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/hackebrot/opportunities/internal/model"
	"github.com/hackebrot/opportunities/internal/store"
)

// ContactInput is the caller-supplied subset of a contact. ID and
// timestamps are owned by the store. CompanyID is a nullable FK: nil (or
// an empty string, which normalizes to nil) means the contact is not tied
// to a company.
type ContactInput struct {
	Name      string
	Email     string
	LinkedIn  string
	Role      string
	CompanyID *string
	Notes     string
}

// normalize trims Name and validates the input. Returns ErrValidation if
// Name is empty after trim, or if a non-empty Email is malformed.
func (in ContactInput) normalize() (name string, companyID *string, err error) {
	name = strings.TrimSpace(in.Name)
	if name == "" {
		return "", nil, fmt.Errorf("%w: name is required", ErrValidation)
	}
	if err := validateEmail(in.Email); err != nil {
		return "", nil, err
	}
	return name, normalizeCompanyID(in.CompanyID), nil
}

// normalizeCompanyID maps a nil or whitespace-only pointer to nil so an
// empty --company flag clears the association rather than failing the FK.
func normalizeCompanyID(id *string) *string {
	if id == nil || strings.TrimSpace(*id) == "" {
		return nil
	}
	return id
}

// CreateContact validates input and persists.
func (s *Service) CreateContact(ctx context.Context, in ContactInput) (model.Contact, error) {
	name, companyID, err := in.normalize()
	if err != nil {
		return model.Contact{}, err
	}
	return s.store.CreateContact(ctx, store.ContactParams{
		Name:      name,
		Email:     in.Email,
		LinkedIn:  in.LinkedIn,
		Role:      in.Role,
		CompanyID: companyID,
		Notes:     in.Notes,
	})
}

// UpdateContact validates input and persists.
func (s *Service) UpdateContact(ctx context.Context, id string, in ContactInput) (model.Contact, error) {
	name, companyID, err := in.normalize()
	if err != nil {
		return model.Contact{}, err
	}
	return s.store.UpdateContact(ctx, id, store.ContactParams{
		Name:      name,
		Email:     in.Email,
		LinkedIn:  in.LinkedIn,
		Role:      in.Role,
		CompanyID: companyID,
		Notes:     in.Notes,
	})
}

// GetContact returns a contact by id.
func (s *Service) GetContact(ctx context.Context, id string) (model.Contact, error) {
	return s.store.GetContact(ctx, id)
}

// ListContacts returns all contacts.
func (s *Service) ListContacts(ctx context.Context) ([]model.Contact, error) {
	return s.store.ListContacts(ctx)
}

// DeleteContact removes a contact by id.
func (s *Service) DeleteContact(ctx context.Context, id string) error {
	return s.store.DeleteContact(ctx, id)
}
