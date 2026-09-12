package tui

// Mouse transitions: click-to-select, double-click-to-open, wheel scroll,
// hover highlight, tab clicks, and form field focus. Coordinates are
// 0-indexed terminal cells.

import (
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// listChromeH is the fixed header above list items: 2 title rows + 2 status rows
// (bubbles list default styles: TitleBar and StatusBar each pad one blank line).
const listChromeH = 4

// detailTableTop is the first terminal row of a detail table: header, table
// title, blank, tabs, blank, then the table itself.
const detailTableTop = 5

// detailHeaderH is the table's own header height: 1 title row plus the
// bottom border drawn by dataTableStyles.
const detailHeaderH = 2

// handleMouse implements click-to-select, double-click-to-open, wheel scroll,
// hover highlight, tab clicks, and form field focus. Coordinates are
// 0-indexed terminal cells.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.MouseWheelUp:
		return m.wheel(-3)
	case tea.MouseWheelDown:
		return m.wheel(3)
	case tea.MouseMotion:
		return m.hover(msg)
	}
	if msg.Type != tea.MouseLeft || msg.Action != tea.MouseActionPress {
		return m, nil // ignore release
	}
	if m.loading {
		return m, nil
	}
	switch m.screen {
	case screenConns:
		return m.clickList(msg.Y, screenConns)
	case screenTables:
		return m.clickList(msg.Y, screenTables)
	case screenForm:
		return m.clickForm(msg.Y)
	default:
		return m.clickTabs(msg.X, msg.Y)
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
	case screenTables:
		for range steps {
			if up {
				m.list.CursorUp()
			} else {
				m.list.CursorDown()
			}
		}
		return m, nil
	default: // detail: scroll the active tab's table
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
		return m, nil
	}
}

// clickList maps a click row to a picker item: single click selects,
// double-click opens. Items are top-anchored right below the 4 chrome rows.
func (m Model) clickList(y int, which screen) (tea.Model, tea.Cmd) {
	idx, ok := m.listIndexAt(y, which)
	if !ok {
		return m, nil
	}
	var l *list.Model
	if which == screenConns {
		l = &m.conns
	} else {
		l = &m.list
	}
	l.Select(idx)
	if which == m.lastClickWhere && idx == m.lastClickIdx && time.Since(m.lastClickAt) < 500*time.Millisecond {
		m.lastClickAt = time.Time{}
		if which == screenConns {
			if sel, ok := l.SelectedItem().(connItem); ok {
				return m.activateConn(sel.name)
			}
			return m, nil
		}
		if sel, ok := l.SelectedItem().(tableItem); ok {
			return m.inspectTable(sel.name)
		}
		return m, nil
	}
	m.lastClickWhere, m.lastClickIdx, m.lastClickAt = which, idx, time.Now()
	return m, nil
}

// listIndexAt resolves a terminal row to a global item index in a picker.
func (m Model) listIndexAt(y int, which screen) (int, bool) {
	var l *list.Model
	var itemH int
	if which == screenConns {
		l, itemH = &m.conns, m.connsItemH
	} else {
		l, itemH = &m.list, m.tablesItemH
	}
	if itemH <= 0 {
		return 0, false
	}
	row := y - 1 - listChromeH // -1 for our header line
	if row < 0 {
		return 0, false
	}
	perPage := l.Paginator.PerPage
	if perPage <= 0 {
		return 0, false
	}
	vis := l.VisibleItems()
	start := l.Paginator.Page * perPage
	onPage := len(vis) - start
	if onPage > perPage {
		onPage = perPage
	}
	if onPage <= 0 || row/itemH >= onPage {
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
	case screenTables:
		return m.hoverList(msg.Y, screenTables)
	default:
		return m.hoverTable(msg.Y)
	}
}

func (m Model) hoverList(y int, which screen) (tea.Model, tea.Cmd) {
	idx, ok := m.listIndexAt(y, which)
	if !ok {
		return m, nil
	}
	if which == m.lastHoverWhere && idx == m.lastHoverIdx {
		return m, nil
	}
	m.lastHoverWhere, m.lastHoverIdx = which, idx
	if which == screenConns {
		m.conns.Select(idx)
	} else {
		m.list.Select(idx)
	}
	return m, nil
}

// hoverTable highlights the data row under the cursor. The table viewport
// only ever scrolls via MoveUp/MoveDown, so the first visible row is
// derivable as clamp(cursor-height, 0, cursor).
func (m Model) hoverTable(y int) (tea.Model, tea.Cmd) {
	var t *table.Model
	switch m.tab {
	case 0:
		t = &m.colTable
	case 1:
		t = &m.idxTable
	default:
		t = &m.rowTable
	}
	rel := y - detailTableTop - detailHeaderH
	if rel < 0 {
		return m, nil
	}
	h := t.Height()
	cursor := t.Cursor()
	start := cursor - h
	if start < 0 {
		start = 0
	}
	if cursor < start {
		start = cursor
	}
	rows := len(t.Rows())
	end := cursor + h
	if end < cursor {
		end = cursor
	}
	if end > rows {
		end = rows
	}
	abs := start + rel
	if abs < start || abs >= end || abs == cursor {
		return m, nil
	}
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

// clickTabs switches tabs when a tab label is clicked. Tab labels sit on row 3
// (header, table title, blank, tabs) with one space between labels.
func (m Model) clickTabs(x, y int) (tea.Model, tea.Cmd) {
	if y != 3 {
		return m, nil
	}
	xpos := 0
	for i, t := range m.detailTabLabels() {
		w := lipgloss.Width(inactiveTab.Render(t))
		if x >= xpos && x < xpos+w {
			m.setTab(i)
			return m, nil
		}
		xpos += w + 1
	}
	return m, nil
}
