package prompt_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hackebrot/opportunities/internal/prompt"
)

// stubInterface is a deterministic prompt.Interface used in tests; it never reads a TTY.
type stubInterface struct {
	selectKey  string
	selectErr  error
	text       string
	textErr    error
	confirm    bool
	confirmErr error

	gotSelect      []prompt.Option
	gotSelectTitle string
	gotTextValid   func(string) error
}

func (s *stubInterface) Select(title string, options []prompt.Option) (string, error) {
	s.gotSelectTitle = title
	s.gotSelect = options
	return s.selectKey, s.selectErr
}

func (s *stubInterface) Text(_ string, validate func(string) error) (string, error) {
	s.gotTextValid = validate
	return s.text, s.textErr
}

func (s *stubInterface) Confirm(_ string) (bool, error) {
	return s.confirm, s.confirmErr
}

type item struct {
	id, name string
}

func itemKey(i item) string     { return i.id }
func itemDisplay(i item) string { return i.name }

func TestPickEntityAutoSelectsSingle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	only := item{id: "a", name: "Only"}

	got, err := prompt.PickEntity(ctx, "Pick a thing", []item{only}, itemDisplay, itemKey)
	if err != nil {
		t.Fatalf("PickEntity: %v", err)
	}
	if got != only {
		t.Fatalf("PickEntity: got %+v, want %+v", got, only)
	}
}

func TestPickEntityEmptyReturnsErrNoItems(t *testing.T) {
	t.Parallel()

	_, err := prompt.PickEntity(context.Background(), "Pick a thing", []item{}, itemDisplay, itemKey)
	if !errors.Is(err, prompt.ErrNoItems) {
		t.Fatalf("PickEntity: got %v, want ErrNoItems", err)
	}
}

func TestPickEntityInteractiveReturnsPickerResult(t *testing.T) {
	t.Parallel()

	stub := &stubInterface{selectKey: "b"}
	ctx := prompt.WithInterface(context.Background(), stub)

	items := []item{{id: "a", name: "A"}, {id: "b", name: "B"}}
	got, err := prompt.PickEntity(ctx, "Pick a company", items, itemDisplay, itemKey)
	if err != nil {
		t.Fatalf("PickEntity: %v", err)
	}
	if got.id != "b" {
		t.Fatalf("PickEntity: got %+v, want id=b", got)
	}
	if stub.gotSelectTitle != "Pick a company" {
		t.Fatalf("PickEntity: title=%q, want %q", stub.gotSelectTitle, "Pick a company")
	}
}

func TestPickEntityNonInteractiveMultipleErrors(t *testing.T) {
	t.Parallel()

	ctx := prompt.WithNonInteractive(context.Background(), true)
	items := []item{{id: "a", name: "A"}, {id: "b", name: "B"}}

	_, err := prompt.PickEntity(ctx, "Pick a thing", items, itemDisplay, itemKey)
	if !errors.Is(err, prompt.ErrNonInteractive) {
		t.Fatalf("PickEntity: got %v, want ErrNonInteractive", err)
	}
}

func TestPickOrCreateSelectsExisting(t *testing.T) {
	t.Parallel()

	stub := &stubInterface{selectKey: "a"}
	ctx := prompt.WithInterface(context.Background(), stub)

	items := []item{{id: "a", name: "A"}}
	createCalled := false
	create := func(context.Context) (item, error) {
		createCalled = true
		return item{}, nil
	}

	got, err := prompt.PickOrCreate(ctx, "Pick or create", items, itemDisplay, itemKey, create)
	if err != nil {
		t.Fatalf("PickOrCreate: %v", err)
	}
	if got.id != "a" {
		t.Fatalf("PickOrCreate: got %+v, want id=a", got)
	}
	if createCalled {
		t.Fatal("PickOrCreate: createFn ran when an existing item was selected")
	}
	if len(stub.gotSelect) != 2 || stub.gotSelect[0].Key != prompt.NewItemKey {
		t.Fatalf("PickOrCreate: expected [+ New …] prepended, got %+v", stub.gotSelect)
	}
	if stub.gotSelectTitle != "Pick or create" {
		t.Fatalf("PickOrCreate: title=%q, want %q", stub.gotSelectTitle, "Pick or create")
	}
}

