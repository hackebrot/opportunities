package prompt_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/hackebrot/opportunities/internal/model"
	"github.com/hackebrot/opportunities/internal/prompt"
	"github.com/hackebrot/opportunities/internal/service"
)

type fakeOpportunityCreator struct {
	companies []model.Company
	contacts  []model.Contact
	got       service.OpportunityCreationInput
	out       model.Opportunity
	err       error
	calls     int
}

func (f *fakeOpportunityCreator) ListCompanies(_ context.Context) ([]model.Company, error) {
	return f.companies, nil
}

func (f *fakeOpportunityCreator) ListContacts(_ context.Context) ([]model.Contact, error) {
	return f.contacts, nil
}

func (f *fakeOpportunityCreator) AddOpportunity(_ context.Context, in service.OpportunityCreationInput) (model.Opportunity, error) {
	f.got = in
	f.calls++
	return f.out, f.err
}

// scriptedStep pairs the prompt the production code is expected to ask
// with the scripted answer. selectStep / textStep / confirmStep make
// each entry self-documenting at the call site so the reviewer doesn't
// have to interleave three parallel slices.
type scriptedStep struct {
	kind, title string
	text        string // select key or text answer
	yes         bool   // confirm answer
}

func selectStep(title, key string) scriptedStep {
	return scriptedStep{kind: "select", title: title, text: key}
}

func textStep(title, value string) scriptedStep {
	return scriptedStep{kind: "text", title: title, text: value}
}

func confirmStep(title string, ok bool) scriptedStep {
	return scriptedStep{kind: "confirm", title: title, yes: ok}
}

// scriptedStub replays the steps in order. A kind/title mismatch fails
// loudly so a production-side reorder shows up as a clear test failure
// rather than a silently wrong answer landing in a later prompt.
type scriptedStub struct {
	steps []scriptedStep
	idx   int
}

func (s *scriptedStub) pop(kind, title string) (scriptedStep, error) {
	if s.idx >= len(s.steps) {
		return scriptedStep{}, fmt.Errorf("scriptedStub: out of steps at %s:%q", kind, title)
	}
	step := s.steps[s.idx]
	s.idx++
	if step.kind != kind || step.title != title {
		return scriptedStep{}, fmt.Errorf(
			"scriptedStub: step %d expected %s:%q, got %s:%q",
			s.idx-1, step.kind, step.title, kind, title,
		)
	}
	return step, nil
}

func (s *scriptedStub) Select(title string, _ []prompt.Option) (string, error) {
	step, err := s.pop("select", title)
	return step.text, err
}

func (s *scriptedStub) Text(title string, _ func(string) error) (string, error) {
	step, err := s.pop("text", title)
	return step.text, err
}

func (s *scriptedStub) Confirm(title string) (bool, error) {
	step, err := s.pop("confirm", title)
	return step.yes, err
}

func TestAddOpportunityInteractivePicksExistingCompanyNoContact(t *testing.T) {
	t.Parallel()

	companies := []model.Company{{ID: "c1", Name: "Acme Corp", Slug: "acmecorp"}}
	stub := &scriptedStub{steps: []scriptedStep{
		selectStep("Pick a company", "c1"),
		textStep("Role title (optional)", "Staff Engineer"),
		textStep("Location (optional)", "Berlin"),
		selectStep("Office days per week", "3"),
		selectStep("Source", "outbound"),
		textStep("Source detail (optional)", ""),
		selectStep("Priority", "normal"),
		textStep("Notes (optional)", ""),
		confirmStep("Add a contact for this opportunity?", false),
	}}
	ctx := prompt.WithInterface(context.Background(), stub)

	creator := &fakeOpportunityCreator{companies: companies, out: model.Opportunity{ID: "o1"}}
	// Mirror the CLI: the office-days flag defaults to OfficeDaysUnset
	// so the prompt fires. A test that left this at the int zero value
	// (0 = explicit remote) would correctly skip the prompt, but
	// wouldn't exercise the interactive path under test here.
	_, err := prompt.AddOpportunity(ctx, creator, service.OpportunityCreationInput{
		Opportunity: service.OpportunityInput{OfficeDaysPerWeek: prompt.OfficeDaysUnset},
	})
	if err != nil {
		t.Fatalf("AddOpportunity: %v", err)
	}
	if creator.calls != 1 {
		t.Fatalf("calls = %d, want 1", creator.calls)
	}
	want := service.OpportunityCreationInput{
		Company: service.OpportunityCompanyChoice{ID: "c1"},
		Opportunity: service.OpportunityInput{
			RoleTitle:         "Staff Engineer",
			Location:          "Berlin",
			OfficeDaysPerWeek: 3,
			Source:            "outbound",
			Priority:          "normal",
		},
	}
	if diff := cmp.Diff(want, creator.got); diff != "" {
		t.Fatalf("input mismatch (-want +got):\n%s", diff)
	}
}

