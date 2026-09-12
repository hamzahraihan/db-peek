package tui

import "strings"

func (m Model) connsView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("db-peek — connections") + "\n")
	body := m.conns.View()
	foot := dimStyle.Render("↑↓/wheel navigate • hover/click select • 2×click connect • a add • e edit • d forget • q quit")
	if m.loading {
		foot += "  " + "connecting…"
	}
	if m.status != "" {
		foot += "\n" + dimStyle.Render(m.status)
	}
	if m.err != "" {
		foot += "\n" + errStyle.Render(m.err)
	}
	b.WriteString(body + "\n" + foot)
	return b.String()
}
