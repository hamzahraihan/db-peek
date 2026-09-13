package tui

// Mouse transitions: click-to-select, double-click-to-open, wheel scroll,
// hover highlight, tab clicks, and form field focus. Coordinates are
// 0-indexed terminal cells.

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// listFirstRow is the terminal row of the first picker item: one app
// header row plus the bubbles list chrome (2 title + 2 status rows).
// The filter input replaces the title block, so this holds in every
// filter state (verified against list.View output).
const listFirstRow = 5

// detailTableTop is the first terminal row of a detail table: header, table
// title, blank, tabs, blank, then the table itself.
const detailTableTop = 5

// handleMouse implements click-to-select, double-click-to-open, wheel scroll,
// hover highlight, tab clicks, and form field focus. Coordinates are
// 0-indexed terminal cells.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	logMouse("event=%q x=%d y=%d screen=%d loading=%v", msg.String(), msg.X, msg.Y, m.screen, m.loading)
	switch msg.Type {
	case tea.MouseWheelUp:
		return m.wheel(-3)
	case tea.MouseWheelDown:
		return m.wheel(3)
	case tea.MouseMotion:
		return m.hover(msg)
	}
	if msg.Type != tea.MouseLeft || msg.Action != tea.MouseActionPress {
		logMouse("  ignored (not left-press)")
		return m, nil // ignore release
	}
	if m.loading {
		return m, nil
	}
	switch m.screen {
	case screenConns:
		if msg.Y >= listFirstRow {
			return m.clickList(msg.Y, screenConns)
		}
		return m, nil
	case screenBrowse:
		if msg.X < m.paneX() {
			if msg.Y >= explorerFirstRow {
				return m.clickExplorer(msg.X, msg.Y)
			}
			return m, nil // sidebar chrome (title/conn/separator): no-op
		}
		if msg.Y == m.tabStripRow() {
			return m.clickTabs(msg.X-m.paneX(), msg.Y)
		}
		return m.clickTable(msg.Y)
	case screenForm:
		return m.clickForm(msg.Y)
	default:
		if msg.Y == m.tabStripRow() {
			return m.clickTabs(msg.X-m.paneX(), msg.Y)
		}
		return m.clickTable(msg.Y)
	}
}

// wheel moves the focused list or detail table by n rows (negative = up).
func (m Model) wheel(n int) (tea.Model, tea.Cmd) {
	if m.loading {
		return m, nil
	}
	up := n < 0
	steps := n
	if steps < 0 {
		steps = -steps
	}
	switch m.screen {
	case screenConns:
		for range steps {
			if up {
				m.conns.CursorUp()
			} else {
				m.conns.CursorDown()
			}
		}
		return m, nil
	case screenBrowse:
		if !m.focusDetail {
			for range steps {
				if up {
					m.explorer.MoveUp()
				} else {
					m.explorer.MoveDown()
				}
			}
		} else {
			switch m.tab {
			case 0:
				if up {
					m.colTable.MoveUp(steps)
				} else {
					m.colTable.MoveDown(steps)
				}
			case 1:
				if up {
					m.idxTable.MoveUp(steps)
				} else {
					m.idxTable.MoveDown(steps)
				}
			default:
				if up {
					m.rowTable.MoveUp(steps)
				} else {
					m.rowTable.MoveDown(steps)
				}
			}
		}
		return m, nil
	default: // conns-adjacent safety; browse handled above
		return m, nil
	}
}

// clickList maps a click row to a conns picker item. Connections need a
// double-click to connect (accidental-connect guard). Browse sidebar
// clicks are handled by clickExplorer.
func (m Model) clickList(y int, which screen) (tea.Model, tea.Cmd) {
	idx, ok := m.listIndexAt(y, which)
	if !ok {
		return m, nil
	}
	l := &m.conns
	l.Select(idx)
	if which == m.lastClickWhere && idx == m.lastClickIdx && time.Since(m.lastClickAt) < 500*time.Millisecond {
		m.lastClickAt = time.Time{}
		if sel, ok := l.SelectedItem().(connItem); ok {
			return m.activateConn(sel.name)
		}
		return m, nil
	}
	m.lastClickWhere, m.lastClickIdx, m.lastClickAt = which, idx, time.Now()
	return m, nil
}

