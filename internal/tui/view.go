package tui

// View is the Elm view function: Model -> string. Pure rendering only —
// no state changes. Dispatches to one renderer per screen.

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

func (m Model) tablesView() string {
	header := titleStyle.Render("db-peek") + " " + dimStyle.Render(m.status)
	body := m.list.View()
	foot := dimStyle.Render("↑↓/wheel navigate • hover/click select • 2×click inspect • / filter • r refresh • c conns • q quit")
	if m.loading {
		foot += "  " + "loading…"
	}
	if m.err != "" {
		foot += "\n" + errStyle.Render(m.err)
	}
	return header + "\n" + body + "\n" + foot
}
