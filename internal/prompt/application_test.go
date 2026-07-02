package prompt_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/hackebrot/opportunities/internal/model"
	"github.com/hackebrot/opportunities/internal/prompt"
	"github.com/hackebrot/opportunities/internal/service"
)

// fakeApplicationCreator stands in for *service.Service in AddApplication
// tests. It embeds fakeOpportunityCreator to inherit the ListCompanies /
// ListContacts methods that feed the inline-create branch's chained
// company/contact pickers; the collected graph rides along on the single
// AddApplication call rather than a separate AddOpportunity (asserted below).
type fakeApplicationCreator struct {
	fakeOpportunityCreator
	opportunities []model.Opportunity
	gotApp        service.ApplicationCreationInput
	outApp        model.Application
	appErr        error
	appCalls      int
}

func (f *fakeApplicationCreator) ListOpportunities(_ context.Context) ([]model.Opportunity, error) {
	return f.opportunities, nil
}

func (f *fakeApplicationCreator) AddApplication(_ context.Context, in service.ApplicationCreationInput) (model.Application, error) {
	f.gotApp = in
	f.appCalls++
	return f.outApp, f.appErr
}

// TestAddApplicationInteractivePicksExistingOpportunity walks the happy
// path of `opps application create`: pick an existing opp, then fill in
// the optional fields.
func TestAddApplicationInteractivePicksExistingOpportunity(t *testing.T) {
	t.Parallel()

	role := "Staff Engineer"
	opps := []model.Opportunity{
		{ID: "o1", CompanyName: "Acme Corp", RoleTitle: &role, LatestStatus: "watching"},
	}
	stub := &scriptedStub{steps: []scriptedStep{
		selectStep("Pick an opportunity", "o1"),
		textStep("Applied with email (optional)", "me@example.test"),
		textStep("Notes (optional)", "applied via careers page"),
	}}
	ctx := prompt.WithInterface(context.Background(), stub)

	creator := &fakeApplicationCreator{
		opportunities: opps,
		outApp:        model.Application{ID: "a1"},
	}
	_, err := prompt.AddApplication(ctx, creator, prompt.ApplicationCreationInput{})
	if err != nil {
		t.Fatalf("AddApplication: %v", err)
	}
	if creator.appCalls != 1 {
		t.Fatalf("AddApplication calls = %d, want 1", creator.appCalls)
	}
	want := service.ApplicationCreationInput{
		Application: service.ApplicationInput{
			OpportunityID:    "o1",
			AppliedWithEmail: "me@example.test",
			Notes:            "applied via careers page",
		},
	}
	if !cmp.Equal(want, creator.gotApp) {
		t.Fatalf("application input (-want +got):\n%s", cmp.Diff(want, creator.gotApp))
	}
}

// TestAddApplicationInteractiveCreatesOpportunityInline proves the
// "[+ New ...]" branch of the opportunity picker collects the full
// opportunity graph (including its own chained company picker) and embeds
// it into a single AddApplication call, rather than persisting the
// opportunity through a separate AddOpportunity call. This is what keeps
// the whole graph in one transaction.
func TestAddApplicationInteractiveCreatesOpportunityInline(t *testing.T) {
	t.Parallel()

	stub := &scriptedStub{steps: []scriptedStep{
		selectStep("Pick an opportunity", prompt.NewItemKey),
		// AddOpportunity flow follows.
		selectStep("Pick a company", prompt.NewItemKey),
		textStep("Company name", "Foo Corp"),
		textStep("Website (optional)", ""),
		textStep("Careers URL (optional)", ""),
		textStep("Preferred email (optional)", ""),
		textStep("Notes (optional)", ""),
		textStep("Role title (optional)", "Staff Engineer"),
		textStep("Location (optional)", ""),
		selectStep("Office days per week", "0"),
		selectStep("Source", "outbound"),
		textStep("Source detail (optional)", ""),
		selectStep("Priority", "normal"),
		textStep("Notes (optional)", ""),
		confirmStep("Add a contact for this opportunity?", false),
		// Back to AddApplication tail.
		textStep("Applied with email (optional)", ""),
		textStep("Notes (optional)", ""),
	}}
	ctx := prompt.WithInterface(context.Background(), stub)

	creator := &fakeApplicationCreator{}
	creator.outApp = model.Application{ID: "a1"}

	_, err := prompt.AddApplication(ctx, creator, prompt.ApplicationCreationInput{})
	if err != nil {
		t.Fatalf("AddApplication: %v", err)
	}
	// The inline branch must not persist the opportunity on its own; the
	// graph rides along on the single AddApplication call instead.
	if creator.calls != 0 {
		t.Fatalf("AddOpportunity calls = %d, want 0 (graph embedded in AddApplication)", creator.calls)
	}
	if creator.appCalls != 1 {
		t.Fatalf("AddApplication calls = %d, want 1", creator.appCalls)
	}
	if creator.gotApp.Application.OpportunityID != "" {
		t.Fatalf("Application.OpportunityID = %q, want empty for an inline-created opportunity", creator.gotApp.Application.OpportunityID)
	}
	if creator.gotApp.Opportunity == nil {
		t.Fatal("Opportunity = nil, want the inline-collected graph")
	}
	if creator.gotApp.Opportunity.Company.New == nil || creator.gotApp.Opportunity.Company.New.Name != "Foo Corp" {
		t.Fatalf("Opportunity.Company.New = %+v, want Foo Corp", creator.gotApp.Opportunity.Company.New)
	}
}

// TestAddApplicationNonInteractiveRespectsPrefill pins that a fully
// flag-supplied input round-trips to AddApplication without any prompts
// firing.
func TestAddApplicationNonInteractiveRespectsPrefill(t *testing.T) {
	t.Parallel()

	ctx := prompt.WithNonInteractive(context.Background(), true)
	creator := &fakeApplicationCreator{outApp: model.Application{ID: "a1"}}
	prefill := prompt.ApplicationCreationInput{
		Application: service.ApplicationInput{
			OpportunityID:    "o1",
			AppliedWithEmail: "me@example.test",
			Notes:            "via careers page",
		},
	}
	if _, err := prompt.AddApplication(ctx, creator, prefill); err != nil {
		t.Fatalf("AddApplication: %v", err)
	}
	want := service.ApplicationCreationInput{Application: prefill.Application}
	if !cmp.Equal(want, creator.gotApp) {
		t.Fatalf("application input (-want +got):\n%s", cmp.Diff(want, creator.gotApp))
	}
}

// TestAddApplicationNonInteractiveMissingOpportunityErrors mirrors the
// AddOpportunity rule: create flows demand an explicit choice in
// non-interactive mode, even when exactly one row exists.
func TestAddApplicationNonInteractiveMissingOpportunityErrors(t *testing.T) {
	t.Parallel()

	ctx := prompt.WithNonInteractive(context.Background(), true)
	creator := &fakeApplicationCreator{
		opportunities: []model.Opportunity{{ID: "o1", CompanyName: "Acme Corp"}},
	}
	_, err := prompt.AddApplication(ctx, creator, prompt.ApplicationCreationInput{})
	if !errors.Is(err, prompt.ErrNonInteractive) {
		t.Fatalf("AddApplication: err=%v, want ErrNonInteractive", err)
	}
	if creator.appCalls != 0 {
		t.Fatalf("AddApplication must not be called; got %d", creator.appCalls)
	}
}
