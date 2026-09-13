package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// All lipgloss styling lives here so screens share one palette.
var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	activeTab   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(lipgloss.Color("#005FD7")).Padding(0, 2)
	inactiveTab = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Background(lipgloss.Color("236")).Padding(0, 2)
	hoverTab    = lipgloss.NewStyle().Underline(true).Bold(true).Foreground(lipgloss.Color("#00D7FF")).Background(lipgloss.Color("236")).Padding(0, 2)
	// Line highlighter: the selected row anywhere (pickers, data tables).
	selTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Padding(0, 1)
	selDesc  = lipgloss.NewStyle().Foreground(lipgloss.Color("254")).Background(lipgloss.Color("62")).Padding(0, 1)
)

// Grid styles for the custom detail dataTable.
var (
	dataHeaderStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	dataHeaderBorder  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	dataSelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62"))
	dataHoverStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("236"))
)

// Dim variants for the detail pane when focus sits on the sidebar.
// Same layout, muted palette, so the eye is drawn to the focused pane.
var (
	dimTitleStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	dimActiveTabStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("8")).Background(lipgloss.Color("238")).Padding(0, 2)
	dimDataHeaderStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	dimDataSelectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Background(lipgloss.Color("236"))
)

// fitText truncates s to w terminal cells so a rendered line can never
// wrap: a wrapped header would push every row below it down one line and
// desync mouse coordinates, which are row-exact. A non-positive w means
// "unknown width" and leaves s untouched.
func fitText(s string, w int) string {
	if w <= 0 || lipgloss.Width(s) <= w {
		return s
	}
	if w <= 3 {
		return "..."[:w]
	}
	runes := []rune(s)
	width := 0
	i := 0
	for ; i < len(runes); i++ {
		rw := lipgloss.Width(string(runes[i]))
		if width+rw > w-3 {
			break
		}
		width += rw
	}
	return string(runes[:i]) + "..."
}

// Explorer styles for the sidebar tree (Task 3). Gold selection #CA8A04,
// cyan-dim column types, muted counts, bright connection header.
var (
	explorerTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#EAB308"))
	explorerSel   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#000000")).Background(lipgloss.Color("#CA8A04")) // hex governs per human ruling; quantizes to 172 under ANSI256
	explorerType  = lipgloss.NewStyle().Foreground(lipgloss.Color("#67E8F9"))
	explorerCount = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	explorerConn  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
)
