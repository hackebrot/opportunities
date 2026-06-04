package service

import (
	"errors"
	"testing"

	"github.com/hackebrot/opportunities/internal/store"
)

// TestApplyAppTransition pins the application-tied event kinds to their
// transition rules: which "from" statuses each kind accepts, which
// "to" status it writes, whether it archives the application, and
// which archive_reason_category values it allows. Together the rows
// cover every cell of the spec's transition table for app-tied kinds.
func TestApplyAppTransition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		kind           string // event kind under test
		fromStatus     string // active application's current status (empty = none)
		reasonCategory string // archive_reason_category supplied with the event
		wantTo         string // expected application status to write
		wantArchives   bool   // expected to mirror archived_at = events.occurred_at
		wantErr        error  // sentinel the call should wrap, or nil
	}{
		// Interview kinds flip applied → in_progress; in_progress stays.
		{"screen from applied", "screen", "applied", "", "in_progress", false, nil},
		{"screen from in_progress", "screen", "in_progress", "", "in_progress", false, nil},
		{"screen rejects offer", "screen", "offer", "", "", false, ErrPrecondition},
		{"screen rejects no active", "screen", "", "", "", false, ErrPrecondition},
		{"technical from applied", "technical", "applied", "", "in_progress", false, nil},
		{"system_design from applied", "system_design", "applied", "", "in_progress", false, nil},
		{"behavioral from applied", "behavioral", "applied", "", "in_progress", false, nil},
		{"onsite from applied", "onsite", "applied", "", "in_progress", false, nil},

		// offer/counter accept applied/in_progress/offer; flip to offer.
		{"offer from applied", "offer", "applied", "", "offer", false, nil},
		{"offer from in_progress", "offer", "in_progress", "", "offer", false, nil},
		{"offer from offer", "offer", "offer", "", "offer", false, nil},
		{"counter from in_progress", "counter", "in_progress", "", "offer", false, nil},
		{"offer rejects no active", "offer", "", "", "", false, ErrPrecondition},

		// accepted requires offer; archives.
		{"accepted from offer", "accepted", "offer", "", "accepted", true, nil},
		{"accepted rejects applied", "accepted", "applied", "", "", false, ErrPrecondition},
		{"accepted rejects in_progress", "accepted", "in_progress", "", "", false, ErrPrecondition},
		{"accepted rejects no active", "accepted", "", "", "", false, ErrPrecondition},

		// rejected: active app + a rejected-bucket category.
		{"rejected from applied with fit_mismatch", "rejected", "applied", "fit_mismatch", "rejected", true, nil},
		{"rejected from in_progress with process_ended", "rejected", "in_progress", "process_ended", "rejected", true, nil},
		{"rejected from offer with other", "rejected", "offer", "other", "rejected", true, nil},
		{"rejected without category", "rejected", "applied", "", "", false, ErrValidation},
		{"rejected with declined-bucket category", "rejected", "applied", "compensation", "", false, ErrValidation},
		{"rejected rejects no active", "rejected", "", "fit_mismatch", "", false, ErrPrecondition},

		// declined (with app): active app + a declined-bucket category.
		{"declined from applied with compensation", "declined", "applied", "compensation", "declined", true, nil},
		{"declined from offer with timing", "declined", "offer", "timing", "declined", true, nil},
		{"declined without category", "declined", "applied", "", "", false, ErrValidation},
		{"declined with rejected-bucket category", "declined", "applied", "fit_mismatch", "", false, ErrValidation},

		// withdrawn: active app + a declined-bucket category.
		{"withdrawn from in_progress with scope", "withdrawn", "in_progress", "scope", "withdrawn", true, nil},
		{"withdrawn without category", "withdrawn", "applied", "", "", false, ErrValidation},
		{"withdrawn rejects no active", "withdrawn", "", "team_fit", "", false, ErrPrecondition},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := applyAppTransition(tt.kind, tt.fromStatus, tt.reasonCategory)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("applyAppTransition(%q, %q, %q): err=%v, want %v",
						tt.kind, tt.fromStatus, tt.reasonCategory, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("applyAppTransition(%q, %q, %q): unexpected error: %v",
					tt.kind, tt.fromStatus, tt.reasonCategory, err)
			}
			if got.to != tt.wantTo {
				t.Fatalf("applyAppTransition(%q, %q, %q): to=%q, want %q",
					tt.kind, tt.fromStatus, tt.reasonCategory, got.to, tt.wantTo)
			}
			if got.archives != tt.wantArchives {
				t.Fatalf("applyAppTransition(%q, %q, %q): archives=%v, want %v",
					tt.kind, tt.fromStatus, tt.reasonCategory, got.archives, tt.wantArchives)
			}
		})
	}
}

// TestRouteEvent pins the dispatch rule between opportunity-only and
// application-tied paths. The "declined" kind is dual; routing depends
// on whether an application exists.
func TestRouteEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		kind    string
		state   store.OpportunityStatusInputs
		want    eventRoute
		wantErr error
	}{
		{"exploring is opp-only", "exploring", store.OpportunityStatusInputs{}, routeOpportunity, nil},
		{"archived is opp-only", "archived", store.OpportunityStatusInputs{}, routeOpportunity, nil},
		{"note is opp-only", "note", store.OpportunityStatusInputs{}, routeOpportunity, nil},
		{"declined without app is opp-only", "declined", store.OpportunityStatusInputs{}, routeOpportunity, nil},
		{"declined with app is application", "declined", store.OpportunityStatusInputs{AnyApp: true}, routeApplication, nil},
		{"screen is application", "screen", store.OpportunityStatusInputs{AnyApp: true, ActiveAppStatus: "applied"}, routeApplication, nil},
		{"offer is application", "offer", store.OpportunityStatusInputs{}, routeApplication, nil},
		{"accepted is application", "accepted", store.OpportunityStatusInputs{}, routeApplication, nil},
		{"rejected is application", "rejected", store.OpportunityStatusInputs{}, routeApplication, nil},
		{"withdrawn is application", "withdrawn", store.OpportunityStatusInputs{}, routeApplication, nil},
		{"applied is rejected (use AddApplication)", "applied", store.OpportunityStatusInputs{}, 0, ErrValidation},
		{"unknown kind is rejected", "bogus", store.OpportunityStatusInputs{}, 0, ErrValidation},
		{"stage_scheduled is rejected", "stage_scheduled", store.OpportunityStatusInputs{}, 0, ErrValidation},
		{"reopened is rejected", "reopened", store.OpportunityStatusInputs{}, 0, ErrValidation},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := routeEvent(tt.kind, tt.state)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("routeEvent(%q): err=%v, want %v", tt.kind, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("routeEvent(%q): unexpected error: %v", tt.kind, err)
			}
			if got != tt.want {
				t.Fatalf("routeEvent(%q): got %v, want %v", tt.kind, got, tt.want)
			}
		})
	}
}
