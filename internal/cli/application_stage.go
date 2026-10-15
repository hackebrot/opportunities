package cli

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/hackebrot/opportunities/internal/model"
)

// applicationStageJSON shapes the on-the-wire stage payload embedded in
// `opps application show`. Nullable timestamps and outcome are RFC3339-UTC
// strings / omitted when unset, matching the other entity payloads.
type applicationStageJSON struct {
	ID               string  `json:"id"`
	ApplicationID    string  `json:"application_id"`
	OrderIndex       int     `json:"order_index"`
	Label            string  `json:"label"`
	ExpectationNotes string  `json:"expectation_notes,omitempty"`
	PrepNotes        string  `json:"prep_notes,omitempty"`
	ScheduledAt      *string `json:"scheduled_at,omitempty"`
	CompletedAt      *string `json:"completed_at,omitempty"`
	Outcome          *string `json:"outcome,omitempty"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
}

func toApplicationStageJSON(s model.ApplicationStage) applicationStageJSON {
	out := applicationStageJSON{
		ID:               s.ID,
		ApplicationID:    s.ApplicationID,
		OrderIndex:       s.OrderIndex,
		Label:            s.Label,
		ExpectationNotes: s.ExpectationNotes,
		PrepNotes:        s.PrepNotes,
		Outcome:          s.Outcome,
		CreatedAt:        s.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:        s.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if s.ScheduledAt != nil {
		v := s.ScheduledAt.UTC().Format(time.RFC3339)
		out.ScheduledAt = &v
	}
	if s.CompletedAt != nil {
		v := s.CompletedAt.UTC().Format(time.RFC3339)
		out.CompletedAt = &v
	}
	return out
}

// toApplicationStageJSONs maps a slice, always returning a non-nil slice so
// the enclosing show payload renders `stages: []` rather than `null` when
// there are none.
func toApplicationStageJSONs(items []model.ApplicationStage) []applicationStageJSON {
	out := make([]applicationStageJSON, len(items))
	for i, s := range items {
		out[i] = toApplicationStageJSON(s)
	}
	return out
}

// printApplicationStageSection renders a stages sub-table below the
// application's own fields in the human-readable `show` output. Nothing is
// printed when there are no stages.
func printApplicationStageSection(w io.Writer, items []model.ApplicationStage) error {
	if len(items) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "\nStages:"); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "ORDER\tLABEL\tSCHEDULED\tCOMPLETED\tOUTCOME"); err != nil {
		return err
	}
	for _, s := range items {
		scheduled := ""
		if s.ScheduledAt != nil {
			scheduled = s.ScheduledAt.UTC().Format(time.RFC3339)
		}
		completed := ""
		if s.CompletedAt != nil {
			completed = s.CompletedAt.UTC().Format(time.RFC3339)
		}
		outcome := ""
		if s.Outcome != nil {
			outcome = *s.Outcome
		}
		if _, err := fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n",
			s.OrderIndex, oneline(s.Label), scheduled, completed, outcome); err != nil {
			return err
		}
	}
	return tw.Flush()
}