func TestAddOpportunityInteractiveCreatesCompanyAndContactInline(t *testing.T) {
	t.Parallel()

	stub := &scriptedStub{steps: []scriptedStep{
		selectStep("Pick a company", prompt.NewItemKey),
		textStep("Company name", "Foo Corp"),
		textStep("Website (optional)", ""),
		textStep("Careers URL (optional)", ""),
		textStep("Preferred email (optional)", ""),
		textStep("Notes (optional)", ""),
		textStep("Role title (optional)", ""),
		textStep("Location (optional)", ""),
		selectStep("Office days per week", "0"),
		selectStep("Source", "inbound_founder"),
		textStep("Source detail (optional)", "warm intro"),
		selectStep("Priority", "high"),
		textStep("Notes (optional)", ""),
		confirmStep("Add a contact for this opportunity?", true),
		selectStep("Pick a contact", prompt.NewItemKey),
		textStep("Contact name", "Alice Example"),
		textStep("Email (optional)", "alice@example.test"),
		textStep("LinkedIn (optional)", ""),
		textStep("Role (optional)", ""),
		textStep("Notes (optional)", ""),
		selectStep("Relationship", "referrer"),
	}}
	ctx := prompt.WithInterface(context.Background(), stub)

	creator := &fakeOpportunityCreator{out: model.Opportunity{ID: "o1"}}
	_, err := prompt.AddOpportunity(ctx, creator, service.OpportunityCreationInput{
		Opportunity: service.OpportunityInput{OfficeDaysPerWeek: prompt.OfficeDaysUnset},
	})
	if err != nil {
		t.Fatalf("AddOpportunity: %v", err)
	}
	if creator.calls != 1 {
		t.Fatalf("calls = %d, want 1", creator.calls)
	}
	if creator.got.Company.New == nil || creator.got.Company.New.Name != "Foo Corp" {
		t.Fatalf("company.New = %+v, want Foo Corp", creator.got.Company.New)
	}
	if creator.got.Company.ID != "" {
		t.Fatalf("company.ID = %q, want empty when New is set", creator.got.Company.ID)
	}
	if creator.got.Opportunity.Source != "inbound_founder" {
		t.Fatalf("source = %q, want inbound_founder", creator.got.Opportunity.Source)
	}
	if creator.got.Opportunity.SourceDetail != "warm intro" {
		t.Fatalf("source detail = %q, want warm intro", creator.got.Opportunity.SourceDetail)
	}
	if creator.got.Contact == nil || creator.got.Contact.New == nil || creator.got.Contact.New.Name != "Alice Example" {
		t.Fatalf("contact.New = %+v, want Alice Example", creator.got.Contact)
	}
	if creator.got.Contact.Relationship != "referrer" {
		t.Fatalf("relationship = %q, want referrer", creator.got.Contact.Relationship)
	}
}

func TestAddOpportunityNonInteractiveRespectsPrefill(t *testing.T) {
	t.Parallel()

	ctx := prompt.WithNonInteractive(context.Background(), true)
	creator := &fakeOpportunityCreator{out: model.Opportunity{ID: "o1"}}
	in := service.OpportunityCreationInput{
		Company: service.OpportunityCompanyChoice{ID: "c1"},
		Opportunity: service.OpportunityInput{
			Source:   "outbound",
			Priority: "normal",
		},
	}
	if _, err := prompt.AddOpportunity(ctx, creator, in); err != nil {
		t.Fatalf("AddOpportunity: %v", err)
	}
	if diff := cmp.Diff(in, creator.got); diff != "" {
		t.Fatalf("input mismatch (-want +got):\n%s", diff)
	}
}

func TestAddOpportunityNonInteractiveMissingCompanyErrors(t *testing.T) {
	t.Parallel()

	ctx := prompt.WithNonInteractive(context.Background(), true)
	creator := &fakeOpportunityCreator{}
	_, err := prompt.AddOpportunity(ctx, creator, service.OpportunityCreationInput{
		Opportunity: service.OpportunityInput{Source: "outbound", Priority: "normal"},
	})
	if !errors.Is(err, prompt.ErrNonInteractive) {
		t.Fatalf("AddOpportunity: err=%v, want ErrNonInteractive", err)
	}
	if creator.calls != 0 {
		t.Fatalf("AddOpportunity must not be called; got %d", creator.calls)
	}
}

// TestAddOpportunityNonInteractiveDoesNotAutoSelectUniqueCompany pins
// the rule that create operations require an explicit --company even
// when exactly one company exists: read-class helpers auto-select on
// uniqueness, but a create establishes a permanent association so the
// caller must pick deliberately.
func TestAddOpportunityNonInteractiveDoesNotAutoSelectUniqueCompany(t *testing.T) {
	t.Parallel()

	ctx := prompt.WithNonInteractive(context.Background(), true)
	creator := &fakeOpportunityCreator{
		companies: []model.Company{{ID: "c1", Name: "Acme Corp", Slug: "acmecorp"}},
	}
	_, err := prompt.AddOpportunity(ctx, creator, service.OpportunityCreationInput{
		Opportunity: service.OpportunityInput{Source: "outbound", Priority: "normal"},
	})
	if !errors.Is(err, prompt.ErrNonInteractive) {
		t.Fatalf("AddOpportunity: err=%v, want ErrNonInteractive", err)
	}
	if creator.calls != 0 {
		t.Fatalf("AddOpportunity must not be called; got %d", creator.calls)
	}
}
