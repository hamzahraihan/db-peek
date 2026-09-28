package tui

// Cell editing: `e` opens an inline editor on the cursor's cell and
// issues a single-row UPDATE keyed on the table's primary key.
//
// The key values come from Sample.Raw, never Sample.Rows: the display
// strings are truncated to 60 characters, so a long key would match the
// wrong row or none at all.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	dbpkg "db-peek/internal/db"
)

// noPrimaryKeyError names the table so the footer can explain the limit.
type noPrimaryKeyError struct{ table string }

func (e noPrimaryKeyError) Error() string {
	return e.table + " has no primary key — cell editing needs one"
}

// startCellEdit opens the inline editor on the cursor's cell.
func (m *Model) startCellEdit() bool {
	if m.tab != 2 || m.sample == nil {
		return false
	}
	row, col := m.rowTable.Cursor(), m.rowTable.Col()
	if row < 0 || row >= len(m.sample.Raw) || col < 0 || col >= len(m.sample.Columns) {
		return false
	}
	width := 20
	if col < len(m.rowTable.widths) {
		width = m.rowTable.widths[col]
	}
	m.cellEditing = true
	m.cellInput = textinput.New()
	m.cellInput.CharLimit = 4096
	m.cellInput.SetWidth(width)
	m.cellInput.SetValue(cellDisplayText(m.sample, row, col))
	m.cellInput.CursorEnd()
	m.cellInput.Focus()
	return true
}

// stopCellEdit closes the inline editor without issuing an UPDATE.
func (m *Model) stopCellEdit() {
	m.cellEditing = false
	m.cellInput.Blur()
}

// cellDisplayText is the value shown in the editor. The literal "NULL"
// becomes empty so the user can retype it.
func cellDisplayText(s *dbpkg.Sample, row, col int) string {
	if row >= len(s.Rows) || col >= len(s.Rows[row]) {
		return ""
	}
	if s.Rows[row][col] == "NULL" {
		return ""
	}
	return s.Rows[row][col]
}

// spliceCellInput draws the inline editor over the cursor's cell.
func (m Model) spliceCellInput(grid string) string {
	row, col := m.rowTable.Cursor(), m.rowTable.Col()
	line := row - m.rowTable.offset + dataHeaderH
	if line < dataHeaderH {
		return grid
	}
	lines := strings.Split(grid, "\n")
	if line >= len(lines) {
		return grid
	}
	w := 20
	if col < len(m.rowTable.widths) {
		w = m.rowTable.widths[col]
	}
	lines[line] = spliceCells(lines[line], m.cellInput.View(), m.rowTable.colX(col), w, m.rowTable.totalWidth())
	return strings.Join(lines, "\n")
}

// totalWidth is the sum of the column widths, i.e. the grid's own
// content width.
func (t *dataTable) totalWidth() int {
	n := 0
	for _, w := range t.widths {
		n += w
	}
	return n
}

// colX is the content x-offset of column col, the inverse of ColAt.
func (t *dataTable) colX(col int) int {
	x := 0
	for i, w := range t.widths {
		if i == col {
			return x
		}
		x += w
	}
	return x
}

// submitCellEdit issues the UPDATE for the edited cell. The primary key
// is resolved off the UI goroutine; the key values are read from the raw
// row captured before the editor closed.
func (m *Model) submitCellEdit() tea.Cmd {
	if m.db == nil || m.sample == nil {
		m.stopCellEdit()
		return nil
	}
	table, column := m.table, ""
	row, col := m.rowTable.Cursor(), m.rowTable.Col()
	if row < 0 || row >= len(m.sample.Raw) || col < 0 || col >= len(m.sample.Columns) {
		m.stopCellEdit()
		return nil
	}
	column = m.sample.Columns[col]
	raw := append([]any(nil), m.sample.Raw[row]...)
	cols := append([]string(nil), m.sample.Columns...)

	text := m.cellInput.Value()
	// The whole value being the word NULL means SQL NULL, not a string.
	var val any
	if !strings.EqualFold(strings.TrimSpace(text), "NULL") {
		val = text
	}
	m.stopCellEdit()
	m.loading = true
	seq := m.detailSeq
	conn := m.connSeq
	ctx := m.newOpContext(10 * time.Second)
	return func() tea.Msg {
		pk, err := m.pkForTable(ctx, table)
		if err != nil {
			return cellUpdatedMsg{table: table, col: column, seq: seq, conn: conn, err: err}
		}
		if len(pk) == 0 {
			return cellUpdatedMsg{table: table, col: column, seq: seq, conn: conn, err: noPrimaryKeyError{table}}
		}
		pkVals := make([]any, len(pk))
		for i, k := range pk {
			idx := columnIndex(cols, k)
			if idx < 0 || idx >= len(raw) {
				return cellUpdatedMsg{table: table, col: column, seq: seq, conn: conn,
					err: fmt.Errorf("primary key column %q is not in the result", k)}
			}
			pkVals[i] = keyParam(raw[idx])
		}
		affected, err := m.db.UpdateCell(ctx, table, pk, pkVals, column, val)
		return cellUpdatedMsg{table: table, col: column, affected: affected, seq: seq, conn: conn, err: err}
	}
}

// pkForTable memoizes the primary key for the session; it costs a query
// on SQLite only the first time a table is edited.
func (m *Model) pkForTable(ctx context.Context, table string) ([]string, error) {
	if m.pkMemoFor == table {
		return m.pkMemoCols, nil
	}
	pk, err := m.db.PrimaryKey(ctx, table)
	if err != nil {
		return nil, err
	}
	m.pkMemoFor, m.pkMemoCols = table, pk
	return pk, nil
}

func columnIndex(cols []string, name string) int {
	for i, c := range cols {
		if strings.EqualFold(c, name) {
			return i
		}
	}
	return -1
}

// keyParam prepares one raw driver value for binding as a WHERE
// parameter. The value's type is preserved on purpose: stringifying an
// integer key would bind text, which Postgres rejects outright
// ("operator does not exist: bigint = text"). Only []byte is converted,
// since binding it raw would send a bytea to a text column.
func keyParam(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}
