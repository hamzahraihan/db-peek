package tui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

func (m Model) detailView() string {
	// Dim the whole pane while focus sits on the sidebar; full color
	// returns the moment focus moves to the detail.
	dim := !m.focusDetail
	titleStyle, activeTabStyle := titleStyle, activeTab
	if dim {
		titleStyle, activeTabStyle = dimTitleStyle, dimActiveTabStyle
	}
	gridView := (*dataTable).View
	if dim {
		gridView = (*dataTable).ViewDimmed
	}
	var b strings.Builder
	// The title must stay one row for mouse alignment: it wraps only
	// when a table name is absurdly long.
	title, suffix := "> "+m.table, ""
	if m.count >= 0 {
		suffix = fmt.Sprintf("  •  %d rows", m.count)
	}
	// Query tab owns two keystroke targets, so the title names the live
	// one — the single focus cue beside the editor panel border.
	if m.tab == 3 {
		if m.queryFocus == 0 {
			suffix += "  •  ● editor"
		} else {
			suffix += "  •  ● results"
		}
	}
	if m.width > 0 && lipgloss.Width(title+suffix) > m.paneInnerW() {
		title = "> " + fitText(m.table, m.paneInnerW()-lipgloss.Width("> ")-lipgloss.Width(suffix))
	}
	b.WriteString(titleStyle.Render(title))
	if suffix != "" {
		b.WriteString(dimStyle.Render(suffix))
	}
	b.WriteString("\n\n")
	for i, t := range m.detailTabLabels() {
		switch {
		case i == m.tab:
			b.WriteString(activeTabStyle.Render(t))
		case i == m.hoverTab && !dim:
			b.WriteString(hoverTab.Render(t))
		default:
			b.WriteString(inactiveTab.Render(t))
		}
		b.WriteString(" ")
	}
	if m.tab == 3 {
		// Buffer strip replaces the blank row so editor Y math
		// (detailTableTop/queryResultsTop) never shifts.
		b.WriteString("\n" + m.queryStripView(m.paneW()) + "\n")
	} else {
		b.WriteString("\n\n")
	}
	if m.loading {
		b.WriteString("loading...\n")
	} else {
		switch m.tab {
		case 0:
			if len(m.cols) == 0 {
				b.WriteString("(no columns)\n")
			} else {
				b.WriteString(gridView(&m.colTable) + "\n")
			}
			// Table DDL echo, mirroring the index tab: the row is always
			// rendered while DDL exists so cursor moves never shift layout.
			if ddl := firstDDLLine(m.tableDDL); ddl != "" {
				b.WriteString(dimStyle.Render(fitText(ddl, m.paneW())) + "\n")
			} else if m.tableDDL != "" {
				b.WriteString("\n")
			}
		case 1:
			if len(m.indexes) == 0 {
				b.WriteString("(no indexes)\n")
			} else {
				b.WriteString(gridView(&m.idxTable) + "\n")
			}
			// Show full DDL for the selected index when postgres/sqlite provides it.
			// The row is always rendered while any index has DDL so the
			// footer below never shifts or clips on cursor moves.
			if ddl := m.selectedIndexDDL(); ddl != "" {
				b.WriteString(dimStyle.Render(fitText(ddl, m.paneW())) + "\n")
			} else if m.hasIndexDDL() {
				b.WriteString("\n")
			}
		case 3:
			b.WriteString("\n")
			b.WriteString(m.queryEditorPanel() + "\n")
			if m.queryFocus == 0 {
				b.WriteString(renderKeyPairs([][2]string{
					{"esc", "results"}, {"ctrl+r", "run"}, {"?", "keys"},
				}, m.paneW()) + "\n")
			} else {
				b.WriteString(m.resultsStateLine() + "\n")
			}
			if m.querySample == nil {
				if m.queryAffected >= 0 {
					unit := "rows"
					if m.queryAffected == 1 {
						unit = "row"
					}
					b.WriteString(m.exportOverlayLine(renderStatus(fmt.Sprintf("%d %s affected • %d ms", m.queryAffected, unit, m.queryMs), m.paneW())) + "\n")
				} else {
					b.WriteString(m.exportOverlayLine(renderStatus("(no results — type above, ctrl+r to run)", m.paneW())) + "\n")
				}
			} else {
				b.WriteString(gridView(&m.queryTable) + "\n")
				if m.queryAffected >= 0 && m.queryPreviewTable != "" {
					unit := "rows"
					if m.queryAffected == 1 {
						unit = "row"
					}
					b.WriteString(m.exportOverlayLine(renderStatus(fmt.Sprintf("%d %s affected • preview of %s • %d ms", m.queryAffected, unit, m.queryPreviewTable, m.queryMs), m.paneW())) + "\n")
				} else {
					b.WriteString(m.exportOverlayLine(renderStatus(fmt.Sprintf("%d rows • %d ms", len(m.querySample.Rows), m.queryMs), m.paneW())) + "\n")
				}
			}
		case 4:
			b.WriteString(m.erView(m.paneInnerW(), m.paneInnerH()) + "\n")
		default:
			b.WriteString(m.exportOverlayLine(dimStyle.Render(fitText(m.pagerLine(), m.paneW()))) + "\n")
			if m.sample == nil || len(m.sample.Rows) == 0 {
				b.WriteString("(no rows)\n")
			} else {
				// The cell editor is spliced over the cursor's own line, so
				// the pane keeps its row count and the mouse math holds.
				grid := gridView(&m.rowTable)
				if m.cellEditing {
					grid = m.spliceCellInput(grid)
				}
				b.WriteString(grid + "\n")
			}
		}
	}
	if m.err != "" {
		b.WriteString(errStyle.Render(m.err) + "\n")
	}
	b.WriteString(m.detailFoot())
	return b.String()
}

