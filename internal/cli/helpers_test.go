package cli

import (
	"regexp"
	"strings"
)

// colGap matches tabwriter's inter-column padding (≥2 spaces). A single
// space inside a cell — multi-word notes/labels, or "BASE (minor)" — is
// left intact, so splitting a rendered line on colGap recovers its cells.
var colGap = regexp.MustCompile(`\s{2,}`)

// tableCells parses a printed `show` section into cell rows for exact
// structural assertions. It drops the blank lead line and the "<Section>:"
// title, then splits each remaining line (header + data) on colGap.
//
// Empty cells collapse (tabwriter pads them with spaces, indistinguishable
// from column gaps), so a row with blank cells parses to fewer cells than
// the header — which is itself a useful signal. Assert exact column
// positions only with fully-populated rows.
func tableCells(section string) [][]string {
	lines := strings.Split(strings.TrimSpace(section), "\n")
	rows := make([][]string, 0, len(lines)-1)
	for _, line := range lines[1:] { // drop the "<Section>:" title line
		rows = append(rows, colGap.Split(strings.TrimRight(line, " "), -1))
	}
	return rows
}
