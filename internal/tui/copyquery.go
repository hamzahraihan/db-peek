package tui

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/atotto/clipboard"

	"db-peek/internal/saved"
)

// Copy-query support: the TUI captures the mouse, so terminal text
// selection is impossible while running. Ctrl+S copies the editor SQL
// plus the last error to the system clipboard (file fallback when the
// clipboard is unavailable), so failing queries can be pasted elsewhere.

// clipboardWriteAll is a seam for tests.
var clipboardWriteAll = clipboard.WriteAll

// clipboardReadAll is a seam for tests (ctrl+v paste fallback).
var clipboardReadAll = clipboard.ReadAll

// userCacheDir is a seam for tests; the dump file follows the platform
// cache dir so the text never lands in the working directory.
var userCacheDir = os.UserCacheDir

// queryDumpText formats the editor SQL and last error for pasting.
func queryDumpText(sql, errStr string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "-- db-peek query dump %s\n", time.Now().Format(time.RFC3339))
	b.WriteString(sql + "\n")
	if strings.TrimSpace(errStr) != "" {
		b.WriteString("-- error:\n" + strings.TrimSpace(errStr) + "\n")
	}
	return b.String()
}

// copyQueryDump copies the dump; on clipboard failure it writes
// db-peek-debug.txt in the working directory instead. It reports the
// human status line for the caller to display. It never panics: NUL bytes
// (fatal to the Windows clipboard API) are stripped, and a panicking
// clipboard backend falls through to the file fallback.
func (m *Model) copyQueryDump() (status string) {
	text := scrubConnSecrets(strings.ReplaceAll(queryDumpText(m.editor.Text(), m.err), "\x00", ""), m.connStr)
	defer func() {
		if recover() != nil {
			status = writeQueryDumpFile(text)
		}
	}()
	if err := clipboardWriteAll(text); err == nil {
		return "query copied to clipboard"
	}
	return writeQueryDumpFile(text)
}

// scrubConnSecrets removes credential material from text destined for the
// clipboard or disk. Driver errors embed the DSN, so the dump is scrubbed
// at the source rather than trusting the driver.
func scrubConnSecrets(text, connStr string) string {
	connStr = strings.TrimSpace(connStr)
	if connStr == "" {
		return text
	}
	text = strings.ReplaceAll(text, connStr, saved.Mask(connStr))
	if pass := connPassword(connStr); pass != "" {
		text = strings.ReplaceAll(text, pass, "***")
	}
	return text
}

// connPassword returns the password embedded in a URL-style conn string,
// or "" when there is none.
func connPassword(connStr string) string {
	u, err := url.Parse(connStr)
	if err != nil || u.User == nil {
		return ""
	}
	pass, ok := u.User.Password()
	if !ok {
		return ""
	}
	return pass
}

// writeQueryDumpFile writes the dump outside the working directory at
// 0600: the text can embed the DSN, so it must not sit world-readable
// next to the project.
func writeQueryDumpFile(text string) string {
	dir, err := userCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "db-peek")
	path := filepath.Join(dir, "last-dump.txt")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "copy failed: " + err.Error()
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		return "copy failed: " + err.Error()
	}
	return "clipboard unavailable — wrote " + path
}