// detailFoot is the single-line keybinding footer: bright keys, muted
// labels, one row always so mouse hit-testing never shifts.
func (m Model) detailFoot() string {
	w := m.paneW()
	if m.sqlFiling {
		return fitText(m.sqlFileInput.View(), w)
	}
	if m.gridFiltering {
		return fitText(m.gridFilterInput.View(), w)
	}
	switch m.tab {
	case 3:
		// Results keys show only while results own keystrokes; while
		// editing, the footer collapses to the help entry.
		if m.queryFocus != 1 {
			return renderKeyPairs([][2]string{
				{"?", "keys"},
			}, w)
		}
		base := renderKeyPairs([][2]string{
			{"H/L", "buffer"}, {"t", "new"}, {"X", "close"},
			{"e", "edit"}, {"n/p", "page"}, {"s", "size"}, {"y/Y", "yank"}, {"F", "filter"},
			{"o", "sort"}, {"/", "find"}, {"1-5", "tabs"}, {"?", "keys"},
		}, w)
		return withFilterSuffix(base, w, m.queryTable.FilterText())
	case 4:
		if m.erIsFocused() {
			return renderKeyPairs([][2]string{
				{"f", "all tables"}, {"n/p", "select"}, {"enter", "recenter"},
				{"wasd", "pan"}, {"r", "reload"},
			}, w)
		}
		return renderKeyPairs([][2]string{
			{"f", "focused view"}, {"n/p", "select"}, {"enter", "recenter"},
			{"wasd", "pan"}, {"r", "reload"},
		}, w)
	default:
		pairs := [][2]string{
			{"↑↓", "move"}, {"pgup/pgdn", "page"}, {"n/p", "page"},
			{"s", "size"}, {"y/Y", "yank"}, {"F", "filter"}, {"o", "sort page"}, {"/", "find"}, {"1-5", "tabs"}, {"?", "keys"},
		}
		if m.tab == 0 {
			pairs = [][2]string{
				{"↑↓", "move"}, {"d", "ddl"}, {"o", "sort page"}, {"/", "find"}, {"1-5", "tabs"}, {"?", "keys"},
			}
		}
		base := renderKeyPairs(pairs, w)
		var f string
		switch m.tab {
		case 0:
			f = m.colTable.FilterText()
		case 1:
			f = m.idxTable.FilterText()
		default:
			f = m.rowTable.FilterText()
		}
		return withFilterSuffix(base, w, f)
	}
}

