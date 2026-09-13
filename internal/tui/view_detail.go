package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) detailView() string {
	// Dim the whole pane while focus sits on the sidebar; full color
	// returns the moment focus moves to the detail.
	dim := !m.focusDetail
	titleStyle, activeTabStyle := titleStyle, activeTab
	if dim {
		titleStyle, activeTabStyle = dimTitleStyle, dimActiveTabStyle
	}
	gridView := (*dataTable).View
	if dim {
		gridView = (*dataTable).ViewDimmed
	}
	var b strings.Builder
	// The title must stay one row for mouse alignment: it wraps only
	// when a table name is absurdly long.
	title, suffix := "> "+m.table, ""
	if m.count >= 0 {
		suffix = fmt.Sprintf("  •  %d rows", m.count)
	}
	if m.width > 0 && lipgloss.Width(title+suffix) > m.paneInnerW() {
		title = "> " + fitText(m.table, m.paneInnerW()-lipgloss.Width("> ")-lipgloss.Width(suffix))
	}
	b.WriteString(titleStyle.Render(title))
	if suffix != "" {
		b.WriteString(dimStyle.Render(suffix))
	}
	b.WriteString("\n\n")
	for i, t := range m.detailTabLabels() {
		switch {
		case i == m.tab:
			b.WriteString(activeTabStyle.Render(t))
		case i == m.hoverTab && !dim:
			b.WriteString(hoverTab.Render(t))
		default:
			b.WriteString(inactiveTab.Render(t))
		}
		b.WriteString(" ")
	}
	b.WriteString("\n\n")
	if m.loading {
		b.WriteString("loading...\n")
	} else {
		switch m.tab {
		case 0:
			if len(m.cols) == 0 {
				b.WriteString("(no columns)\n")
			} else {
				b.WriteString(gridView(&m.colTable) + "\n")
			}
		case 1:
			if len(m.indexes) == 0 {
				b.WriteString("(no indexes)\n")
			} else {
				b.WriteString(gridView(&m.idxTable) + "\n")
			}
			// Show full DDL for the selected index when postgres/sqlite provides it.
			// The row is always rendered while any index has DDL so the
			// footer below never shifts or clips on cursor moves.
			if ddl := m.selectedIndexDDL(); ddl != "" {
				b.WriteString(dimStyle.Render(fitText(ddl, m.paneW())) + "\n")
			} else if m.hasIndexDDL() {
				b.WriteString("\n")
			}
		default:
			b.WriteString(dimStyle.Render(fitText(m.pagerLine(), m.paneW())) + "\n")
			if m.sample == nil || len(m.sample.Rows) == 0 {
				b.WriteString("(no rows)\n")
			} else {
				b.WriteString(gridView(&m.rowTable) + "\n")
			}
		}
	}
	if m.err != "" {
		b.WriteString(errStyle.Render(m.err) + "\n")
	}
	b.WriteString(dimStyle.Render(fitText("hover highlights • click tabs • wheel scroll • 1/2/3 tabs • r reload", m.paneW())))
	return b.String()
}

// paneW is the detail pane's usable width: terminal minus sidebar and separator.
func (m Model) paneW() int {
	return m.paneInnerW()
}