// copySelectionText copies the raw selected lines (no dump header, no
// error trailer). It shares the never-panic clipboard path.
func (m *Model) copySelectionText(text string) (status string) {
	text = strings.ReplaceAll(text, "\x00", "")
	n := len(strings.Split(text, "\n"))
	defer func() {
		if recover() != nil {
			status = writeQueryDumpFile(text)
		}
	}()
	if err := clipboardWriteAll(text); err == nil {
		if n == 1 {
			return "1 line copied to clipboard"
		}
		return fmt.Sprintf("%d lines copied to clipboard", n)
	}
	return writeQueryDumpFile(text)
}
func (m *Model) copyCellText() string {
	var colName string
	var val string

	switch {
	case m.tab == 2:
		if m.sample == nil || len(m.sample.Raw) == 0 {
			return "no cell to copy"
		}
		row := m.rowTable.Cursor()
		col := m.rowTable.Col()
		if row < 0 || row >= len(m.sample.Raw) || col < 0 || col >= len(m.sample.Columns) {
			return "no cell to copy"
		}
		colName = m.sample.Columns[col]
		val = formatCellRaw(m.sample.Raw[row][col])
	case m.tab == 3 && m.queryFocus == 1:
		if m.querySample == nil || len(m.querySample.Raw) == 0 {
			return "no cell to copy"
		}
		pageSize := m.queryPageSize
		if pageSize <= 0 {
			pageSize = 50
		}
		absRow := m.queryPage*pageSize + m.queryTable.Cursor()
		col := m.queryTable.Col()
		if absRow < 0 || absRow >= len(m.querySample.Raw) || col < 0 || col >= len(m.querySample.Columns) {
			return "no cell to copy"
		}
		colName = m.querySample.Columns[col]
		val = formatCellRaw(m.querySample.Raw[absRow][col])
	case m.tab == 0:
		row := m.colTable.Cursor()
		col := m.colTable.Col()
		if row < 0 || row >= len(m.cols) {
			return "no cell to copy"
		}
		c := m.cols[row]
		switch col {
		case 0:
			colName, val = "name", c.Name
		case 1:
			colName, val = "type", c.Type
		case 2:
			colName, val = "nullable", c.Nullable
		case 3:
			colName, val = "default", c.Default
		default:
			colName, val = "extra", c.Extra
		}
	case m.tab == 1:
		row := m.idxTable.Cursor()
		col := m.idxTable.Col()
		if row < 0 || row >= len(m.indexes) {
			return "no cell to copy"
		}
		idx := m.indexes[row]
		switch col {
		case 0:
			colName, val = "name", idx.Name
		case 1:
			colName, val = "unique", idx.Unique
		default:
			colName, val = "columns", idx.Columns
		}
	default:
		return "no cell to copy"
	}

	_ = clipboardWriteAll(val)
	return fmt.Sprintf("copied %s (%s)", colName, fitText(val, 24))
}

func (m *Model) copyRowText() string {
	var cols []string
	var raw []any

	switch {
	case m.tab == 2:
		if m.sample == nil || len(m.sample.Raw) == 0 {
			return "no row to copy"
		}
		row := m.rowTable.Cursor()
		if row < 0 || row >= len(m.sample.Raw) {
			return "no row to copy"
		}
		cols = m.sample.Columns
		raw = m.sample.Raw[row]
	case m.tab == 3 && m.queryFocus == 1:
		if m.querySample == nil || len(m.querySample.Raw) == 0 {
			return "no row to copy"
		}
		pageSize := m.queryPageSize
		if pageSize <= 0 {
			pageSize = 50
		}
		absRow := m.queryPage*pageSize + m.queryTable.Cursor()
		if absRow < 0 || absRow >= len(m.querySample.Raw) {
			return "no row to copy"
		}
		cols = m.querySample.Columns
		raw = m.querySample.Raw[absRow]
	default:
		return "no row to copy"
	}

	mObj := make(map[string]any, len(cols))
	for i, c := range cols {
		if i < len(raw) {
			mObj[c] = raw[i]
		} else {
			mObj[c] = nil
		}
	}
	b, err := json.Marshal(mObj)
	if err != nil {
		return "failed to copy row: " + err.Error()
	}
	_ = clipboardWriteAll(string(b))
	return "copied row as JSON"
}

func (m *Model) quickFilterCell() string {
	var val string
	var g *dataTable

	switch {
	case m.tab < 3:
		g = m.activeGrid()
		if m.tab == 2 && m.sample != nil {
			row, col := m.rowTable.Cursor(), m.rowTable.Col()
			if row >= 0 && row < len(m.sample.Rows) && col >= 0 && col < len(m.sample.Rows[row]) {
				val = m.sample.Rows[row][col]
			}
		} else if g != nil && len(g.cols) > 0 {
			row, col := g.Cursor(), g.Col()
			if row >= 0 && row < len(g.rows) && col >= 0 && col < len(g.rows[row]) {
				val = g.rows[row][col]
			}
		}
	case m.tab == 3 && m.queryFocus == 1:
		g = &m.queryTable
		row, col := m.queryTable.Cursor(), m.queryTable.Col()
		if m.querySample != nil && row >= 0 && row < len(m.queryTable.rows) && col >= 0 && col < len(m.queryTable.rows[row]) {
			val = m.queryTable.rows[row][col]
		}
	}

	if g == nil || val == "" {
		return "cannot filter empty cell"
	}
	if val == "NULL" {
		return "cannot filter NULL cell"
	}
	g.SetFilter(val)
	m.gridFilterInput.SetValue(val)
	return fmt.Sprintf("filtered on %q (esc to clear)", val)
}
