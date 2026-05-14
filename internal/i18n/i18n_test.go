package i18n_test

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hackebrot/opportunities/internal/i18n"
)

func newRecorder(t *testing.T) (warnFn i18n.WarnFunc, out *[]string) {
	t.Helper()
	var lines []string
	return func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	}, &lines
}

func TestTranslatorLooksUpInRequestedLocale(t *testing.T) {
	t.Parallel()

	warn, lines := newRecorder(t)
	tr := i18n.New(i18n.Catalogs{
		"en-US": {"greeting": "Hello"},
		"de-DE": {"greeting": "Hallo"},
	}, i18n.WithWarn(warn))

	if got, want := tr.T("de-DE", "greeting"), "Hallo"; got != want {
		t.Errorf("T: got %q, want %q", got, want)
	}
	if len(*lines) != 0 {
		t.Errorf("unexpected warnings: %v", *lines)
	}
}

func TestTranslatorFallsBackToEnUSWhenKeyMissing(t *testing.T) {
	t.Parallel()

	warn, lines := newRecorder(t)
	tr := i18n.New(i18n.Catalogs{
		"en-US": {"greeting": "Hello"},
		"de-DE": {},
	}, i18n.WithWarn(warn))

	if got, want := tr.T("de-DE", "greeting"), "Hello"; got != want {
		t.Errorf("T: got %q, want %q", got, want)
	}
	if len(*lines) != 0 {
		t.Errorf("expected no warnings on fallback hit, got %v", *lines)
	}
}

func TestTranslatorAppliesTemplateSubstitution(t *testing.T) {
	t.Parallel()

	tr := i18n.New(i18n.Catalogs{
		"en-US": {"hello.name": "Hello, {{.Name}}!"},
	})

	got := tr.T("en-US", "hello.name", map[string]any{"Name": "World"})
	if want := "Hello, World!"; got != want {
		t.Errorf("T: got %q, want %q", got, want)
	}
}

func TestTranslatorExtraArgsWarnAndOnlyFirstIsUsed(t *testing.T) {
	t.Parallel()

	warn, lines := newRecorder(t)
	tr := i18n.New(i18n.Catalogs{
		"en-US": {"hello.name": "Hello, {{.Name}}!"},
	}, i18n.WithWarn(warn))

	got := tr.T(
		"en-US", "hello.name",
		map[string]any{"Name": "World"},
		map[string]any{"Name": "Ignored"},
	)
	if want := "Hello, World!"; got != want {
		t.Errorf("T: got %q, want %q", got, want)
	}
	if len(*lines) != 1 {
		t.Fatalf("expected one warning, got %d: %v", len(*lines), *lines)
	}
	warning := (*lines)[0]
	if !strings.Contains(warning, "hello.name") {
		t.Errorf("warning %q should mention the key", warning)
	}
	if !strings.Contains(warning, "2 args") {
		t.Errorf("warning %q should report the arg count", warning)
	}
	if !strings.Contains(warning, "only the first is used") {
		t.Errorf("warning %q should explain that extras are ignored", warning)
	}
}

func TestTranslatorTemplateParseErrorReturnsRawMessageAndWarns(t *testing.T) {
	t.Parallel()

	warn, lines := newRecorder(t)
	tr := i18n.New(i18n.Catalogs{
		"en-US": {"broken": "Hello, {{.Name"},
	}, i18n.WithWarn(warn))

	got := tr.T("en-US", "broken", map[string]any{"Name": "World"})
	if want := "Hello, {{.Name"; got != want {
		t.Errorf("T: got %q, want %q", got, want)
	}
	if len(*lines) != 1 {
		t.Fatalf("expected one warning, got %d: %v", len(*lines), *lines)
	}
	if !strings.Contains((*lines)[0], "broken") {
		t.Errorf("warning %q should mention the key", (*lines)[0])
	}
}

func TestTranslatorTemplateExecuteErrorReturnsRawMessageAndWarns(t *testing.T) {
	t.Parallel()

	warn, lines := newRecorder(t)
	tr := i18n.New(i18n.Catalogs{
		"en-US": {"needs.field": "Hello, {{.Name}}!"},
	}, i18n.WithWarn(warn))

	// Passing a struct without the Name field triggers an execute error
	// (parse succeeds; missing field on a struct fails at exec time).
	got := tr.T("en-US", "needs.field", struct{ Other string }{Other: "x"})
	if want := "Hello, {{.Name}}!"; got != want {
		t.Errorf("T: got %q, want %q", got, want)
	}
	if len(*lines) != 1 {
		t.Fatalf("expected one warning, got %d: %v", len(*lines), *lines)
	}
	if !strings.Contains((*lines)[0], "needs.field") {
		t.Errorf("warning %q should mention the key", (*lines)[0])
	}
}

func TestTranslatorMissingKeyReturnsKeyAndWarns(t *testing.T) {
	t.Parallel()

	warn, lines := newRecorder(t)
	tr := i18n.New(i18n.Catalogs{
		"en-US": {},
	}, i18n.WithWarn(warn))

	got := tr.T("en-US", "no.such.key")
	if want := "no.such.key"; got != want {
		t.Errorf("T: got %q, want %q", got, want)
	}
	if len(*lines) != 1 {
		t.Fatalf("expected one warning, got %d: %v", len(*lines), *lines)
	}
	if !strings.Contains((*lines)[0], "no.such.key") {
		t.Errorf("warning %q should mention the missing key", (*lines)[0])
	}
}

func TestLoadReadsCatalogsFromFS(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"en-US.toml": {Data: []byte(`greeting = "Hello"`)},
		"de-DE.toml": {Data: []byte(`greeting = "Hallo"`)},
	}

	tr, err := i18n.Load(fsys)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := tr.T("de-DE", "greeting"), "Hallo"; got != want {
		t.Errorf("de-DE greeting: got %q, want %q", got, want)
	}
	if got, want := tr.T("en-US", "greeting"), "Hello"; got != want {
		t.Errorf("en-US greeting: got %q, want %q", got, want)
	}
}

func TestPackageLevelTUsesEmbeddedDefault(t *testing.T) {
	t.Parallel()

	// en-US.toml is empty; lookup of a missing key should return the key
	// itself without panicking, proving the default translator initializes.
	if got, want := i18n.T("en-US", "missing"), "missing"; got != want {
		t.Errorf("package-level T: got %q, want %q", got, want)
	}
}
