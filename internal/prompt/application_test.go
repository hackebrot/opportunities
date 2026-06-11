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
// tests. It embeds fakeOpportunityCreator so the inline-create branch
// (which calls AddOpportunity through the same interface) can run without
// a second mock.
type fakeApplicationCreator struct {
	fakeOpportunityCreator
	opportunities []model.Opportunity
	gotApp        service.ApplicationInput
	outApp        model.Application
	appErr        error
	appCalls      int
}

func (f *fakeApplicationCreator) ListOpportunities(_ context.Context) ([]model.Opportunity, error) {
	return f.opportunities, nil
}

func (f *fakeApplicationCreator) AddApplication(_ context.Context, in service.ApplicationInput) (model.Application, error) {
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
	want := service.ApplicationInput{
		OpportunityID:    "o1",
		AppliedWithEmail: "me@example.test",
		Notes:            "applied via careers page",
	}
	if !cmp.Equal(want, creator.gotApp) {
		t.Fatalf("application input (-want +got):\n%s", cmp.Diff(want, creator.gotApp))
	}
}

// TestAddApplicationInteractiveCreatesOpportunityInline proves the
// "[+ New ...]" branch of the opportunity picker dives into
// AddOpportunity (including its own chained company picker), then
// returns the persisted opportunity id and continues with the
// application fields.
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
	creator.out = model.Opportunity{ID: "o-new"}
	creator.outApp = model.Application{ID: "a1"}

	_, err := prompt.AddApplication(ctx, creator, prompt.ApplicationCreationInput{})
	if err != nil {
		t.Fatalf("AddApplication: %v", err)
	}
	if creator.calls != 1 {
		t.Fatalf("AddOpportunity calls = %d, want 1", creator.calls)
	}
	if creator.appCalls != 1 {
		t.Fatalf("AddApplication calls = %d, want 1", creator.appCalls)
	}
	if creator.gotApp.OpportunityID != "o-new" {
		t.Fatalf("OpportunityID = %q, want o-new", creator.gotApp.OpportunityID)
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
	if !cmp.Equal(prefill.Application, creator.gotApp) {
		t.Fatalf("application input (-want +got):\n%s", cmp.Diff(prefill.Application, creator.gotApp))
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
