package tui

// View is the Elm view function: Model -> string. Pure rendering only —
// no state changes. Dispatches to one renderer per screen.

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) View() string {
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

// explorerFirstRow is the terminal row of the first explorer tree row:
// y0 header, y1 side top border, y2 title, y3 conn, y4 separator,
// y5 first tree row.
const explorerFirstRow = 5

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

	side := strings.Split(m.explorer.Render(innerW, innerH-4), "\n")
	// Insert a dim separator after the conn line so the first tree row
	// lands at explorerFirstRow: y0=app header, y1=border, y2=title,
	// y3=conn, y4=separator, y5=first tree row.
	if len(side) >= 2 {
		sep := dimStyle.Render(fitText(strings.Repeat("─", innerW), innerW))
		side = append(side[:2], append([]string{sep}, side[2:]...)...)
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
	sideBox := paneBorder(!m.focusDetail).Width(innerW).Height(innerH).Render(strings.Join(side, "\n"))
	detailBox := paneBorder(m.focusDetail).Width(detailW).Height(innerH).Render(strings.Join(right, "\n"))
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

	foot := dimStyle.Render(fitText("sidebar: /filter • enter preview • tab detail • r refresh • c conns • q quit • ? keys", m.width))
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
