package tui

// Package tui is the interactive terminal UI, structured as
// Model-View-Update: model.go holds state, update_*.go and mouse.go
// transition it on messages (msg.go), view_*.go renders it, and
// commands.go performs the DB side effects.

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"db-peek/internal/saved"
)

// Run starts the fullscreen TUI. An empty connStr opens the saved
// connection picker instead of connecting directly.
func Run(connStr string, store *saved.Store) error {
	if mouseDebug {
		fmt.Fprintln(os.Stderr, "db-peek: mouse debug log →", mouseLogPath())
	}
	m := New(connStr, store)
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}
