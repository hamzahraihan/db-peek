package tui

// View is the Elm view function: Model -> string. Pure rendering only —
// no state changes. Dispatches to one renderer per screen.

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) View() string {
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

// explorerFirstRow is the terminal row of the first explorer tree row:
// one app header row plus the explorer chrome (title + conn + separator).
const explorerFirstRow = 4

// browseView renders the split layout: sidebar explorer tree (left) and
// table detail (right). The separator is one column, and detail content
// is padded to it so rows in both panes share terminal rows.
func (m Model) browseView() string {
	var b strings.Builder
	b.WriteString(m.fitHeader() + "\n")

	side := strings.Split(m.explorer.Render(m.sidebarW, m.contentH()), "\n")
	// Insert a dim separator after the conn line so the first tree row
	// lands at explorerFirstRow: y0=app header, y1=title, y2=conn,
	// y3=separator, y4=first tree row. Render height semantics unchanged.
	if len(side) >= 2 {
		sep := dimStyle.Render(fitText(strings.Repeat("─", m.sidebarW), m.sidebarW))
		side = append(side[:2], append([]string{sep}, side[2:]...)...)
	}
	right := strings.Split(m.detailView(), "\n")
	h := len(side)
	if len(right) > h {
		h = len(right)
	}
	w := m.sidebarW
	for i := range h {
		l, r := "", ""
		if i < len(side) {
			l = side[i]
		}
		if i < len(right) {
			r = right[i]
		}
		if lipgloss.Width(l) > w {
			l = ansi.Truncate(l, w, "...")
		}
		pad := w - lipgloss.Width(l)
		if pad < 0 {
			pad = 0
		}
		b.WriteString(l + strings.Repeat(" ", pad) + "│" + r + "\n")
	}

	foot := dimStyle.Render(fitText("sidebar: /filter • enter preview • tab detail • r refresh • c conns • q quit", m.width))
	if m.loading {
		foot += "  " + "loading..."
	}
	if m.err != "" {
		foot += "\n" + errStyle.Render(m.err)
	}
	b.WriteString(foot)
	return b.String()
}

// detailView renders the right pane: title, tabs, and grid (view_detail.go).
