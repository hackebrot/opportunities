package prompt

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// NewItemKey is the synthetic Option.Key emitted by PickOrCreate for the
// "[+ New …]" branch. Exposed so callers (and tests) can identify it.
const NewItemKey = "__opps_new__"

// ErrNoItems is returned by PickEntity when the input list is empty.
// Callers may treat this as a clean "nothing to pick" exit rather than a
// hard failure.
var ErrNoItems = errors.New("prompt: no items to pick")

// ErrNonInteractive is returned when a value is required but the caller
// is running with --non-interactive (or stdin is otherwise unavailable)
// and no flag-supplied value was provided.
var ErrNonInteractive = errors.New("prompt: value required but running non-interactively")

// Option is the key/label pair passed to Interface.Select. Key is the
// stable identifier returned by Select; Label is what the user sees.
type Option struct {
	Key   string
	Label string
}

// Interface is the boundary between this package and the underlying
// TUI (huh). cmd/opps installs prompt.Huh{} for production; tests
// install a deterministic stub via WithInterface. There is no default
// implementation — InterfaceFrom panics if nothing is installed.
//
// Text's validate callback (if non-nil) is forwarded to the underlying
// form so empty or otherwise invalid input re-prompts in place rather
// than returning a bad value to the caller.
type Interface interface {
	Select(title string, options []Option) (key string, err error)
	Text(title string, validate func(string) error) (string, error)
	Confirm(title string) (bool, error)
}

type (
	interfaceKey      struct{}
	nonInteractiveKey struct{}
)

// WithInterface returns a context that uses f for all prompt helpers.
// Must be called once at startup (in cmd/opps for production, in tests
// before invoking any prompt helper).
func WithInterface(ctx context.Context, f Interface) context.Context {
	return context.WithValue(ctx, interfaceKey{}, f)
}

// InterfaceFrom returns the Interface attached to ctx. There is no
// fallback: cmd/opps installs prompt.Huh{} for production; tests
// install a stub via WithInterface. A missing Interface is a programmer
// error — panicking surfaces it loudly instead of silently reading from
// a TTY (which would hang locally or fail opaquely in CI).
func InterfaceFrom(ctx context.Context) Interface {
	if f, ok := ctx.Value(interfaceKey{}).(Interface); ok {
		return f
	}
	panic("prompt: no Interface installed — call prompt.WithInterface before invoking prompt helpers")
}

// WithNonInteractive marks ctx as (non-)interactive. Helpers consult
// IsNonInteractive before attempting to read from a TTY.
func WithNonInteractive(ctx context.Context, v bool) context.Context {
	return context.WithValue(ctx, nonInteractiveKey{}, v)
}

// IsNonInteractive reports whether ctx was marked non-interactive.
func IsNonInteractive(ctx context.Context) bool {
	v, _ := ctx.Value(nonInteractiveKey{}).(bool)
	return v
}

// PickEntity returns the single item the user selects from items. With
// zero items it returns ErrNoItems; with exactly one item it
// auto-selects (no prompt). In non-interactive mode with more than one
// item it returns ErrNonInteractive. title is the prompt label shown
// to the user (e.g. "Pick a company").
func PickEntity[T any](ctx context.Context, title string, items []T, display func(T) string, key func(T) string) (T, error) {
	var zero T
	switch len(items) {
	case 0:
		return zero, ErrNoItems
	case 1:
		return items[0], nil
	}
	if IsNonInteractive(ctx) {
		return zero, fmt.Errorf("%w: cannot pick among %d items", ErrNonInteractive, len(items))
	}
	opts := make([]Option, len(items))
	for i, it := range items {
		opts[i] = Option{Key: key(it), Label: display(it)}
	}
	k, err := InterfaceFrom(ctx).Select(title, opts)
	if err != nil {
		return zero, err
	}
	return findByKey(items, key, k)
}

// PickOrCreate prepends a synthetic "[+ New …]" option to the picker;
// selecting it invokes create. In non-interactive mode it falls back to
// PickEntity (no inline-create branch — flags must supply the value).
// title is the prompt label shown to the user.
func PickOrCreate[T any](
	ctx context.Context,
	title string,
	items []T,
	display func(T) string,
	key func(T) string,
	create func(context.Context) (T, error),
) (T, error) {
	var zero T
	if IsNonInteractive(ctx) {
		return PickEntity(ctx, title, items, display, key)
	}
	opts := make([]Option, 0, len(items)+1)
	opts = append(opts, Option{Key: NewItemKey, Label: "[+ New …]"})
	for _, it := range items {
		k := key(it)
		if k == NewItemKey {
			return zero, fmt.Errorf("prompt: item key %q collides with PickOrCreate sentinel", k)
		}
		opts = append(opts, Option{Key: k, Label: display(it)})
	}
	k, err := InterfaceFrom(ctx).Select(title, opts)
	if err != nil {
		return zero, err
	}
	if k == NewItemKey {
		return create(ctx)
	}
	return findByKey(items, key, k)
}

func findByKey[T any](items []T, key func(T) string, k string) (T, error) {
	var zero T
	for _, it := range items {
		if key(it) == k {
			return it, nil
		}
	}
	return zero, fmt.Errorf("prompt: selected key %q not found", k)
}

// Text returns immediately if *value is already non-empty (the caller
// pre-populated it, typically from a flag) — that value is trusted
// as-is; callers own flag validation. Otherwise it prompts the user
// interactively; the underlying form re-prompts on whitespace-only
// input so callers always receive a non-empty *value on success. In
// non-interactive mode with an empty *value it returns ErrNonInteractive.
func Text(ctx context.Context, title string, value *string) error {
	if *value != "" {
		return nil
	}
	if IsNonInteractive(ctx) {
		return fmt.Errorf("%w: %s", ErrNonInteractive, title)
	}
	v, err := InterfaceFrom(ctx).Text(title, nonEmpty)
	if err != nil {
		return err
	}
	*value = v
	return nil
}

// nonEmpty rejects whitespace-only input. Used by Text so the
// underlying form re-prompts instead of returning an empty string.
func nonEmpty(s string) error {
	if strings.TrimSpace(s) == "" {
		return errors.New("value required")
	}
	return nil
}

// Confirm asks for a yes/no decision. In non-interactive mode it
// returns ErrNonInteractive — callers that want a flag-driven bypass
// (e.g. --yes) must short-circuit this call themselves. Unlike Text,
// the bool zero-value is indistinguishable from an explicit "no", so
// there is no safe default to honor here.
func Confirm(ctx context.Context, title string) (bool, error) {
	if IsNonInteractive(ctx) {
		return false, fmt.Errorf("%w: %s", ErrNonInteractive, title)
	}
	return InterfaceFrom(ctx).Confirm(title)
}
