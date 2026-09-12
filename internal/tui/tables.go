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
	if m.tab == 1 && m.hasIndexDDL() {
		chrome++ // DDL echo row on the indexes tab, reserved whenever any
		// index has DDL so cursor moves never change the layout height
	}
	h := m.height - chrome
	if h < 4 {
		h = 4
	}
	m.colTable.Resize(w, h)
	m.idxTable.Resize(w, h)
	m.rowTable.Resize(w, h)
}
