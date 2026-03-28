// Package ui provides shared lipgloss styles and output formatting helpers.
package ui

import (
	"io"
	"text/tabwriter"

	"charm.land/lipgloss/v2"
)

// Symbols used in status output.
const (
	SymOK   = "✓"
	SymWarn = "⚠"
	SymFail = "!"
	SymUp   = "↑"
)

// Styles.
var (
	OK    = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	Warn  = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	Fail  = lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
	Muted = lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // bright black / grey
	Bold  = lipgloss.NewStyle().Bold(true)
)

// Table is a tab-aligned columnar writer. Call Row for each line, then Flush.
// Columns are separated by \t; tabwriter handles alignment.
type Table struct {
	w *tabwriter.Writer
}

// NewTableWriter returns a Table writing to w.
func NewTableWriter(w io.Writer) *Table {
	return &Table{w: tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)}
}

// Row writes one row. Fields are separated by tab stops for alignment.
func (t *Table) Row(fields ...string) {
	for i, f := range fields {
		if i > 0 {
			t.w.Write([]byte("\t"))
		}
		t.w.Write([]byte(f))
	}
	t.w.Write([]byte("\n"))
}

// Flush finalises the tabwriter alignment and flushes all output.
func (t *Table) Flush() {
	t.w.Flush()
}
