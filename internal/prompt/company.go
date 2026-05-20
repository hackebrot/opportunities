package prompt

import (
	"context"

	"github.com/hackebrot/opportunities/internal/model"
	"github.com/hackebrot/opportunities/internal/service"
)

// CompanyCreator is the subset of *service.Service needed by AddCompany.
// Defined as an interface so tests can substitute a fake without
// constructing a real store.
type CompanyCreator interface {
	CreateCompany(ctx context.Context, in service.CompanyInput) (model.Company, error)
}

// AddCompany is the reusable company-creation flow. Fields already set
// in prefill are trusted as-is (the caller owns flag validation);
// remaining fields are prompted interactively. In non-interactive mode
// only Name is required — empty optional fields stay empty.
func AddCompany(ctx context.Context, c CompanyCreator, prefill service.CompanyInput) (model.Company, error) {
	in := prefill
	if err := Text(ctx, "Company name", &in.Name); err != nil {
		return model.Company{}, err
	}
	if err := textOptional(ctx, "Website (optional)", &in.Website); err != nil {
		return model.Company{}, err
	}
	if err := textOptional(ctx, "Careers URL (optional)", &in.CareersURL); err != nil {
		return model.Company{}, err
	}
	if err := textOptional(ctx, "Preferred email (optional)", &in.PreferredEmail); err != nil {
		return model.Company{}, err
	}
	if err := textOptional(ctx, "Notes (optional)", &in.Notes); err != nil {
		return model.Company{}, err
	}
	return c.CreateCompany(ctx, in)
}

// textOptional is like Text but accepts an empty answer. In
// non-interactive mode it leaves *value untouched and returns nil
// (empty optional fields stay empty).
func textOptional(ctx context.Context, title string, value *string) error {
	if *value != "" {
		return nil
	}
	if IsNonInteractive(ctx) {
		return nil
	}
	v, err := InterfaceFrom(ctx).Text(title, nil)
	if err != nil {
		return err
	}
	*value = v
	return nil
}
