package cli

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/hackebrot/opportunities/internal/model"
)

// TestPrintCompensationSection asserts the exact table structure — header
// plus one cell row per compensation — so a column swap or a value landing
// on the wrong row fails here. Both rows are fully populated so every cell
// holds a position (blank cells are covered separately).
func TestPrintCompensationSection(t *testing.T) {
	t.Parallel()

	comps := []model.Compensation{
		{Kind: "target", BaseMinor: 100, BonusMinor: new(int64(50)), Currency: "EUR", Notes: "range placeholder"},
		{Kind: "listed", BaseMinor: 200, BonusMinor: new(int64(75)), Currency: "USD", Notes: "advertised band"},
	}
	var buf bytes.Buffer
	if err := printCompensationSection(&buf, comps); err != nil {
		t.Fatalf("printCompensationSection: %v", err)
	}
	want := [][]string{
		{"KIND", "BASE (minor)", "BONUS (minor)", "CURRENCY", "NOTES"},
		{"target", "100", "50", "EUR", "range placeholder"},
		{"listed", "200", "75", "USD", "advertised band"},
	}
	if got := tableCells(buf.String()); !cmp.Equal(want, got) {
		t.Fatalf("compensation table (-want +got):\n%s\nraw:\n%s", cmp.Diff(want, got), buf.String())
	}
}

// TestPrintCompensationSectionNilBonusBlank pins the one behavior the JSON
// path can't show (omitempty hides it): a nil bonus renders as a blank
// cell. The blank BONUS and NOTES cells collapse, so the data row parses
// to just kind, base, and currency — no bonus wedged between base and
// currency.
func TestPrintCompensationSectionNilBonusBlank(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := printCompensationSection(&buf, []model.Compensation{
		{Kind: "listed", BaseMinor: 200, Currency: "USD"},
	}); err != nil {
		t.Fatalf("printCompensationSection: %v", err)
	}
	want := [][]string{
		{"KIND", "BASE (minor)", "BONUS (minor)", "CURRENCY", "NOTES"},
		{"listed", "200", "USD"},
	}
	if got := tableCells(buf.String()); !cmp.Equal(want, got) {
		t.Fatalf("nil-bonus table (-want +got):\n%s\nraw:\n%s", cmp.Diff(want, got), buf.String())
	}
}

// TestPrintCompensationSectionEmptyOmitted asserts an entity with no
// compensations grows no empty header — the section prints nothing.
func TestPrintCompensationSectionEmptyOmitted(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := printCompensationSection(&buf, nil); err != nil {
		t.Fatalf("printCompensationSection: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("empty compensations: want no output, got %q", buf.String())
	}
}
