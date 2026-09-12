package tui

// Transitions for the saved-connection picker screen.

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// connsKey drives the saved-connection picker.
func (m Model) connsKey(msg tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q":
		if m.conns.IsFiltered() || m.conns.FilterInput.Focused() {
			break // list consumes it
		}
		return m, tea.Quit
	case "enter":
		sel, ok := m.conns.SelectedItem().(connItem)
		if !ok {
			m.err = "no saved connections yet — press a to add one"
			return m, nil
		}
		return m.activateConn(sel.name)
	case "a":
		m.delArm = ""
		m.editing = ""
		m.nameInput.SetValue("")
		m.connInput.SetValue("")
		m.formFocus = 0
		m.nameInput.Focus()
		m.connInput.Blur()
		m.screen = screenForm
		m.err = ""
		return m, textinput.Blink
	case "e":
		sel, ok := m.conns.SelectedItem().(connItem)
		if !ok {
			return m, nil
		}
		m.delArm = ""
		p, _ := m.store.Get(sel.name)
		m.editing = p.Name
		m.nameInput.SetValue(p.Name)
		m.connInput.SetValue(p.Conn)
		m.formFocus = 1
		m.nameInput.Blur()
		m.connInput.Focus()
		m.screen = screenForm
		m.err = ""
		return m, textinput.Blink
	case "d":
		sel, ok := m.conns.SelectedItem().(connItem)
		if !ok {
			return m, nil
		}
		if m.delArm != sel.name {
			m.delArm = sel.name
			m.status = fmt.Sprintf("press d again to forget %q", sel.name)
			return m, nil
		}
		m.delArm = ""
		if err := m.store.Delete(sel.name); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.refreshConns()
		m.status = fmt.Sprintf("forgot %q", sel.name)
		return m, nil
	}
	m.delArm = ""
	var cmd tea.Cmd
	m.conns, cmd = m.conns.Update(msg)
	return m, cmd
}
