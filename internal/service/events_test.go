package service

import (
	"errors"
	"testing"
)

// TestComputeLatestStatus pins the 6-step rule documented on
// computeLatestStatus. Each row is one (state → expected latest_status)
// instance; together they cover every branch (archived, active app,
// latest accepted, any app present, non-passive event without an app,
// otherwise).
func TestComputeLatestStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   statusInputs
		want string
	}{
		// Rule 1: archived wins over everything else.
		{"archived without apps or events", statusInputs{archived: true}, "archived"},
		{"archived overrides active app", statusInputs{archived: true, activeAppStatus: "applied", anyApp: true}, "archived"},
		{"archived overrides accepted", statusInputs{archived: true, latestAppStatus: "accepted", anyApp: true}, "archived"},

		// Rule 2: an active application mirrors its status.
		{"active applied", statusInputs{activeAppStatus: "applied", latestAppStatus: "applied", anyApp: true}, "applied"},
		{"active in_progress", statusInputs{activeAppStatus: "in_progress", latestAppStatus: "in_progress", anyApp: true}, "in_progress"},
		{"active offer", statusInputs{activeAppStatus: "offer", latestAppStatus: "offer", anyApp: true}, "offer"},

		// Rule 3: latest app accepted (no active row, since 'accepted' is terminal).
		{"latest accepted", statusInputs{latestAppStatus: "accepted", anyApp: true}, "accepted"},

		// Rule 4: any app present but none active and not accepted → dormant.
		{"latest rejected", statusInputs{latestAppStatus: "rejected", anyApp: true}, "dormant"},
		{"latest declined", statusInputs{latestAppStatus: "declined", anyApp: true}, "dormant"},
		{"latest withdrawn", statusInputs{latestAppStatus: "withdrawn", anyApp: true}, "dormant"},

		// Rule 5: no apps yet, but a non-passive event present.
		{"non-passive event without app", statusInputs{anyNonPassiveEvent: true}, "exploring"},

		// Rule 6: fall-through.
		{"freshly created opportunity", statusInputs{}, "watching"},
		// 'added' is a passive event; the rule sees only added → watching.
		{"only passive events present", statusInputs{anyNonPassiveEvent: false}, "watching"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := computeLatestStatus(tt.in)
			if got != tt.want {
				t.Fatalf("computeLatestStatus(%+v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestNormalizeEventLabel pins the label rules: required and trimmed for
// custom kinds, forbidden otherwise. Whitespace padding is stripped so
// it never reaches the DB.
func TestNormalizeEventLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		kind    string
		label   string
		want    *string
		wantErr bool
	}{
		{"custom with clean label", "custom", "reference check", new("reference check"), false},
		{"custom strips surrounding whitespace", "custom", "  reference check  ", new("reference check"), false},
		{"custom rejects empty label", "custom", "", nil, true},
		{"custom rejects whitespace-only label", "custom", "   ", nil, true},
		{"note allows empty label", "note", "", nil, false},
		{"note rejects non-empty label", "note", "stray label", nil, true},
		// Whitespace-only is treated as "no label" for non-custom kinds:
		// the user clearly didn't supply a meaningful value.
		{"note treats whitespace-only label as empty", "note", "   ", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeEventLabel(tt.kind, tt.label)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeEventLabel(%q, %q): want error", tt.kind, tt.label)
				}
				if !errors.Is(err, ErrValidation) {
					t.Fatalf("normalizeEventLabel(%q, %q): err=%v, want ErrValidation", tt.kind, tt.label, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeEventLabel(%q, %q): unexpected error: %v", tt.kind, tt.label, err)
			}
			switch {
			case tt.want == nil && got != nil:
				t.Fatalf("normalizeEventLabel(%q, %q) = %q, want nil", tt.kind, tt.label, *got)
			case tt.want != nil && got == nil:
				t.Fatalf("normalizeEventLabel(%q, %q) = nil, want %q", tt.kind, tt.label, *tt.want)
			case tt.want != nil && *got != *tt.want:
				t.Fatalf("normalizeEventLabel(%q, %q) = %q, want %q", tt.kind, tt.label, *got, *tt.want)
			}
		})
	}
}