// resultsStateLine summarizes the results grid view state (sort + find)
// for the filler row under the editor panel; "" when there is nothing to
// report, keeping the default calm. The row itself always renders, so
// results-row math never shifts either way.
func (m Model) resultsStateLine() string {
	t := &m.queryTable
	var parts []string
	if m.querySample != nil && len(m.querySample.Rows) > 0 {
		pageSize := m.queryPageSize
		if pageSize <= 0 {
			pageSize = 50
		}
		totalPages := (len(m.querySample.Rows) + pageSize - 1) / pageSize
		if totalPages > 1 {
			parts = append(parts, fmt.Sprintf("page %d/%d (%d rows)", m.queryPage+1, totalPages, len(m.querySample.Rows)))
		}
	}
	if c := t.SortedCol(); c >= 0 && c < len(t.cols) && t.SortState() != "" {
		dir := "asc"
		if st := t.SortState(); len(st) >= 4 && st[len(st)-4:] == "desc" {
			dir = "desc"
		}
		parts = append(parts, "sort: "+t.cols[c]+" "+dir)
	}
	if f := t.FilterText(); f != "" {
		parts = append(parts, "/"+f)
	}
	if len(parts) == 0 {
		return ""
	}
	return dimStyle.Render(fitText(strings.Join(parts, " · "), m.paneW()))
}

// withFilterSuffix appends the active find text (• /foo) to a footer,
// truncating the text (never the keys) so the line still fits w.
func withFilterSuffix(base string, w int, filter string) string {
	if filter == "" {
		return base
	}
	suffix := " • /" + filter
	if w > 0 && lipgloss.Width(base+suffix) > w {
		keep := w - lipgloss.Width(base) - lipgloss.Width(" • /…")
		if keep < 0 {
			return base
		}
		rs := []rune(filter)
		if len(rs) > keep {
			filter = string(rs[:keep]) + "…"
		}
		suffix = " • /" + filter
	}
	return base + dimStyle.Render(suffix)
}

// firstDDLLine returns the first non-empty DDL line for echo display.
func firstDDLLine(ddl string) string {
	for _, ln := range strings.Split(ddl, "\n") {
		if strings.TrimSpace(ln) != "" {
			return ln
		}
	}
	return ""
}

// paneW is the detail pane's usable width: terminal minus sidebar and separator.
func (m Model) paneW() int {
	return m.paneInnerW()
}

// queryEditorH is the fixed editor height (rows) on the query tab.
const queryEditorH = 8

// queryCursorStyle marks the cursor cell while the editor has focus.
var queryCursorStyle = lipgloss.NewStyle().Reverse(true)

// queryEditorTop is the first terminal row of the editor content:
// detailTableTop + 1 padding row + 1 panel top border.
func queryEditorTop() int { return detailTableTop + 2 }

// queryEditorInnerW is the editor content width: pane minus the panel's
// left/right borders.
func (m Model) queryEditorInnerW() int {
	w := m.paneInnerW() - 2
	if w < 1 {
		w = 1
	}
	return w
}

// queryResultsTop is the first terminal row of the query results grid:
// padding(1) + panel top(1) + editor(8) + panel bottom(1) + hint(1)
// below the strip row.
func queryResultsTop() int { return detailTableTop + queryEditorH + 4 }

