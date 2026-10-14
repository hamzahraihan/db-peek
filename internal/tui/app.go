package tui

// Package tui is the interactive terminal UI, structured as
// Model-View-Update: model.go holds state, update_*.go and mouse.go
// transition it on messages (msg.go), view_*.go renders it, and
// commands.go performs the DB side effects.

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"peekerdb/internal/saved"
)

// Run starts the fullscreen TUI. An empty connStr opens the saved
// connection picker instead of connecting directly. readOnly refuses
// writes and cell edits with a status note.
func Run(connStr string, store *saved.Store, readOnly bool) error {
	if mouseDebug {
		fmt.Fprintln(os.Stderr, "peekerdb: mouse debug log →", mouseLogPath())
	}
	m := New(connStr, store)
	m.readOnly = readOnly
	p := tea.NewProgram(m)
	final, err := p.Run()
	// One hook covers every exit path (ctrl+c, q in browse, q in the
	// picker): whatever the final model holds is what comes back next run.
	if fm, ok := final.(Model); ok {
		if sess, serr := loadSessionFunc(); serr == nil {
			fm.saveSession(sess) // synchronous: the process is exiting
		}
	}
	return err
}
