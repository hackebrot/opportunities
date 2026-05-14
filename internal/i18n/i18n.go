package i18n

import (
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"text/template"

	"github.com/BurntSushi/toml"

	"github.com/hackebrot/opportunities/locales"
)

// FallbackLocale is the catalog consulted when a key is missing from the
// requested locale's catalog.
const FallbackLocale = "en-US"

// Catalog maps message keys to their translated text for a single locale.
type Catalog map[string]string

// Catalogs maps locale tags (e.g. "en-US") to their per-locale Catalog.
type Catalogs map[string]Catalog

// WarnFunc receives a single line per missing key, template parse/execute
// failure, or extra args passed to T. The format and arguments follow
// fmt.Sprintf conventions.
type WarnFunc func(format string, args ...any)

// Translator resolves message keys against a set of per-locale catalogs,
// with single-step fallback to FallbackLocale and optional text/template
// substitution for the args.
type Translator struct {
	catalogs Catalogs
	warn     WarnFunc
}

// Option configures a Translator at construction time.
type Option func(*Translator)

// WithWarn replaces the no-op warning sink. The function is called once
// per missing key, template parse/execute failure, or extra args passed
// to T.
func WithWarn(fn WarnFunc) Option {
	return func(t *Translator) { t.warn = fn }
}

// New constructs a Translator from pre-loaded catalogs.
func New(catalogs Catalogs, opts ...Option) *Translator {
	t := &Translator{
		catalogs: catalogs,
		// no-op sink so t.warn(...) is always safe to call; WithWarn overrides.
		warn: func(string, ...any) {},
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Load reads every *.toml entry at the root of fsys as a locale catalog,
// keyed by the file's base name without its extension.
func Load(fsys fs.FS, opts ...Option) (*Translator, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("i18n: read dir: %w", err)
	}
	catalogs := make(Catalogs)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".toml") {
			continue
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("i18n: read %s: %w", name, err)
		}
		cat := make(Catalog)
		if _, err := toml.Decode(string(data), &cat); err != nil {
			return nil, fmt.Errorf("i18n: decode %s: %w", name, err)
		}
		catalogs[strings.TrimSuffix(name, ".toml")] = cat
	}
	return New(catalogs, opts...), nil
}

// T returns the message for key in locale, falling back to the
// FallbackLocale catalog when not present. Missing keys return the key
// itself and emit a warning. When args is supplied, the message is
// parsed as a text/template and rendered against the first arg; any
// extra args are ignored with a warning.
func (t *Translator) T(locale, key string, args ...any) string {
	msg, ok := t.lookup(locale, key)
	if !ok {
		t.warn("i18n: missing key %q (locale %q)", key, locale)
		return key
	}
	if len(args) == 0 {
		return msg
	}
	if len(args) > 1 {
		t.warn("i18n: key %q got %d args, only the first is used", key, len(args))
	}
	tmpl, err := template.New(key).Option("missingkey=error").Parse(msg)
	if err != nil {
		t.warn("i18n: parse %q: %v", key, err)
		return msg
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, args[0]); err != nil {
		t.warn("i18n: execute %q: %v", key, err)
		return msg
	}
	return buf.String()
}

func (t *Translator) lookup(locale, key string) (string, bool) {
	if v, ok := t.catalogs[locale][key]; ok {
		return v, true
	}
	if locale != FallbackLocale {
		if v, ok := t.catalogs[FallbackLocale][key]; ok {
			return v, true
		}
	}
	return "", false
}

var defaultTranslator = sync.OnceValue(func() *Translator {
	t, err := Load(locales.FS)
	if err != nil {
		panic(fmt.Errorf("i18n: load default catalogs: %w", err))
	}
	return t
})

// T resolves key against the default translator (catalogs embedded from
// locales/*.toml). The default translator is built lazily on first call;
// if the embedded catalogs fail to parse, T panics. See (*Translator).T
// for lookup, fallback, and template semantics.
func T(locale, key string, args ...any) string {
	return defaultTranslator().T(locale, key, args...)
}
