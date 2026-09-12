package tui

// Package tui is the interactive terminal UI, structured as
// Model-View-Update: model.go holds state, update_*.go and mouse.go
// transition it on messages (msg.go), view_*.go renders it, and
// commands.go performs the DB side effects.

import (
	tea "github.com/charmbracelet/bubbletea"

	"db-peek/internal/saved"
)

// Run starts the fullscreen TUI. An empty connStr opens the saved
// connection picker instead of connecting directly.
func Run(connStr string, store *saved.Store) error {
	m := New(connStr, store)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