func TestPickOrCreateInvokesCreate(t *testing.T) {
	t.Parallel()

	stub := &stubInterface{selectKey: prompt.NewItemKey}
	ctx := prompt.WithInterface(context.Background(), stub)

	want := item{id: "new", name: "Fresh"}
	create := func(context.Context) (item, error) { return want, nil }

	got, err := prompt.PickOrCreate(ctx, "Pick or create", []item{{id: "a", name: "A"}}, itemDisplay, itemKey, create)
	if err != nil {
		t.Fatalf("PickOrCreate: %v", err)
	}
	if got != want {
		t.Fatalf("PickOrCreate: got %+v, want %+v", got, want)
	}
}

func TestPickOrCreateNonInteractiveAutoSelectsSingle(t *testing.T) {
	t.Parallel()

	ctx := prompt.WithNonInteractive(context.Background(), true)
	only := item{id: "a", name: "Only"}

	got, err := prompt.PickOrCreate(ctx, "Pick or create", []item{only}, itemDisplay, itemKey,
		func(context.Context) (item, error) { return item{}, errors.New("should not be called") })
	if err != nil {
		t.Fatalf("PickOrCreate: %v", err)
	}
	if got != only {
		t.Fatalf("PickOrCreate: got %+v, want %+v", got, only)
	}
}

func TestTextNonInteractiveWithValueReturnsValue(t *testing.T) {
	t.Parallel()

	ctx := prompt.WithNonInteractive(context.Background(), true)
	v := "from-flag"
	if err := prompt.Text(ctx, "name", &v); err != nil {
		t.Fatalf("Text: %v", err)
	}
	if v != "from-flag" {
		t.Fatalf("Text: got %q, want %q", v, "from-flag")
	}
}

func TestTextNonInteractiveWithoutValueErrors(t *testing.T) {
	t.Parallel()

	ctx := prompt.WithNonInteractive(context.Background(), true)
	v := ""
	err := prompt.Text(ctx, "name", &v)
	if !errors.Is(err, prompt.ErrNonInteractive) {
		t.Fatalf("Text: got %v, want ErrNonInteractive", err)
	}
}

func TestTextInteractivePromptsAndSets(t *testing.T) {
	t.Parallel()

	stub := &stubInterface{text: "typed"}
	ctx := prompt.WithInterface(context.Background(), stub)
	v := ""
	if err := prompt.Text(ctx, "name", &v); err != nil {
		t.Fatalf("Text: %v", err)
	}
	if v != "typed" {
		t.Fatalf("Text: got %q, want %q", v, "typed")
	}
	if stub.gotTextValid == nil {
		t.Fatal("Text: expected a non-empty validator to be forwarded to Interface.Text")
	}
	if err := stub.gotTextValid("   "); err == nil {
		t.Fatal("Text: forwarded validator should reject whitespace-only input")
	}
	if err := stub.gotTextValid("ok"); err != nil {
		t.Fatalf("Text: forwarded validator should accept non-empty input: %v", err)
	}
}

func TestConfirmInteractiveReturnsAnswer(t *testing.T) {
	t.Parallel()

	stub := &stubInterface{confirm: true}
	ctx := prompt.WithInterface(context.Background(), stub)
	ok, err := prompt.Confirm(ctx, "Sure?")
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if !ok {
		t.Fatal("Confirm: expected true after confirmation")
	}
}

func TestConfirmNonInteractiveErrors(t *testing.T) {
	t.Parallel()

	ctx := prompt.WithNonInteractive(context.Background(), true)
	_, err := prompt.Confirm(ctx, "Sure?")
	if !errors.Is(err, prompt.ErrNonInteractive) {
		t.Fatalf("Confirm: got %v, want ErrNonInteractive", err)
	}
}

func TestPickOrCreateRejectsCollidingKey(t *testing.T) {
	t.Parallel()

	stub := &stubInterface{}
	ctx := prompt.WithInterface(context.Background(), stub)
	items := []item{{id: prompt.NewItemKey, name: "Sneaky"}}

	_, err := prompt.PickOrCreate(ctx, "Pick or create", items, itemDisplay, itemKey,
		func(context.Context) (item, error) { return item{}, nil })
	if err == nil {
		t.Fatal("PickOrCreate: expected collision error, got nil")
	}
}
