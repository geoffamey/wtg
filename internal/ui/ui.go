// Package ui provides shared lipgloss styles and output formatting helpers.
package ui

import (
	"fmt"
	"io"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
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
	OK     = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	Warn   = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	Fail   = lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
	Muted  = lipgloss.NewStyle().Foreground(lipgloss.Color("8")) // bright black / grey
	Bold   = lipgloss.NewStyle().Bold(true)
	Header = Muted.Bold(true)
)

// SectionHeader renders a bold+muted label followed by a muted trailing rule,
// e.g. "REPOS ────────────────────────────".
func SectionHeader(label string) string {
	const ruleLen = 30
	return Header.Render(label) + " " + Muted.Render(strings.Repeat("─", ruleLen))
}

// Table is an ANSI-aware columnar writer. Call Row for each line, then Flush.
// Columns are padded to the widest value using lipgloss's table package, which
// measures visual width rather than byte length so styled cells align correctly.
type Table struct {
	w   io.Writer
	tbl *table.Table
}

// NewTableWriter returns a Table writing to w.
func NewTableWriter(w io.Writer) *Table {
	tbl := table.New().
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderColumn(false).
		BorderHeader(false).
		StyleFunc(func(_, _ int) lipgloss.Style {
			return lipgloss.NewStyle().PaddingRight(2)
		})
	return &Table{w: w, tbl: tbl}
}

// Row writes one row. Fields are padded per-column for alignment.
func (t *Table) Row(fields ...string) {
	t.tbl.Row(fields...)
}

// Flush finalises the table alignment and writes all output.
func (t *Table) Flush() {
	if s := t.tbl.String(); s != "" {
		fmt.Fprintln(t.w, s)
	}
}