// clickExplorer maps a sidebar click row to an explorer tree row.
// Schema rows toggle collapse; table rows toggle and preview in the
// detail pane (columns load + inspect); column rows preview their table.
// Clicks on sidebar chrome (title/conn/separator) are no-ops — disconnect
// stays on c/esc keys.
func (m Model) clickExplorer(x, y int) (tea.Model, tea.Cmd) {
	idx := y - explorerFirstRow
	r, ok := m.explorer.RowAt(idx)
	if !ok {
		logMouse("  clickExplorer x=%d y=%d -> miss", x, y)
		return m, nil
	}
	m.explorer.Cursor = idx
	switch r.Kind {
	case RowSchema:
		logMouse("  clickExplorer x=%d y=%d -> toggle schema %s", x, y, r.Schema)
		m.explorer.Toggle()
		return m, nil
	case RowTable:
		logMouse("  clickExplorer x=%d y=%d -> preview table %s", x, y, r.Table)
		m.explorer.Toggle()
		colCmd := m.loadColumns(r.Schema, r.Table)
		m2, inspectCmd := m.inspectTable(r.Table)
		m = m2
		return m, tea.Batch(colCmd, inspectCmd)
	case RowColumn:
		logMouse("  clickExplorer x=%d y=%d -> preview column %s.%s", x, y, r.Table, r.Column)
		return m.inspectTable(r.Table)
	default:
		return m, nil
	}
}

// listIndexAt resolves a terminal row to a global item index in the conns
// picker (the only remaining bubbles list; sidebar uses the explorer tree).
func (m Model) listIndexAt(y int, which screen) (idx int, ok bool) {
	idx, ok = m.listIndexAtRaw(y, which)
	logMouse("  listIndexAt y=%d which=%d -> idx=%d ok=%v", y, which, idx, ok)
	return idx, ok
}

func (m Model) listIndexAtRaw(y int, which screen) (int, bool) {
	if which != screenConns {
		return 0, false
	}
	l, itemH := &m.conns, m.connsItemH
	if itemH <= 0 {
		logMouse("  listIndexAtRaw: itemH <= 0 (%d)", itemH)
		return 0, false
	}
	row := y - listFirstRow
	if row < 0 {
		logMouse("  listIndexAtRaw: row < 0 (y=%d firstRow=%d)", y, listFirstRow)
		return 0, false
	}
	perPage := l.Paginator.PerPage
	if perPage <= 0 {
		logMouse("  listIndexAtRaw: perPage <= 0 (%d)", perPage)
		return 0, false
	}
	vis := l.VisibleItems()
	start := l.Paginator.Page * perPage
	onPage := len(vis) - start
	if onPage > perPage {
		onPage = perPage
	}
	if onPage <= 0 || row/itemH >= onPage {
		logMouse("  listIndexAtRaw: out of range onPage=%d row=%d itemH=%d len(vis)=%d", onPage, row, itemH, len(vis))
		return 0, false // empty padding
	}
	return start + row/itemH, true
}

// hover moves the highlight to follow the mouse without activating anything.
// Motion events are deduplicated so holding the cursor still is a no-op.
func (m Model) hover(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.loading {
		return m, nil
	}
	switch m.screen {
	case screenConns:
		return m.hoverList(msg.Y, screenConns)
	case screenBrowse:
		if msg.X < m.paneX() {
			return m.hoverList(msg.Y, screenBrowse)
		}
		m.hoverTab = -1
		if msg.Y == m.tabStripRow() {
			return m.hoverTabs(msg.X - m.paneX()), nil
		}
		return m.hoverTable(msg.Y)
	default:
		if msg.Y == m.tabStripRow() {
			return m.hoverTabs(msg.X - m.paneX()), nil
		}
		m.hoverTab = -1
		return m.hoverTable(msg.Y)
	}
}

// hoverTabs highlights the tab under the cursor without switching to it.
func (m Model) hoverTabs(x int) Model {
	m.hoverTab = m.tabAtX(x)
	return m
}

