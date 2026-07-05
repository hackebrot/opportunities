package cli

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/hackebrot/opportunities/internal/model"
)

// compensationJSON shapes the on-the-wire compensation payload embedded in
// `opps opportunity show` / `opps application show`. Amounts stay in the
// currency's minor unit (the raw stored integer). Time fields are
// RFC3339-UTC strings for a stable format, matching the other entity
// payloads.
type compensationJSON struct {
	ID            string  `json:"id"`
	OpportunityID *string `json:"opportunity_id,omitempty"`
	ApplicationID *string `json:"application_id,omitempty"`
	Kind          string  `json:"kind"`
	BaseMinor     int64   `json:"base_minor"`
	BonusMinor    *int64  `json:"bonus_minor,omitempty"`
	EquityNotes   string  `json:"equity_notes,omitempty"`
	VestingNotes  string  `json:"vesting_notes,omitempty"`
	Currency      string  `json:"currency"`
	Notes         string  `json:"notes,omitempty"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
}

func toCompensationJSON(c model.Compensation) compensationJSON {
	return compensationJSON{
		ID:            c.ID,
		OpportunityID: c.OpportunityID,
		ApplicationID: c.ApplicationID,
		Kind:          c.Kind,
		BaseMinor:     c.BaseMinor,
		BonusMinor:    c.BonusMinor,
		EquityNotes:   c.EquityNotes,
		VestingNotes:  c.VestingNotes,
		Currency:      c.Currency,
		Notes:         c.Notes,
		CreatedAt:     c.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     c.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// toCompensationJSONs maps a slice, always returning a non-nil slice so the
// enclosing show payload renders `compensations: []` rather than `null`
// when there are none.
func toCompensationJSONs(items []model.Compensation) []compensationJSON {
	out := make([]compensationJSON, len(items))
	for i, c := range items {
		out[i] = toCompensationJSON(c)
	}
	return out
}

// printCompensationSection renders a compensations sub-table below an
// entity's own fields in the human-readable `show` output. Amounts are the
// raw minor-unit integers (cents); a nil bonus renders blank. Nothing is
// printed when there are no rows — an entity without compensation data
// shouldn't grow an empty header.
func printCompensationSection(w io.Writer, items []model.Compensation) error {
	if len(items) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, "\nCompensations:"); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "KIND\tBASE (minor)\tBONUS (minor)\tCURRENCY\tNOTES"); err != nil {
		return err
	}
	for _, c := range items {
		bonus := ""
		if c.BonusMinor != nil {
			bonus = fmt.Sprintf("%d", *c.BonusMinor)
		}
		if _, err := fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\n",
			c.Kind, c.BaseMinor, bonus, c.Currency, oneline(c.Notes)); err != nil {
			return err
		}
	}
	return tw.Flush()
}
