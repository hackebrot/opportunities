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

type fakeContactCreator struct {
	got   service.ContactInput
	out   model.Contact
	err   error
	calls int
}

func (f *fakeContactCreator) CreateContact(_ context.Context, in service.ContactInput) (model.Contact, error) {
	f.got = in
	f.calls++
	return f.out, f.err
}

func TestAddContactInteractivePromptsTextFields(t *testing.T) {
	t.Parallel()

	stub := &recordingStub{texts: []string{
		"Alice Example",
		"alice@example.com",
		"https://linkedin.test/in/alice",
		"Hiring Manager",
		"met at conf",
	}}
	ctx := prompt.WithInterface(context.Background(), stub)

	creator := &fakeContactCreator{out: model.Contact{ID: "k1", Name: "Alice Example"}}
	// Company is resolved by the caller, not prompted here.
	companyID := "co1"
	got, err := prompt.AddContact(ctx, creator, service.ContactInput{CompanyID: &companyID})
	if err != nil {
		t.Fatalf("AddContact: %v", err)
	}
	if creator.calls != 1 {
		t.Fatalf("CreateContact calls = %d, want 1", creator.calls)
	}
	want := service.ContactInput{
		Name:      "Alice Example",
		Email:     "alice@example.com",
		LinkedIn:  "https://linkedin.test/in/alice",
		Role:      "Hiring Manager",
		CompanyID: &companyID,
		Notes:     "met at conf",
	}
	if diff := cmp.Diff(want, creator.got); diff != "" {
		t.Fatalf("CreateContact input (-want +got):\n%s", diff)
	}
	if got.ID != "k1" {
		t.Fatalf("returned contact ID = %q, want k1", got.ID)
	}
}

func TestAddContactPrefillSkipsPrompt(t *testing.T) {
	t.Parallel()

	stub := &recordingStub{}
	ctx := prompt.WithInterface(context.Background(), stub)

	creator := &fakeContactCreator{out: model.Contact{ID: "k2"}}
	in := service.ContactInput{
		Name:     "Bob",
		Email:    "bob@example.com",
		LinkedIn: "in/bob",
		Role:     "Recruiter",
		Notes:    "n/a",
	}
	if _, err := prompt.AddContact(ctx, creator, in); err != nil {
		t.Fatalf("AddContact: %v", err)
	}
	if len(stub.titles) != 0 {
		t.Fatalf("expected no prompts, got %v", stub.titles)
	}
	if diff := cmp.Diff(in, creator.got); diff != "" {
		t.Fatalf("CreateContact input (-want +got):\n%s", diff)
	}
}

func TestAddContactNonInteractiveMissingNameErrors(t *testing.T) {
	t.Parallel()

	ctx := prompt.WithNonInteractive(context.Background(), true)
	creator := &fakeContactCreator{}
	_, err := prompt.AddContact(ctx, creator, service.ContactInput{})
	if !errors.Is(err, prompt.ErrNonInteractive) {
		t.Fatalf("AddContact: err=%v, want ErrNonInteractive", err)
	}
	if creator.calls != 0 {
		t.Fatalf("CreateContact must not be called when validation fails; got %d calls", creator.calls)
	}
}

func TestAddContactServiceErrorPropagates(t *testing.T) {
	t.Parallel()

	ctx := prompt.WithNonInteractive(context.Background(), true)
	boom := errors.New("boom")
	creator := &fakeContactCreator{err: boom}
	_, err := prompt.AddContact(ctx, creator, service.ContactInput{Name: "Alice"})
	if !errors.Is(err, boom) {
		t.Fatalf("AddContact: err=%v, want boom", err)
	}
}
