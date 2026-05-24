package prompt

import (
	"context"

	"github.com/hackebrot/opportunities/internal/model"
	"github.com/hackebrot/opportunities/internal/service"
)

// ContactCreator is the subset of *service.Service needed by AddContact.
// Defined as an interface so tests can substitute a fake without
// constructing a real store.
type ContactCreator interface {
	CreateContact(ctx context.Context, in service.ContactInput) (model.Contact, error)
}

// AddContact is the reusable contact-creation flow. Fields already set in
// prefill are trusted as-is (the caller owns flag validation and company
// resolution); remaining text fields are prompted interactively. The
// company association is carried in prefill.CompanyID — callers resolve
// it (via a picker or a flag) before calling, and inline callers prepop
// it with the parent flow's company. In non-interactive mode only Name is
// required; empty optional fields stay empty.
func AddContact(ctx context.Context, c ContactCreator, prefill service.ContactInput) (model.Contact, error) {
	in := prefill
	if err := Text(ctx, "Contact name", &in.Name); err != nil {
		return model.Contact{}, err
	}
	if err := textOptional(ctx, "Email (optional)", &in.Email); err != nil {
		return model.Contact{}, err
	}
	if err := textOptional(ctx, "LinkedIn (optional)", &in.LinkedIn); err != nil {
		return model.Contact{}, err
	}
	if err := textOptional(ctx, "Role (optional)", &in.Role); err != nil {
		return model.Contact{}, err
	}
	if err := textOptional(ctx, "Notes (optional)", &in.Notes); err != nil {
		return model.Contact{}, err
	}
	return c.CreateContact(ctx, in)
}
