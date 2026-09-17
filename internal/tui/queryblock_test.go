package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestShiftUpExtendsAndCtrlDDeletesLine(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("SELECT 1\nFROM foo\nWHERE x")
	m.editor.CurLine, m.editor.CurCol = 2, 0
	m.refreshCompletion()
	m.showComplete = false
	u, _ := m.queryKeys(tea.KeyMsg{Type: tea.KeyShiftUp}, "shift+up")
	m = u.(Model)
	if _, _, active := m.editor.SelectedRange(); !active {
		t.Fatal("shift+up must activate selection")
	}
	u, _ = m.queryKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}, "x")
	m = u.(Model)
	if _, _, active := m.editor.SelectedRange(); active {
		t.Fatal("plain typing must clear selection")
	}
	m.editor.SetText("SELECT 1\nFROM foo")
	m.editor.CurLine = 0
	m.editor.ClearSelection()
	u, _ = m.queryKeys(tea.KeyMsg{Type: tea.KeyCtrlD}, "ctrl+d")
	m = u.(Model)
	if got := m.editor.Text(); got != "FROM foo" {
		t.Fatalf("ctrl+d on single line, got %q", got)
	}
}

func TestCommentToggleBothKeyStrings(t *testing.T) {
	for _, key := range []string{"ctrl+/", "ctrl+_"} {
		m := queryTabModel()
		m.editor.SetText("SELECT 1")
		m.showComplete = false
		u, _ := m.queryKeys(tea.KeyMsg{Type: tea.KeyRunes}, key)
		m = u.(Model)
		if got := m.editor.Text(); got != "-- SELECT 1" {
			t.Fatalf("%s: want commented, got %q", key, got)
		}
		u, _ = m.queryKeys(tea.KeyMsg{Type: tea.KeyRunes}, key)
		m = u.(Model)
		if got := m.editor.Text(); got != "SELECT 1" {
			t.Fatalf("%s: want uncommented, got %q", key, got)
		}
	}
}
