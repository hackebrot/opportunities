package service

import "testing"

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
