package prompt

import (
	"context"
	"fmt"

	"github.com/hackebrot/opportunities/internal/model"
	"github.com/hackebrot/opportunities/internal/service"
)

// ApplicationCreator is the subset of *service.Service needed by
// AddApplication. The list methods feed the opportunity picker and (via
// collectOpportunityInput) the chained company/contact pickers of the
// inline-create branch; AddApplication persists the whole graph in one
// service call. *service.Service satisfies it.
type ApplicationCreator interface {
	ListOpportunities(ctx context.Context) ([]model.Opportunity, error)
	ListCompanies(ctx context.Context) ([]model.Company, error)
	ListContacts(ctx context.Context) ([]model.Contact, error)
	AddApplication(ctx context.Context, in service.ApplicationCreationInput) (model.Application, error)
}

// ApplicationCreationInput bundles the prefill for AddApplication. The
// opportunity choice lives in Application.OpportunityID when supplied via
// --opportunity; otherwise the picker fills it in (existing or inline-
// created).
type ApplicationCreationInput struct {
	Application service.ApplicationInput
}

// AddApplication is the reusable application-creation flow. Fields
// already set in prefill.Application are trusted as-is (callers own flag
// validation); the rest are prompted interactively. The opportunity is
// picked from the existing rows or, via the "[+ New …]" branch, collected
// inline together with its chained company / contact pickers. Either way
// the assembled graph is handed to a single service.AddApplication call,
// which writes the opportunity (when inline) and the application in one
// transaction — so aborting partway through leaves nothing behind.
//
// In non-interactive mode the caller must supply
// prefill.Application.OpportunityID — even when exactly one opportunity
// exists. Mirrors AddOpportunity's "create requires deliberate choice"
// rule: an application establishes a permanent association, so the
// caller must pick via --opportunity rather than letting the CLI silently
// fall through to "the only one in the DB".
func AddApplication(ctx context.Context, c ApplicationCreator, prefill ApplicationCreationInput) (model.Application, error) {
	in := prefill.Application
	var inlineOpp *service.OpportunityCreationInput

	if in.OpportunityID == "" {
		existingID, inline, err := resolveOpportunityChoice(ctx, c)
		if err != nil {
			return model.Application{}, err
		}
		in.OpportunityID = existingID
		inlineOpp = inline
	}

	if err := textOptional(ctx, "Applied with email (optional)", &in.AppliedWithEmail); err != nil {
		return model.Application{}, err
	}
	if err := textOptional(ctx, "Notes (optional)", &in.Notes); err != nil {
		return model.Application{}, err
	}

	return c.AddApplication(ctx, service.ApplicationCreationInput{
		Application: in,
		Opportunity: inlineOpp,
	})
}

// resolveOpportunityChoice prompts the user to pick an existing
// opportunity or, via "[+ New …]", collect a brand-new opportunity graph
// for inline creation. It returns exactly one of: a non-empty existingID,
// or a non-nil inline input to embed in ApplicationCreationInput.Opportunity
// — never both. The inline branch only collects input; the service
// performs the insert inside the application's transaction, so no
// opportunity is persisted if the caller later aborts. In non-interactive
// mode it returns ErrNonInteractive unconditionally — see the doc comment
// on pickOrCaptureNewCompany for the reasoning.
func resolveOpportunityChoice(ctx context.Context, c ApplicationCreator) (existingID string, inline *service.OpportunityCreationInput, err error) {
	if IsNonInteractive(ctx) {
		return "", nil, fmt.Errorf("%w: opportunity is required", ErrNonInteractive)
	}
	opps, err := c.ListOpportunities(ctx)
	if err != nil {
		return "", nil, err
	}
	opts := make([]Option, 0, len(opps)+1)
	opts = append(opts, Option{Key: NewItemKey, Label: "[+ New opportunity]"})
	for _, o := range opps {
		opts = append(opts, Option{Key: o.ID, Label: opportunityPickLabel(o)})
	}
	k, err := InterfaceFrom(ctx).Select("Pick an opportunity", opts)
	if err != nil {
		return "", nil, err
	}
	if k == NewItemKey {
		oppIn, err := collectOpportunityInput(ctx, c, service.OpportunityCreationInput{
			Opportunity: service.OpportunityInput{OfficeDaysPerWeek: OfficeDaysUnset},
		})
		if err != nil {
			return "", nil, err
		}
		return "", &oppIn, nil
	}
	return k, nil, nil
}

// opportunityPickLabel mirrors the CLI's opportunityPickLabel — kept
// here in the prompt layer so AddApplication does not pull in a CLI
// dependency.
func opportunityPickLabel(o model.Opportunity) string {
	role := "(no role)"
	if o.RoleTitle != nil && *o.RoleTitle != "" {
		role = *o.RoleTitle
	}
	return fmt.Sprintf("%s — %s [%s]", o.CompanyName, role, o.LatestStatus)
}
