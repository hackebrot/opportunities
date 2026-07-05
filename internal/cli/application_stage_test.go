package cli

import (
	"bytes"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/hackebrot/opportunities/internal/model"
)

// TestPrintApplicationStageSection asserts the exact table structure for a
// fully-populated stage row: header plus order, label, the two timestamps,
// and the outcome, each in its column.
func TestPrintApplicationStageSection(t *testing.T) {
	t.Parallel()

	sched := time.Date(2026, 6, 28, 14, 0, 0, 0, time.UTC)
	completed := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	stages := []model.ApplicationStage{
		{OrderIndex: 1, Label: "Recruiter screen", ScheduledAt: &sched, CompletedAt: &completed, Outcome: new("passed")},
	}
	var buf bytes.Buffer
	if err := printApplicationStageSection(&buf, stages); err != nil {
		t.Fatalf("printApplicationStageSection: %v", err)
	}
	want := [][]string{
		{"ORDER", "LABEL", "SCHEDULED", "COMPLETED", "OUTCOME"},
		{"1", "Recruiter screen", "2026-06-28T14:00:00Z", "2026-06-22T12:00:00Z", "passed"},
	}
	if got := tableCells(buf.String()); !cmp.Equal(want, got) {
		t.Fatalf("stage table (-want +got):\n%s\nraw:\n%s", cmp.Diff(want, got), buf.String())
	}
}

// TestPrintApplicationStageSectionNilCellsBlank pins that nil
// scheduled/completed/outcome render as blank cells: they collapse, so the
// data row parses to just the order index and label.
func TestPrintApplicationStageSectionNilCellsBlank(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := printApplicationStageSection(&buf, []model.ApplicationStage{
		{OrderIndex: 1, Label: "Screen"},
	}); err != nil {
		t.Fatalf("printApplicationStageSection: %v", err)
	}
	want := [][]string{
		{"ORDER", "LABEL", "SCHEDULED", "COMPLETED", "OUTCOME"},
		{"1", "Screen"},
	}
	if got := tableCells(buf.String()); !cmp.Equal(want, got) {
		t.Fatalf("nil-cell stage table (-want +got):\n%s\nraw:\n%s", cmp.Diff(want, got), buf.String())
	}
}

// TestPrintApplicationStageSectionEmptyOmitted asserts an application with
// no stages grows no empty header.
func TestPrintApplicationStageSectionEmptyOmitted(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := printApplicationStageSection(&buf, nil); err != nil {
		t.Fatalf("printApplicationStageSection: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("empty stages: want no output, got %q", buf.String())
	}
}
