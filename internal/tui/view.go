package tui

// View is the Elm view function: Model -> string. Pure rendering only —
// no state changes. Dispatches to one renderer per screen.

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
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

// browseView renders the split layout: sidebar table list (left) and
// table detail (right). The separator is one column, and detail content
// is padded to it so rows in both panes share terminal rows.
func (m Model) browseView() string {
	var b strings.Builder
	b.WriteString(m.fitHeader() + "\n")

	side := strings.Split(m.list.View(), "\n")
	right := strings.Split(m.detailView(), "\n")
	h := len(side)
	if len(right) > h {
		h = len(right)
	}
	w := m.sidebarW
	for i := 0; i < h; i++ {
		l, r := "", ""
		if i < len(side) {
			l = side[i]
		}
		if i < len(right) {
			r = right[i]
		}
		if lipgloss.Width(l) > w {
			l = fitText(l, w)
		}
		b.WriteString(l + strings.Repeat(" ", w-lipgloss.Width(l)) + "│" + r + "\n")
	}

	foot := dimStyle.Render(fitText("sidebar: /filter • enter preview • tab detail • r refresh • c conns • q quit", m.width))
	if m.loading {
		foot += "  " + "loading…"
	}
	if m.err != "" {
		foot += "\n" + errStyle.Render(m.err)
	}
	b.WriteString(foot)
	return b.String()
}

// detailView renders the right pane: title, tabs, and grid (view_detail.go).
