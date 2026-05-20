package service

import (
	"errors"
	"testing"
)

func TestSlug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"two words", "Foo Corp", "foocorp", false},
		{"ampersand and punctuation", "Foo & Bar", "foobar", false},
		{"trailing suffix word", "Bar Limited LLC", "barlimitedllc", false},
		{"already slug", "abcxyz", "abcxyz", false},
		{"mixed digits and punctuation", "Foo 2.0", "foo20", false},
		{"leading trailing space", "   Foo   ", "foo", false},
		{"accented letters stripped", "Café FooBar", "caffoobar", false},
		{"empty", "", "", true},
		{"whitespace only", "   \t\n", "", true},
		{"all punctuation", "!!! &&& ???", "", true},
		{"unicode symbols only", "😀", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := Slug(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Slug(%q): want error, got %q", tt.in, got)
				}
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("Slug(%q): err=%v, want ErrValidation", tt.in, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Slug(%q): unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("Slug(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		in       CompanyInput
		wantName string
		wantSlug string
		wantErr  bool
	}{
		{
			name:     "valid minimal",
			in:       CompanyInput{Name: "Foo"},
			wantName: "Foo",
			wantSlug: "foo",
		},
		{
			name: "valid full trims name",
			in: CompanyInput{
				Name: "  Foo Corp  ", Website: "https://foo.test",
				CareersURL: "https://foo.test/careers", Notes: "hi",
			},
			wantName: "Foo Corp",
			wantSlug: "foocorp",
		},
		{"empty name", CompanyInput{Name: ""}, "", "", true},
		{"whitespace name", CompanyInput{Name: "   "}, "", "", true},
		{"all punctuation name (slug fails)", CompanyInput{Name: "!!!"}, "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			name, slug, err := tt.in.normalize()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalize(%+v): want error", tt.in)
				}
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("normalize(%+v): err=%v, want ErrValidation", tt.in, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalize(%+v): unexpected error: %v", tt.in, err)
			}
			if name != tt.wantName {
				t.Fatalf("normalize name = %q, want %q", name, tt.wantName)
			}
			if slug != tt.wantSlug {
				t.Fatalf("normalize slug = %q, want %q", slug, tt.wantSlug)
			}
		})
	}
}