// queryEditorView renders the fixed 8-row highlighted editor with dim line
// numbers and a reverse-video cursor cell when the editor has focus.
// HighlightSQL("") yields [[]] and SetText("") yields [""], so empty
// cell-rows render as blank lines and no empty row is ever indexed.
// Content width is the panel inner width (pane minus panel borders).
func (m Model) queryEditorView() string {
	w := m.queryEditorInnerW()
	// Highlighting the whole buffer costs a lex pass over every line, but
	// only 8 rows are drawn: cache the cells against the buffer text.
	text := m.editor.Text()
	if m.hlMemo == nil {
		m.hlMemo = &hlMemo{}
	}
	if m.hlMemo.text != text {
		m.hlMemo.text = text
		m.hlMemo.cells = HighlightSQL(text)
	}
	hl := m.hlMemo.cells
	lines := make([]string, 0, queryEditorH)
	for i := 0; i < queryEditorH; i++ {
		lineIdx := m.editor.OffY + i
		var src string
		if lineIdx < len(m.editor.Lines) {
			src = m.editor.Lines[lineIdx]
		}
		var cells []hlCell
		if lineIdx < len(hl) {
			cells = hl[lineIdx]
		}
		lines = append(lines, m.editorLineView(lineIdx, src, cells, w))
	}
	// Autocomplete popup splices its bordered box into the editor rows so
	// the 8-row budget (and queryResultsTop mouse math) never shifts and
	// the editor text beside the box survives.
	if m.showComplete && m.focusDetail && m.tab == 3 && m.queryFocus == 0 && len(m.completeItems) > 0 {
		box := m.completePopupLines(w)
		top, left, boxW, _ := m.popupGeometry(w)
		for r, bl := range box {
			if top+r >= len(lines) {
				break
			}
			lines[top+r] = spliceCells(lines[top+r], bl, left, boxW, w)
		}
	}
	return strings.Join(lines, "\n")
}

// queryEditorPanel frames the 8 editor rows in a rounded panel with one
// blank padding row above it (written by the caller) separating the edit
// area from the tab/buffer strip. Content lines are padded to the inner
// width so the panel keeps a uniform box.
func (m Model) queryEditorPanel() string {
	w := m.queryEditorInnerW()
	lines := strings.Split(m.queryEditorView(), "\n")
	for i := range lines {
		if pad := w - lipgloss.Width(lines[i]); pad > 0 {
			lines[i] += strings.Repeat(" ", pad)
		}
	}
	focused := m.focusDetail && m.tab == 3 && m.queryFocus == 0
	// v2 Width is border-box: +2 keeps the outer panel at the v1 width
	// (w inner content columns + border) so editor lines never wrap.
	return editorPanelBorder(focused).Width(w + 2).Render(strings.Join(lines, "\n"))
}

// completePopupLines renders the suggestion list as a bordered box: gold
// border on a raised background, keywords gold, fields white, types cyan,
// selected row in the purple selection role. Every line is padded to the
// box width; truncation runs on plain text before styling so rows never
// wrap and mouse hit-testing (popupGeometry) stays exact. Lines carry no
// horizontal offset — the caller splices them into the editor at the
// geometry's left column so surrounding editor text survives.
func (m Model) completePopupLines(w int) []string {
	_, _, boxW, nItems := m.popupGeometry(w)
	cw := boxW - 4 // padding + borders
	if cw < 1 {
		cw = 1
	}
	more := len(m.completeItems) > nItems
	content := make([]string, 0, nItems+1)
	for i, it := range m.completeItems[:min(nItems, len(m.completeItems))] {
		text := fitText(it.Text, cw)
		detail := fitText(" "+it.Detail, cw-lipgloss.Width(text))
		var line string
		switch it.Kind {
		case "keyword":
			line = sqlKeyword.Render(text) + dimStyle.Render(detail)
		case "table":
			line = colFieldStyle.Render(text) + dimStyle.Render(detail)
		default:
			line = colFieldStyle.Render(text) + colTypeStyle.Render(detail)
		}
		if i == m.completeIdx {
			line = dataSelectedStyle.Render(fitText(it.Text+" "+it.Detail, cw))
		}
		if pad := cw - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		content = append(content, line)
	}
	if more {
		foot := fitText("… "+strconv.Itoa(len(m.completeItems)-nItems)+" more", cw)
		line := dimStyle.Render(foot)
		if pad := cw - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		content = append(content, line)
	}
	box := completeBoxStyle.Render(strings.Join(content, "\n"))
	return strings.Split(box, "\n")
}

