package prompt

import (
	"context"
	"fmt"

	"github.com/hackebrot/opportunities/internal/model"
	"github.com/hackebrot/opportunities/internal/service"
)

// ApplicationCreator is the subset of *service.Service needed by
// AddApplication. It embeds OpportunityCreator so the inline-create
// branch of the opportunity picker can run the full AddOpportunity flow
// against the same value.
type ApplicationCreator interface {
	OpportunityCreator
	ListOpportunities(ctx context.Context) ([]model.Opportunity, error)
	AddApplication(ctx context.Context, in service.ApplicationInput) (model.Application, error)
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
// picked or created inline via PickOrCreate, with the chained company /
// contact pickers in AddOpportunity firing recursively when the user
// chooses "[+ New …]". The whole graph lands when AddApplication on the
// service writes the row in one transaction.
//
// In non-interactive mode the caller must supply
// prefill.Application.OpportunityID — even when exactly one opportunity
// exists. Mirrors AddOpportunity's "create requires deliberate choice"
// rule: an application establishes a permanent association, so the
// caller must pick via --opportunity rather than letting the CLI silently
// fall through to "the only one in the DB".
func AddApplication(ctx context.Context, c ApplicationCreator, prefill ApplicationCreationInput) (model.Application, error) {
	in := prefill.Application

	if in.OpportunityID == "" {
		oppID, err := resolveOpportunityChoice(ctx, c)
		if err != nil {
			return model.Application{}, err
		}
		in.OpportunityID = oppID
	}

	if err := textOptional(ctx, "Applied with email (optional)", &in.AppliedWithEmail); err != nil {
		return model.Application{}, err
	}
	if err := textOptional(ctx, "Notes (optional)", &in.Notes); err != nil {
		return model.Application{}, err
	}

	return c.AddApplication(ctx, in)
}

// resolveOpportunityChoice prompts the user to pick an existing
// opportunity or run AddOpportunity inline to create a new one. In
// non-interactive mode it returns ErrNonInteractive unconditionally —
// see the doc comment on pickOrCaptureNewCompany for the reasoning.
func resolveOpportunityChoice(ctx context.Context, c ApplicationCreator) (string, error) {
	if IsNonInteractive(ctx) {
		return "", fmt.Errorf("%w: opportunity is required", ErrNonInteractive)
	}
	opps, err := c.ListOpportunities(ctx)
	if err != nil {
		return "", err
	}
	chosen, err := PickOrCreate(
		ctx, "Pick an opportunity", opps,
		opportunityPickLabel,
		func(o model.Opportunity) string { return o.ID },
		func(ctx context.Context) (model.Opportunity, error) {
			return AddOpportunity(ctx, c, service.OpportunityCreationInput{
				Opportunity: service.OpportunityInput{OfficeDaysPerWeek: OfficeDaysUnset},
			})
		},
	)
	if err != nil {
		return "", err
	}
	return chosen.ID, nil
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
