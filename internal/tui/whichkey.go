package tui

// Which-key help overlay: a centered registry of every handled key,
// grouped by context header in registry order. helpRect is shared by
// render (helpView) and mouse hit-testing so the two can never drift.

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// KeyBinding is one handled key (slash-separated aliases) with its
// description and the comma-separated contexts it applies in.
type KeyBinding struct {
	Key      string
	Desc     string
	Contexts string
}

var keyRegistry = []KeyBinding{
	{"up/k", "move up", "Sidebar,Detail,Query-results,ER"},
	{"down/j", "move down", "Sidebar,Detail,Query-results,ER"},
	{"f", "focused/all tables", "ER"},
	{"n/p", "select box", "ER"},
	{"enter", "recenter diagram", "ER"},
	{"left/right", "collapse/expand", "Sidebar"},
	{"enter", "preview table", "Sidebar"},
	{"/", "filter tables (enter keeps, esc clears)", "Sidebar"},
	{"r", "refresh", "Sidebar,Detail"},
	{"c/esc", "back to connections", "Sidebar"},
	{"q", "quit", "Global"},
	{"tab/shift+tab", "switch pane", "Global"},
	{"h/l", "prev/next tab", "Detail"},
	{"esc/backspace", "back to sidebar", "Detail"},
	{"home/end", "top/bottom of grid", "Detail"},
	{"1/2/3/4/5", "schema/indexes/rows/query/er tab", "Detail"},
	{"n/p", "next/prev rows page", "Detail"},
	{"s", "cycle page size", "Detail"},
	{"pgup/pgdown", "page grid", "Detail"},
	{"ctrl+u/ctrl+d", "half-page grid", "Detail"},
	{"g/G", "top/bottom of grid", "Detail"},
	{"ctrl+r/F5", "run query", "Query editor"},
	{"ctrl+s", "copy query+error", "Query editor"},
	{"esc", "editor to results", "Query editor"},
	{"shift+up/down", "select block", "Query editor"},
	{"ctrl+d", "delete line/block", "Query editor"},
	{"ctrl+/", "toggle -- comment", "Query editor"},
	{"?", "this help", "Global"},
	{"a/e/d", "add/edit/forget connection", "Connections"},
	{"enter", "connect", "Connections"},
}

// helpGroups folds the registry into context groups, preserving the
// registry's first-occurrence order.
func helpGroups() []struct {
	header string
	cells  []string
} {
	var groups []struct {
		header string
		cells  []string
	}
	for _, b := range keyRegistry {
		if len(groups) == 0 || groups[len(groups)-1].header != b.Contexts {
			groups = append(groups, struct {
				header string
				cells  []string
			}{header: b.Contexts})
		}
		groups[len(groups)-1].cells = append(groups[len(groups)-1].cells, b.Key+"  "+b.Desc)
	}
	return groups
}

// helpContentLines renders the overlay body: a title plus one context
// header per group with its `key desc` pairs flowed into up to 3
// columns. Content width is capped to the terminal so the box never
// exceeds it; fewer columns are used when 3 would overflow.
func (m Model) helpContentLines() []string {
	maxW := m.width - 6 // border + margin
	if m.width <= 0 {
		maxW = 1 << 30 // unknown width: no cap
	}
	var lines []string
	lines = append(lines, titleStyle.Render("keys")+dimStyle.Render("  ?/esc close"))
	for _, g := range helpGroups() {
		lines = append(lines, explorerTitle.Render(g.header))
		cols := 3
		for ; cols > 1; cols-- {
			if helpRowW(g.cells, cols) <= maxW {
				break
			}
		}
		for i := 0; i < len(g.cells); i += cols {
			end := i + cols
			if end > len(g.cells) {
				end = len(g.cells)
			}
			lines = append(lines, helpRow(g.cells[i:end], cols, maxW))
		}
	}
	return lines
}

// helpRowW is the display width of cells flowed into cols columns
// (3-space gutters), used to pick the column count that fits.
func helpRowW(cells []string, cols int) int {
	w := 0
	for i := 0; i < len(cells); i += cols {
		end := i + cols
		if end > len(cells) {
			end = len(cells)
		}
		cw := 0
		for _, c := range cells[i:end] {
			if ww := lipgloss.Width(c); ww > cw {
				cw = ww
			}
		}
		rowW := cw*len(cells[i:end]) + 3*(len(cells[i:end])-1)
		if rowW > w {
			w = rowW
		}
	}
	return w
}

// helpRow pads one row of cells to equal column width so the 3-column
// flow stays aligned.
func helpRow(cells []string, cols, maxW int) string {
	cw := 0
	for _, c := range cells {
		if ww := lipgloss.Width(c); ww > cw {
			cw = ww
		}
	}
	if total := cw*cols + 3*(cols-1); total > maxW && maxW > 0 {
		cw = (maxW - 3*(cols-1)) / cols
		if cw < 1 {
			cw = 1
		}
	}
	parts := make([]string, len(cells))
	for i, c := range cells {
		for lipgloss.Width(c) < cw {
			c += " "
		}
		parts[i] = c
	}
	return strings.Join(parts, "   ")
}

// helpBox renders the overlay box: content in a rounded gold border.
func (m Model) helpBox() string {
	lines := m.helpContentLines()
	if m.height > 0 && len(lines)+2 > m.height {
		lines = lines[:m.height-2] // terminal-capped, top-anchored
	}
	return paneBorder(true).Render(strings.Join(lines, "\n"))
}

// helpRect is the overlay rect in terminal cells, shared by render and
// mouse hit-testing. Derived from the rendered box so the two agree by
// construction.
func (m Model) helpRect() (x, y, w, h int) {
	lines := strings.Split(m.helpBox(), "\n")
	h = len(lines)
	for _, ln := range lines {
		if ww := lipgloss.Width(ln); ww > w {
			w = ww
		}
	}
	if m.width > 0 && w > m.width {
		w = m.width
	}
	if m.height > 0 && h > m.height {
		h = m.height
	}
	if m.width > w {
		x = (m.width - w) / 2
	}
	if m.height > h {
		y = (m.height - h) / 2
	}
	return x, y, w, h
}

// helpView renders the current screen with the help overlay centered on
// top. Overlay lines replace base lines row-for-row (left-padded to the
// centered x, truncated to the terminal width) so the width can never
// exceed the terminal.
func (m Model) helpView() string {
	var base string
	switch m.screen {
	case screenConns:
		base = m.connsView()
	case screenForm:
		base = m.formView()
	default:
		base = m.browseView()
	}
	baseLines := strings.Split(base, "\n")
	boxLines := strings.Split(m.helpBox(), "\n")
	x, y, w, h := m.helpRect()
	termW := m.width
	if termW <= 0 {
		termW = w
	}
	for i := 0; i < h && i < len(boxLines) && y+i < len(baseLines); i++ {
		row := ansi.Truncate(boxLines[i], w, "")
		if rw := lipgloss.Width(row); x+rw > termW {
			row = ansi.Truncate(row, termW-x, "")
		}
		row = strings.Repeat(" ", x) + row
		if lw := lipgloss.Width(row); lw < termW {
			row += strings.Repeat(" ", termW-lw)
		}
		baseLines[y+i] = row
	}
	return strings.Join(baseLines, "\n")
}