// editorLineView renders one editor row: dim line number plus highlighted
// code capped to the panel inner width. cells may be empty (blank line).
// The gutter carries one leading space so line numbers sit slightly off
// the panel's left border.
func (m Model) editorLineView(lineIdx int, src string, cells []hlCell, w int) string {
	const gutter = 4 // " %2d " line numbers
	maxCode := w - gutter
	if maxCode < 1 {
		maxCode = 1
	}
	rs := []rune(src)
	styles := make([]lipgloss.Style, len(rs))
	ri := 0
	for _, c := range cells {
		for range c.Text {
			if ri < len(styles) {
				styles[ri] = c.Style
			}
			ri++
		}
	}
	for ; ri < len(styles); ri++ {
		styles[ri] = sqlPlain
	}
	// Width-aware truncation before styling so rows never wrap.
	keep, width := 0, 0
	for keep < len(rs) && width+lipgloss.Width(string(rs[keep])) <= maxCode {
		width += lipgloss.Width(string(rs[keep]))
		keep++
	}
	rs, styles = rs[:keep], styles[:keep]
	var b strings.Builder
	b.WriteString(dimStyle.Render(fmt.Sprintf(" %2d ", lineIdx+1)))
	// Character-precise selection (shift+left/right, same-line drag):
	// highlight only the rune span on this line. Whole-line selections
	// keep the legacy whole-row wrap in finish() below.
	selS, selE := -1, -1
	if _, _, active := m.editor.SelectedRange(); active && !m.editor.selLineWise {
		alo, aco, chi, cco, _ := m.editor.SelectionSpan()
		if lineIdx >= alo && lineIdx <= chi {
			selS = 0
			selE = len(rs)
			if lineIdx == alo {
				selS = aco
			}
			if lineIdx == chi {
				selE = cco
			}
			if selS >= selE {
				selS, selE = -1, -1
			}
		}
	}
	sel := func(i int) bool { return selS >= 0 && i >= selS && i < selE }
	finish := func() string {
		lineStr := b.String()
		if lo, hi, active := m.editor.SelectedRange(); active && m.editor.selLineWise && lineIdx >= lo && lineIdx <= hi {
			// Reuse the grid selection role so block matches app chrome.
			return dataSelectedStyle.Render(lineStr)
		}
		return lineStr
	}
	focused := m.focusDetail && m.tab == 3 && m.queryFocus == 0
	if !focused {
		b.WriteString(dimStyle.Render(string(rs)))
		return finish()
	}
	cur := m.editor.CurCol
	if lineIdx != m.editor.CurLine || cur < 0 {
		for i, r := range rs {
			if sel(i) {
				b.WriteString(dataSelectedStyle.Render(string(r)))
			} else {
				b.WriteString(styles[i].Render(string(r)))
			}
		}
		return finish()
	}
	if cur > len(rs) {
		cur = len(rs)
	}
	for i, r := range rs {
		if sel(i) {
			b.WriteString(dataSelectedStyle.Render(string(r)))
		} else if i == cur {
			b.WriteString(queryCursorStyle.Render(string(r)))
		} else {
			b.WriteString(styles[i].Render(string(r)))
		}
	}
	if cur == len(rs) {
		b.WriteString(queryCursorStyle.Render(" "))
	}
	return finish()
}
