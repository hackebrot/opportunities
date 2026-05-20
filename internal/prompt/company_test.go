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

type fakeCompanyCreator struct {
	got   service.CompanyInput
	out   model.Company
	err   error
	calls int
}

func (f *fakeCompanyCreator) CreateCompany(_ context.Context, in service.CompanyInput) (model.Company, error) {
	f.got = in
	f.calls++
	return f.out, f.err
}

// recordingStub captures every Text call so we can replay deterministic
// answers per call.
type recordingStub struct {
	texts   []string
	textIdx int
	titles  []string
}

func (r *recordingStub) Select(_ string, _ []prompt.Option) (string, error) {
	return "", errors.New("Select not expected")
}

func (r *recordingStub) Text(title string, _ func(string) error) (string, error) {
	r.titles = append(r.titles, title)
	if r.textIdx >= len(r.texts) {
		return "", errors.New("recordingStub: out of scripted answers")
	}
	v := r.texts[r.textIdx]
	r.textIdx++
	return v, nil
}

func (r *recordingStub) Confirm(_ string) (bool, error) {
	return false, errors.New("Confirm not expected")
}

func TestAddCompanyInteractivePromptsAllFields(t *testing.T) {
	t.Parallel()

	stub := &recordingStub{texts: []string{
		"Foo Corp",
		"https://foo.test",
		"https://foo.test/careers",
		"hi+foo@example.com",
		"met at meetup",
	}}
	ctx := prompt.WithInterface(context.Background(), stub)

	creator := &fakeCompanyCreator{out: model.Company{ID: "c1", Name: "Foo Corp", Slug: "foocorp"}}
	got, err := prompt.AddCompany(ctx, creator, service.CompanyInput{})
	if err != nil {
		t.Fatalf("AddCompany: %v", err)
	}
	if creator.calls != 1 {
		t.Fatalf("CreateCompany calls = %d, want 1", creator.calls)
	}
	want := service.CompanyInput{
		Name:           "Foo Corp",
		Website:        "https://foo.test",
		CareersURL:     "https://foo.test/careers",
		PreferredEmail: "hi+foo@example.com",
		Notes:          "met at meetup",
	}
	if diff := cmp.Diff(want, creator.got); diff != "" {
		t.Fatalf("CreateCompany input (-want +got):\n%s", diff)
	}
	if got.ID != "c1" {
		t.Fatalf("returned company ID = %q, want c1", got.ID)
	}
}

func TestAddCompanyPrefillSkipsPrompt(t *testing.T) {
	t.Parallel()

	// All fields prefilled — no prompts expected.
	stub := &recordingStub{}
	ctx := prompt.WithInterface(context.Background(), stub)

	creator := &fakeCompanyCreator{out: model.Company{ID: "c2"}}
	in := service.CompanyInput{
		Name:           "Bar Labs",
		Website:        "https://bar.test",
		CareersURL:     "https://bar.test/jobs",
		PreferredEmail: "hi+bar@example.com",
		Notes:          "n/a",
	}
	if _, err := prompt.AddCompany(ctx, creator, in); err != nil {
		t.Fatalf("AddCompany: %v", err)
	}
	if len(stub.titles) != 0 {
		t.Fatalf("expected no prompts, got %v", stub.titles)
	}
	if diff := cmp.Diff(in, creator.got); diff != "" {
		t.Fatalf("CreateCompany input (-want +got):\n%s", diff)
	}
}

func TestAddCompanyNonInteractiveMissingNameErrors(t *testing.T) {
	t.Parallel()

	ctx := prompt.WithNonInteractive(context.Background(), true)
	creator := &fakeCompanyCreator{}
	_, err := prompt.AddCompany(ctx, creator, service.CompanyInput{})
	if !errors.Is(err, prompt.ErrNonInteractive) {
		t.Fatalf("AddCompany: err=%v, want ErrNonInteractive", err)
	}
	if creator.calls != 0 {
		t.Fatalf("CreateCompany must not be called when validation fails; got %d calls", creator.calls)
	}
}

func TestAddCompanyNonInteractiveWithPrefilledNameSucceeds(t *testing.T) {
	t.Parallel()

	ctx := prompt.WithNonInteractive(context.Background(), true)
	creator := &fakeCompanyCreator{out: model.Company{ID: "c3"}}
	in := service.CompanyInput{Name: "Foo"}
	if _, err := prompt.AddCompany(ctx, creator, in); err != nil {
		t.Fatalf("AddCompany: %v", err)
	}
	if creator.got.Name != "Foo" {
		t.Fatalf("got name=%q, want Foo", creator.got.Name)
	}
}

func TestAddCompanyServiceErrorPropagates(t *testing.T) {
	t.Parallel()

	ctx := prompt.WithNonInteractive(context.Background(), true)
	boom := errors.New("boom")
	creator := &fakeCompanyCreator{err: boom}
	_, err := prompt.AddCompany(ctx, creator, service.CompanyInput{Name: "Foo"})
	if !errors.Is(err, boom) {
		t.Fatalf("AddCompany: err=%v, want boom", err)
	}
}
