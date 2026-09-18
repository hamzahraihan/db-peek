package tui

// Table builders translate query results into detail grids and fit them
// to the terminal. They run after load messages arrive, before View.

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *Model) buildTables() {
	colCols := []string{"column", "type", "null", "default", "extra"}
	var colRows [][]string
	for _, c := range m.cols {
		colRows = append(colRows, []string{c.Name, c.Type, c.Nullable, c.Default, c.Extra})
	}
	m.colTable.setData(colCols, colRows)
	m.colTable.SetColStyles(
		[]lipgloss.Style{colFieldStyle, colTypeStyle, colMetaStyle, colMetaStyle, colMetaStyle},
		[]lipgloss.Style{dimColFieldStyle, dimColTypeStyle, dimColMetaStyle, dimColMetaStyle, dimColMetaStyle},
	)
	m.colTable.SetCellHook(func(col int, val string, dim bool) (lipgloss.Style, bool) {
		if col != 4 {
			return lipgloss.Style{}, false
		}
		if strings.HasPrefix(val, "PK") || strings.Contains(val, "PRI") {
			if dim {
				return dimColPKStyle, true
			}
			return colPKStyle, true
		}
		return lipgloss.Style{}, false
	})

	idxCols := []string{"index", "unique", "columns"}
	var idxRows [][]string
	for _, ix := range m.indexes {
		idxRows = append(idxRows, []string{ix.Name, ix.Unique, ix.Columns})
	}
	m.idxTable.setData(idxCols, idxRows)
	m.idxTable.SetColStyles(
		[]lipgloss.Style{colFieldStyle, colMetaStyle, colTypeStyle},
		[]lipgloss.Style{dimColFieldStyle, dimColMetaStyle, dimColTypeStyle},
	)
	m.idxTable.SetCellHook(func(col int, val string, dim bool) (lipgloss.Style, bool) {
		if col != 1 {
			return lipgloss.Style{}, false
		}
		if dim {
			return dimColUniqueStyle, true
		}
		if strings.EqualFold(strings.TrimSpace(val), "yes") {
			return colUniqueYesStyle, true
		}
		return colUniqueNoStyle, true
	})

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
	// Header cells are field names: render them in the field role so they
	// read as fields, distinct from the blue generic headers elsewhere.
	m.rowTable.SetHeaderStyles(dataFieldHeaderStyle, dimFieldHeaderStyle)
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

// sidebarTreeH is the number of explorer tree rows visible in the
// sidebar: inner box height minus title, conn, separator, footer.
func (m Model) sidebarTreeH() int {
	h := m.contentH() - 2 - 4
	if h < 1 {
		h = 1
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
	m.explorer.ensureVisible(m.sidebarTreeH())
	m.sizeTables()
}

// paneX is the first terminal column of the detail pane (after sidebar + separator).
func (m Model) paneX() int { return m.sidebarW + 1 }

// paneInnerH is the ER tab's usable height: terminal content minus the
// detail borders and the base chrome (title+blank+tabs+blank = 4).
// Floor 3 keeps narrow terminals usable.
func (m Model) paneInnerH() int {
	h := m.contentH() - 2 - 4
	if h < 3 {
		h = 3
	}
	return h
}

// paneInnerW is the single width truth for the detail inner content:
// terminal minus sidebar box, gap, detail borders, and the 1-col right
// margin. Floor 20 keeps narrow terminals usable.
func (m Model) paneInnerW() int {
	w := m.width - m.paneX() - 3
	if w < 20 {
		w = 20
	}
	return w
}

func (m *Model) sizeTables() {
	w := m.paneInnerW()
	// Detail column: title, blank, tabs, blank, then the grid.
	chrome := 4
	if m.tab == 2 {
		chrome++ // pager line on the rows tab
	}
	if m.tab == 1 && m.hasIndexDDL() {
		chrome++ // DDL echo row on the indexes tab, reserved whenever any
		// index has DDL so cursor moves never change the layout height
	}
	if m.tab == 4 {
		// ER tab chrome = title+blank+tabs+blank (4); the grid area is
		// unused (ER renders the full remainder via paneInnerH).
	}
	h := m.contentH() - 2 - chrome
	if h < 3 {
		h = 3
	}
	m.colTable.Resize(w, h)
	m.idxTable.Resize(w, h)
	m.rowTable.Resize(w, h)
	// ER viewport: upper pan clamp happens at render via
	// erSliceViewport; clamp the lower bound here so a resize never
	// leaves a negative pan offset.
	if m.erPanX < 0 {
		m.erPanX = 0
	}
	if m.erPanY < 0 {
		m.erPanY = 0
	}
	if m.tab == 3 {
		// Query tab: title+blank+tabs+strip(4) + padding(1) + panel(8+2)
		// + hint/status(2); the results grid takes the remainder.
		qh := m.contentH() - 2 - (4 + 1 + queryEditorH + 2 + 2)
		if qh < 3 {
			qh = 3
		}
		m.queryTable.Resize(w, qh)
	}
}
