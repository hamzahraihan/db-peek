package tui

import (
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/lipgloss"
)

// All lipgloss styling lives here so screens share one palette.
var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	activeTab   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("4")).Padding(0, 2)
	inactiveTab = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Padding(0, 2)
	// Line highlighter: the selected row anywhere (pickers, data tables).
	selTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Padding(0, 1)
	selDesc  = lipgloss.NewStyle().Foreground(lipgloss.Color("254")).Background(lipgloss.Color("62")).Padding(0, 1)
)

// dataTableStyles gives every data table a bright selected row and a
// distinct header so the cursor is never lost in plain text.
func dataTableStyles() table.Styles {
	s := table.DefaultStyles()
	s.Header = s.Header.Foreground(lipgloss.Color("12")).BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("8"))
	s.Selected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62"))
	return s
}
