package tui

// Transitions for the add/edit connection form screen.

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// formKey drives the add/edit connection form.
func (m Model) formKey(msg tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.screen = screenConns
		m.err = ""
		m.nameInput.Blur()
		m.connInput.Blur()
		return m, nil
	case "tab", "shift+tab", "up", "down":
		if m.formFocus == 0 {
			m.formFocus = 1
			m.nameInput.Blur()
			m.connInput.Focus()
		} else {
			m.formFocus = 0
			m.connInput.Blur()
			m.nameInput.Focus()
		}
		return m, textinput.Blink
	case "enter":
		name := strings.TrimSpace(m.nameInput.Value())
		conn := strings.TrimSpace(m.connInput.Value())
		if name == "" || conn == "" {
			m.err = "name and connection string are both required"
			return m, nil
		}
		if err := m.store.Upsert(name, conn); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.refreshConns()
		m.nameInput.Blur()
		m.connInput.Blur()
		m.connStr = conn
		m.screen = screenBrowse
		m.loading = true
		m.err = ""
		return m, m.openAndLoad(conn)
	}
	var cmd tea.Cmd
	if m.formFocus == 0 {
		m.nameInput, cmd = m.nameInput.Update(msg)
	} else {
		m.connInput, cmd = m.connInput.Update(msg)
	}
	return m, cmd
}
