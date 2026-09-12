package tui

import (
	"fmt"
	"strings"
)

func (m Model) detailView() string {
	header := titleStyle.Render("db-peek") + " " + dimStyle.Render(m.status)
	var b strings.Builder
	b.WriteString(header + "\n")
	b.WriteString(titleStyle.Render("⌂ " + m.table))
	if m.count >= 0 {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  •  %d rows", m.count)))
	}
	b.WriteString("\n\n")
	for i, t := range m.detailTabLabels() {
		if i == m.tab {
			b.WriteString(activeTab.Render(t))
		} else {
			b.WriteString(inactiveTab.Render(t))
		}
		b.WriteString(" ")
	}
	b.WriteString("\n\n")
	if m.loading {
		b.WriteString("loading…\n")
	} else {
		switch m.tab {
		case 0:
			if len(m.cols) == 0 {
				b.WriteString("(no columns)\n")
			} else {
				b.WriteString(m.colTable.View() + "\n")
			}
		case 1:
			if len(m.indexes) == 0 {
				b.WriteString("(no indexes)\n")
			} else {
				b.WriteString(m.idxTable.View() + "\n")
			}
			// Show full DDL for the selected index when postgres/sqlite provides it.
			if ddl := m.selectedIndexDDL(); ddl != "" {
				b.WriteString(dimStyle.Render(ddl) + "\n")
			}
		default:
			b.WriteString(dimStyle.Render(m.pagerLine()) + "\n")
			if m.sample == nil || len(m.sample.Rows) == 0 {
				b.WriteString("(no rows)\n")
			} else {
				b.WriteString(m.rowTable.View() + "\n")
			}
		}
	}
	if m.err != "" {
		b.WriteString(errStyle.Render(m.err) + "\n")
	}
	b.WriteString(dimStyle.Render("hover highlights • click tabs • wheel scroll • 1/2/3 tabs • r reload • esc back • q quit (from list)") + "\n")
	return b.String()
}
