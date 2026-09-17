package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	b.WriteString("\n\n")
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
			b.WriteString(m.queryEditorView() + "\n")
			b.WriteString(dimStyle.Render(fitText("shift+↑↓ select • ctrl+d del • ctrl+s copy • ctrl+/ comment • ctrl+r run", m.paneW())) + "\n")
			if m.querySample == nil {
				if m.queryAffected >= 0 {
					unit := "rows"
					if m.queryAffected == 1 {
						unit = "row"
					}
					b.WriteString(dimStyle.Render(fitText(fmt.Sprintf("%d %s affected • %d ms", m.queryAffected, unit, m.queryMs), m.paneW())) + "\n")
				} else {
					b.WriteString(dimStyle.Render("(no results — ctrl+r to run)") + "\n")
				}
			} else {
				b.WriteString(gridView(&m.queryTable) + "\n")
				if m.queryAffected >= 0 && m.queryPreviewTable != "" {
					unit := "rows"
					if m.queryAffected == 1 {
						unit = "row"
					}
					b.WriteString(dimStyle.Render(fitText(fmt.Sprintf("%d %s affected • preview of %s • %d ms", m.queryAffected, unit, m.queryPreviewTable, m.queryMs), m.paneW())) + "\n")
				} else {
					b.WriteString(dimStyle.Render(fitText(fmt.Sprintf("%d rows • %d ms", len(m.querySample.Rows), m.queryMs), m.paneW())) + "\n")
				}
			}
		case 4:
			b.WriteString(m.erView(m.paneInnerW(), m.paneInnerH()) + "\n")
		default:
			b.WriteString(dimStyle.Render(fitText(m.pagerLine(), m.paneW())) + "\n")
			if m.sample == nil || len(m.sample.Rows) == 0 {
				b.WriteString("(no rows)\n")
			} else {
				b.WriteString(gridView(&m.rowTable) + "\n")
			}
		}
	}
	if m.err != "" {
		b.WriteString(errStyle.Render(m.err) + "\n")
	}
	foot := "hover highlights • click tabs • wheel scroll • 1-5 tabs • r reload"
	if m.tab == 4 {
		if m.erIsFocused() {
			foot = "f all tables • n/p select • enter recenter • wasd/arrows pan • r reload"
		} else {
			foot = "f focused view • n/p select • enter recenter • wasd/arrows pan • r reload"
		}
	}
	b.WriteString(dimStyle.Render(fitText(foot, m.paneW())))
	return b.String()
}

// paneW is the detail pane's usable width: terminal minus sidebar and separator.
func (m Model) paneW() int {
	return m.paneInnerW()
}

// queryEditorH is the fixed editor height (rows) on the query tab.
const queryEditorH = 8

// queryCursorStyle marks the cursor cell while the editor has focus.
var queryCursorStyle = lipgloss.NewStyle().Reverse(true)

// queryResultsTop is the first terminal row of the query results grid:
// editor top + editor rows + the status/hint line.
func queryResultsTop() int { return detailTableTop + queryEditorH + 1 }

// queryEditorView renders the fixed 8-row highlighted editor with dim line
// numbers and a reverse-video cursor cell when the editor has focus.
// HighlightSQL("") yields [[]] and SetText("") yields [""], so empty
// cell-rows render as blank lines and no empty row is ever indexed.
func (m Model) queryEditorView() string {
	w := m.paneInnerW()
	hl := HighlightSQL(m.editor.Text())
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
	// Autocomplete popup overlays the editor rows as a bordered box so the
	// 8-row budget (and queryResultsTop mouse math) never shifts.
	if m.showComplete && m.focusDetail && m.tab == 3 && m.queryFocus == 0 && len(m.completeItems) > 0 {
		box := m.completePopupLines(w)
		top, _, _, _ := m.popupGeometry(w)
		copy(lines[top:], box)
	}
	return strings.Join(lines, "\n")
}

// completePopupLines renders the suggestion list as a bordered box: gold
// border on a raised background, keywords gold, fields white, types cyan,
// selected row in the purple selection role. Every line is padded to the
// box width; truncation runs on plain text before styling so rows never
// wrap and mouse hit-testing (popupGeometry) stays exact.
func (m Model) completePopupLines(w int) []string {
	top, left, boxW, nItems := m.popupGeometry(w)
	_ = top
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
	lines := strings.Split(box, "\n")
	gap := ""
	if left > 0 {
		gap = strings.Repeat(" ", left)
	}
	for i := range lines {
		lines[i] = gap + lines[i]
	}
	return lines
}

// editorLineView renders one editor row: dim line number plus highlighted
// code capped to the pane width. cells may be empty (blank line).
func (m Model) editorLineView(lineIdx int, src string, cells []hlCell, w int) string {
	const gutter = 3 // "%2d " line numbers
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
	b.WriteString(dimStyle.Render(fmt.Sprintf("%2d ", lineIdx+1)))
	finish := func() string {
		lineStr := b.String()
		if lo, hi, active := m.editor.SelectedRange(); active && lineIdx >= lo && lineIdx <= hi {
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
			b.WriteString(styles[i].Render(string(r)))
		}
		return finish()
	}
	if cur > len(rs) {
		cur = len(rs)
	}
	for i, r := range rs {
		if i == cur {
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
