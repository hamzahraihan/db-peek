package tui

// Table builders translate query results into detail grids and fit them
// to the terminal. They run after load messages arrive, before View.

func (m *Model) buildTables() {
	colCols := []string{"column", "type", "null", "default", "extra"}
	var colRows [][]string
	for _, c := range m.cols {
		colRows = append(colRows, []string{c.Name, c.Type, c.Nullable, c.Default, c.Extra})
	}
	m.colTable.setData(colCols, colRows)

	idxCols := []string{"index", "unique", "columns"}
	var idxRows [][]string
	for _, ix := range m.indexes {
		idxRows = append(idxRows, []string{ix.Name, ix.Unique, ix.Columns})
	}
	m.idxTable.setData(idxCols, idxRows)

	m.buildRowTable()
}

// buildRowTable rebuilds only the rows tab so paging keeps schema/index cursors.
func (m *Model) buildRowTable() {
	var rowCols []string
	var rowRows [][]string
	if m.sample != nil {
		rowCols = append([]string(nil), m.sample.Columns...)
		for _, r := range m.sample.Rows {
			rowRows = append(rowRows, append([]string(nil), r...))
		}
	}
	if len(rowCols) == 0 {
		rowCols = []string{"rows"}
		rowRows = [][]string{{"(no rows)"}}
	}
	m.rowTable.setData(rowCols, rowRows)
}

// contentH is the split content height shared by sidebar and detail:
// terminal minus header, footer, and the always-rendered error reserve.
func (m Model) contentH() int {
	h := m.height - 3
	if h < 5 {
		h = 5
	}
	return h
}

// resizeBrowse fits the sidebar and detail grids to the terminal.
// The sidebar takes a fixed slice; detail gets the remainder.
func (m *Model) resizeBrowse() {
	w := 34
	if m.width < 72 {
		w = m.width / 2
	}
	if w < 20 {
		w = 20
	}
	if w > 44 {
		w = 44
	}
	m.sidebarW = w
	m.sizeTables()
}

// paneX is the first terminal column of the detail pane (after sidebar + separator).
func (m Model) paneX() int { return m.sidebarW + 1 }

func (m *Model) sizeTables() {
	w := m.width - m.paneX() - 2
	if w < 20 {
		w = 20
	}
	// Detail column: title, blank, tabs, blank, then the grid.
	chrome := 4
	if m.tab == 2 {
		chrome++ // pager line on the rows tab
	}
	if m.tab == 1 && m.hasIndexDDL() {
		chrome++ // DDL echo row on the indexes tab, reserved whenever any
		// index has DDL so cursor moves never change the layout height
	}
	h := m.contentH() - chrome
	if h < 3 {
		h = 3
	}
	m.colTable.Resize(w, h)
	m.idxTable.Resize(w, h)
	m.rowTable.Resize(w, h)
}
