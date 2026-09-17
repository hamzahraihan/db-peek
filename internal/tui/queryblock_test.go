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

func TestShiftClickExtendsBlock(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("SELECT 1\nFROM foo\nWHERE x")
	m.width, m.height = 120, 40
	m.resizeBrowse()
	// Plain click line 0 (editor-relative): detailTableTop + 0.
	u, _ := m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: m.paneX() + 4, Y: detailTableTop})
	m = u.(Model)
	if m.editor.CurLine != 0 {
		t.Fatalf("plain click line 0, got %d", m.editor.CurLine)
	}
	u, _ = m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: m.paneX() + 4, Y: detailTableTop + 2, Shift: true})
	m = u.(Model)
	lo, hi, active := m.editor.SelectedRange()
	if !active || lo != 0 || hi != 2 {
		t.Fatalf("shift-click must select 0-2, got %d-%d active=%v", lo, hi, active)
	}
}
