package tui

// Table builders translate query results into bubbles tables and fit them
// to the terminal. They run after load messages arrive, before View.

import (
	"github.com/charmbracelet/bubbles/table"
)

func (m *Model) buildTables() {
	colCols := []table.Column{
		{Title: "column", Width: 24}, {Title: "type", Width: 22},
		{Title: "null", Width: 6}, {Title: "default", Width: 22}, {Title: "extra", Width: 18},
	}
	var colRows []table.Row
	for _, c := range m.cols {
		colRows = append(colRows, table.Row{c.Name, c.Type, c.Nullable, c.Default, c.Extra})
	}
	m.colTable = table.New(
		table.WithColumns(colCols), table.WithRows(colRows),
		table.WithFocused(true), table.WithHeight(12),
	)
	m.colTable.SetStyles(dataTableStyles())

	idxCols := []table.Column{{Title: "index", Width: 30}, {Title: "unique", Width: 8}, {Title: "columns", Width: 50}}
	var idxRows []table.Row
	for _, ix := range m.indexes {
		idxRows = append(idxRows, table.Row{ix.Name, ix.Unique, ix.Columns})
	}
	m.idxTable = table.New(
		table.WithColumns(idxCols), table.WithRows(idxRows),
		table.WithFocused(true), table.WithHeight(12),
	)
	m.idxTable.SetStyles(dataTableStyles())

	m.buildRowTable()
}

// buildRowTable rebuilds only the rows tab so paging keeps schema/index cursors.
func (m *Model) buildRowTable() {
	var rowCols []table.Column
	var rowRows []table.Row
	if m.sample != nil {
		for _, c := range m.sample.Columns {
			rowCols = append(rowCols, table.Column{Title: c, Width: 20})
		}
		for _, r := range m.sample.Rows {
			rowRows = append(rowRows, table.Row(r))
		}
	}
	if len(rowCols) == 0 {
		rowCols = []table.Column{{Title: "rows", Width: 40}}
		rowRows = []table.Row{{"(no rows)"}}
	}
	m.rowTable = table.New(
		table.WithColumns(rowCols), table.WithRows(rowRows),
		table.WithFocused(true), table.WithHeight(12),
	)
	m.rowTable.SetStyles(dataTableStyles())
}

func (m *Model) sizeTables() {
	w := m.width - 8
	if w < 40 {
		w = 40
	}
	// Fill the terminal: header, table title, blank, tabs, blank, footer.
	chrome := 6
	if m.err != "" {
		chrome++ // error line
	}
	if m.tab == 2 {
		chrome++ // pager line on the rows tab
	}
	if m.selectedIndexDDL() != "" {
		chrome++ // DDL echo on the indexes tab
	}
	h := m.height - chrome
	if h < 4 {
		h = 4
	}
	for _, t := range []*table.Model{&m.colTable, &m.idxTable, &m.rowTable} {
		t.SetWidth(w)
		t.SetHeight(h)
		cols := t.Columns()
		if len(cols) == 0 {
			continue
		}
		// Distribute width across columns; last column takes the slack.
		per := (w - 4) / len(cols)
		if per < 8 {
			per = 8
		}
		for i := range cols {
			if i == len(cols)-1 {
				cols[i].Width = w - 4 - per*(len(cols)-1)
				if cols[i].Width < 10 {
					cols[i].Width = 10
				}
			} else {
				cols[i].Width = per
			}
		}
		t.SetColumns(cols)
	}
}
