package tui

// View is the Elm view function: Model -> string. Pure rendering only —
// no state changes. Dispatches to one renderer per screen.

import "github.com/charmbracelet/lipgloss"

func (m Model) View() string {
	switch m.screen {
	case screenConns:
		return m.connsView()
	case screenForm:
		return m.formView()
	case screenTables:
		return m.tablesView()
	default:
		return m.detailView()
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

func (m Model) tablesView() string {
	header := m.fitHeader()
	body := m.list.View()
	foot := dimStyle.Render(fitText("↑↓/wheel navigate • hover/click select • 2×click inspect • / filter • r refresh • c conns • q quit", m.width))
	if m.loading {
		foot += "  " + "loading…"
	}
	if m.err != "" {
		foot += "\n" + errStyle.Render(m.err)
	}
	return header + "\n" + body + "\n" + foot
}