// tabAtX resolves a terminal column to a tab index, or -1 on gaps.
func (m Model) tabAtX(x int) int {
	xpos := 0
	for i, t := range m.detailTabLabels() {
		w := lipgloss.Width(inactiveTab.Render(t))
		if x >= xpos && x < xpos+w {
			return i
		}
		xpos += w + 1
	}
	return -1
}

func (m Model) hoverList(y int, which screen) (tea.Model, tea.Cmd) {
	if which == screenBrowse {
		// Sidebar hover follows the explorer cursor; no preview.
		idx := y - explorerFirstRow
		if _, ok := m.explorer.RowAt(idx); !ok {
			return m, nil
		}
		m.explorer.Cursor = idx
		return m, nil
	}
	idx, ok := m.listIndexAt(y, which)
	if !ok {
		return m, nil
	}
	if which == m.lastHoverWhere && idx == m.lastHoverIdx {
		return m, nil
	}
	m.lastHoverWhere, m.lastHoverIdx = which, idx
	m.conns.Select(idx)
	return m, nil
}

// hoverTable highlights the data row under the cursor. The grid viewport
// only moves on explicit scroll actions, so unlike the old widget the
// content cannot shift under a stationary mouse. Hover is separate from
// selection: keyboard context (cursor, DDL echo) is untouched.
func (m Model) hoverTable(y int) (tea.Model, tea.Cmd) {
	var t *dataTable
	switch m.tab {
	case 0:
		t = &m.colTable
	case 1:
		t = &m.idxTable
	default:
		t = &m.rowTable
	}
	top := detailTableTop
	if m.tab == 2 {
		top++ // pager line above the rows table
	}
	abs, ok := t.RowAt(y - top)
	if !ok {
		t.SetHover(-1)
		return m, nil
	}
	logMouse("  hoverTable y=%d -> hover %d", y, abs)
	t.SetHover(abs)
	return m, nil
}

// clickTable selects the data row under the cursor.
func (m Model) clickTable(y int) (tea.Model, tea.Cmd) {
	var t *dataTable
	switch m.tab {
	case 0:
		t = &m.colTable
	case 1:
		t = &m.idxTable
	default:
		t = &m.rowTable
	}
	top := detailTableTop
	if m.tab == 2 {
		top++ // pager line above the rows table
	}
	abs, ok := t.RowAt(y - top)
	if !ok {
		return m, nil
	}
	logMouse("  clickTable y=%d -> select %d", y, abs)
	t.SetCursor(abs)
	return m, nil
}

// clickForm focuses the clicked field. Layout: title, blank, name label,
// name input, blank, conn label, conn input — so inputs sit on rows 3 and 6.
func (m Model) clickForm(y int) (tea.Model, tea.Cmd) {
	switch y {
	case 3:
		m.formFocus = 0
		m.nameInput.Focus()
		m.connInput.Blur()
		return m, textinput.Blink
	case 6:
		m.formFocus = 1
		m.nameInput.Blur()
		m.connInput.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

// clickTabs switches tabs when a tab label is clicked. The strip row is
// located in the rendered view (labels joined by single spaces) rather
// than hardcoded, so clicks track the layout if the header above grows.
func (m Model) clickTabs(x, y int) (tea.Model, tea.Cmd) {
	row := m.tabStripRow()
	if y != row {
		logMouse("  clickTabs x=%d y=%d stripRow=%d -> miss", x, y, row)
		return m, nil
	}
	if i := m.tabAtX(x); i >= 0 {
		logMouse("  clickTabs x=%d y=%d -> tab %d", x, y, i)
		m.setTab(i)
		return m, nil
	}
	logMouse("  clickTabs x=%d y=%d -> gap", x, y)
	return m, nil
}

// tabStripRow finds the tab strip in the rendered view by its labels.
// Only the head of the view is scanned, skipping header/title (which can
// contain digits): the data table below could theoretically match too.
func (m Model) tabStripRow() int {
	labels := m.detailTabLabels()
	lines := strings.Split(m.View(), "\n")
	for i, ln := range lines {
		if i > 8 {
			break
		}
		if i < 2 {
			continue
		}
		match := true
		for _, t := range labels {
			if !strings.Contains(ln, t) {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return 3 // layout default: header, title, blank, tabs
}
