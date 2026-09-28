package tui

// Result export: E opens a one-line path prompt rendered in place of the
// pane's status line, so the row count above it never changes and the
// mouse math (detailTableTop) stays valid.

import (
	"fmt"
	"os"
	"strings"

	"charm.land/bubbles/v2/textinput"

	"db-peek/internal/export"
)

// exportTarget is the result set the export overlay applies to.
func (m Model) exportTarget() (cols []string, raw [][]any, name string) {
	if m.tab == 3 {
		if m.querySample == nil {
			return nil, nil, "query"
		}
		return m.querySample.Columns, m.querySample.Raw, "query"
	}
	if m.sample == nil {
		return nil, nil, m.table
	}
	return m.sample.Columns, m.sample.Raw, m.table
}

// startExport opens the path prompt for the visible result set.
func (m *Model) startExport() {
	_, _, name := m.exportTarget()
	ext := ".csv"
	if m.tab == 3 {
		ext = ".json" // a query result is often heterogeneous
	}
	m.exporting = true
	m.exportInput = textinput.New()
	m.exportInput.Prompt = "export → "
	m.exportInput.CharLimit = 200
	m.exportInput.SetWidth(m.paneW())
	m.exportInput.SetValue(fmt.Sprintf("./db-peek-%s%s", sanitizeFileStem(name), ext))
	m.exportInput.CursorEnd()
	m.exportInput.Focus()
}

// stopExport closes the prompt without writing anything.
func (m *Model) stopExport() {
	m.exporting = false
	m.exportInput.Blur()
}

// finishExport writes the visible result set to the entered path and
// reports the outcome on the status line.
func (m *Model) finishExport() {
	path := strings.TrimSpace(m.exportInput.Value())
	if path == "" {
		m.stopExport()
		return
	}
	format, err := export.FormatFromExt(path)
	if err != nil {
		m.stopExport()
		m.err = err.Error()
		return
	}
	cols, raw, _ := m.exportTarget()
	if len(raw) == 0 {
		m.stopExport()
		m.err = "nothing to export"
		return
	}
	f, err := os.Create(path)
	if err != nil {
		m.stopExport()
		m.err = err.Error()
		return
	}
	n, werr := format.Write(f, cols, raw)
	cerr := f.Close()
	m.stopExport()
	switch {
	case werr != nil:
		m.err = werr.Error()
	case cerr != nil:
		m.err = cerr.Error()
	default:
		m.err = ""
		m.status = fmt.Sprintf("exported %d rows → %s", n, path)
	}
}

// exportOverlayLine replaces a status line with the export prompt. It
// stays exactly one line so nothing below it moves.
func (m Model) exportOverlayLine(fallback string) string {
	if !m.exporting {
		return fallback
	}
	return fitText(m.exportInput.View(), m.paneW())
}

func sanitizeFileStem(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-.")
	if out == "" {
		return "query"
	}
	return out
}
