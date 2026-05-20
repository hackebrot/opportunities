package service_test

import (
	"errors"
	"testing"

	"github.com/hackebrot/opportunities/internal/service"
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
			got, err := service.Slug(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Slug(%q): want error, got %q", tt.in, got)
				}
				if !errors.Is(err, service.ErrValidation) {
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

func TestValidateCompanyInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      service.CompanyInput
		wantErr bool
	}{
		{"valid minimal", service.CompanyInput{Name: "Foo"}, false},
		{"valid full", service.CompanyInput{
			Name: "Foo Corp", Website: "https://foo.test",
			CareersURL: "https://foo.test/careers", Notes: "hi",
		}, false},
		{"empty name", service.CompanyInput{Name: ""}, true},
		{"whitespace name", service.CompanyInput{Name: "   "}, true},
		{"all punctuation name (slug fails)", service.CompanyInput{Name: "!!!"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.in.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Validate(%+v): want error", tt.in)
				}
				if !errors.Is(err, service.ErrValidation) {
					t.Fatalf("Validate(%+v): err=%v, want ErrValidation", tt.in, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate(%+v): unexpected error: %v", tt.in, err)
			}
		})
	}
}
