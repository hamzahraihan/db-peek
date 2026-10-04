package tui

// View is the Elm view function: Model -> string. Pure rendering only —
// no state changes. Dispatches to one renderer per screen.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) View() tea.View {
	v := tea.NewView(m.viewString())
	// Fullscreen alt-screen + cell-motion mouse: the v1 program options
	// (WithAltScreen, WithMouseCellMotion) moved to View fields in v2.
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// viewString renders the current screen to a plain string. View wraps it
// in a tea.View; tests and mouse hit-testing use the string directly.
func (m Model) viewString() string {
	if m.showHelp {
		return m.helpView()
	}
	switch m.screen {
	case screenConns:
		return m.connsView()
	case screenForm:
		return m.formView()
	default:
		return m.browseView()
	}
}

// fitHeader renders "db-peek <status>" truncated to the terminal width.
// The header must stay one row: wrapping would shift every mouse row below.
func (m Model) fitHeader() string {
	const title = "db-peek "
	status := m.status
	if m.width > 0 {
		status = fitText(status, m.width-lipgloss.Width(title))
	}
	return titleStyle.Render("db-peek") + " " + dimStyle.Render(status)
}

// sideBoxTop is the terminal row of both pane top borders (just below header).
const sideBoxTop = 1

// Sidebar chrome rows, counted from the top of the terminal. The pane
// border takes y1, then the connection row and the three-row search
// field box, so the tree starts on explorerFirstRow.
const (
	sideConnRow   = 2
	sideSearchTop = 3 // field box top border
	sideSearchRow = 4 // field content row
	// searchPad is the horizontal chrome outside the field's value area:
	// two border cells, one space of padding per side, and the trailing
	// cell the × affordance occupies.
	searchPad = 5
	explorerFirstRow = 6
)

// searchClearX is the terminal column of the field's × affordance: the
// last content cell inside the field box, which sits one padding cell
// and one border cell left of the pane's own border.
func (m Model) searchClearX() int { return m.sidebarW - searchPad + 1 }

// browseView renders the split layout: sidebar explorer tree (left) and
// table detail (right), each wrapped in a rounded border. Boxes join with
// one space gap; both have fixed outer height contentH() so rows align.
func (m Model) browseView() string {
	var b strings.Builder
	b.WriteString(m.fitHeader() + "\n")

	innerW := m.sidebarW - 2
	if innerW < 1 {
		innerW = 1
	}
	detailW := m.paneInnerW()
	innerH := m.contentH() - 2
	if innerH < 1 {
		innerH = 1
	}

	// Sidebar chrome: the framed search field sits directly under the
	// connection row, above the tree, so the first tree row still lands
	// on explorerFirstRow and every mouse row below it is unaffected.
	side := strings.Split(m.explorer.Render(innerW, m.sidebarTreeH()), "\n")
	if len(side) >= 1 {
		field := m.searchBox(innerW)
		side = append(side[:1], append(field, side[1:]...)...)
	}
	// Sidebar footer/empty states (visual lines only, appended AFTER tree
	// rows so explorerFirstRow hit-testing is unchanged and the cursor
	// never lands on them). Render height semantics stay in explorer.go.
	totalTables := 0
	for _, s := range m.explorer.Schemas {
		totalTables += len(s.Tables)
	}
	if totalTables == 0 {
		side = append(side, dimStyle.Render(fitText("(no tables)", innerW)))
	} else {
		for _, s := range m.explorer.Schemas {
			if len(s.Tables) == 0 {
				side = append(side, dimStyle.Render(fitText("  (empty) "+s.Name, innerW)))
			}
		}
	}
	n := len(m.explorer.Schemas)
	schemaFooter := "1 schema"
	if n != 1 {
		schemaFooter = fmt.Sprintf("%d schemas", n)
	}
	side = append(side, explorerTitle.Render(fitText(schemaFooter, innerW)))
	// Cap sidebar inner lines to innerW and pad/truncate to innerH.
	for i, ln := range side {
		if lipgloss.Width(ln) > innerW {
			side[i] = ansi.Truncate(ln, innerW, "...")
		}
	}
	for len(side) < innerH {
		side = append(side, "")
	}
	if len(side) > innerH {
		side = side[:innerH]
	}
	right := strings.Split(m.detailView(), "\n")
	for i, ln := range right {
		if lipgloss.Width(ln) > detailW {
			right[i] = ansi.Truncate(ln, detailW, "...")
		}
	}
	for len(right) < innerH {
		right = append(right, "")
	}
	if len(right) > innerH {
		right = right[:innerH]
	}
	// v2 Width/Height are border-box (border inside): +2 restores the v1
	// outer geometry (sidebarW / detailW+2 wide, contentH tall) that
	// mouse hit-testing is computed against.
	sideBox := paneBorder(!m.focusDetail).Width(innerW+2).Height(innerH+2).Render(strings.Join(side, "\n"))
	detailBox := paneBorder(m.focusDetail).Width(detailW+2).Height(innerH+2).Render(strings.Join(right, "\n"))
	sideLines := strings.Split(sideBox, "\n")
	detailLines := strings.Split(detailBox, "\n")
	h := len(sideLines)
	if len(detailLines) > h {
		h = len(detailLines)
	}
	for i := range h {
		l, r := "", ""
		if i < len(sideLines) {
			l = sideLines[i]
		}
		if i < len(detailLines) {
			r = detailLines[i]
		}
		b.WriteString(l + " " + r + "\n")
	}

	foot := dimStyle.Render("sidebar: ") + renderKeyPairs([][2]string{
		{"/", "filter"}, {"enter", "preview"}, {"tab", "detail"},
		{"r", "refresh"}, {"c", "conns"}, {"q", "quit"}, {"?", "keys"},
	}, m.width-lipgloss.Width("sidebar: "))
	if m.loading {
		foot += "  " + statusStyle.Render("loading...")
	}
	if m.err != "" {
		foot += "\n" + errStyle.Render(m.err)
	}
	b.WriteString(foot)
	return b.String()
}

// searchBox renders the sidebar's table search field inside its own
// border: top rule, field row, bottom rule. Always present, so the
// filter is discoverable without pressing "/". The border follows the
// pane rule — gold while the field has focus, dim otherwise — so
// keystrokes visibly belong to the field. The value line is trimmed to
// the inner width, so the box can never wrap and shift the tree below.
func (m Model) searchBox(w int) []string {
	inner := w - searchPad + 1
	if inner < 1 {
		inner = 1
	}
	line := m.filterInput.View()
	if !m.filtering && m.explorer.Filter != "" {
		line = setLastCell(line, dimStyle.Render("×"), inner)
	}
	border := lipgloss.Color("#3A3A3A")
	if m.filtering {
		border = lipgloss.Color("#EAB308")
	}
	return strings.Split(lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Width(w).
		Render(ansi.Truncate(line, inner, "")), "\n")
}

// detailView renders the right pane: title, tabs, and grid (view_detail.go).
