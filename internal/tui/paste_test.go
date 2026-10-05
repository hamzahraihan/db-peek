package tui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestEditorInsertTextSingleLine(t *testing.T) {
	e := NewEditor()
	e.SetText("SELECT 1")
	e.CurLine, e.CurCol = 0, 8
	e.InsertText(" FROM foo")
	if got := e.Text(); got != "SELECT 1 FROM foo" {
		t.Fatalf("single-line paste, got %q", got)
	}
	if e.CurCol != len([]rune("SELECT 1 FROM foo")) {
		t.Fatalf("cursor must park at end, got %d", e.CurCol)
	}
}

func TestEditorInsertTextMultiLine(t *testing.T) {
	e := NewEditor()
	e.SetText("ab")
	e.CurLine, e.CurCol = 0, 1 // a|b
	e.InsertText("X\nY\nZ")
	if got := e.Text(); got != "aX\nY\nZb" {
		t.Fatalf("multi-line paste must split rows, got %q", got)
	}
	if e.CurLine != 2 || e.CurCol != 1 {
		t.Fatalf("cursor must land after Z, got %d,%d", e.CurLine, e.CurCol)
	}
}

func TestEditorInsertTextNormalizesCRLFAndNUL(t *testing.T) {
	e := NewEditor()
	e.InsertText("a\r\nb\rc\x00d")
	if got := e.Text(); got != "a\nb\ncd" {
		t.Fatalf("CRLF/CR/NUL must normalize, got %q", got)
	}
	// No embedded newline rune may remain inside a single row.
	for _, ln := range e.Lines {
		for _, r := range ln {
			if r == '\r' || r == '\x00' {
				t.Fatalf("line must not contain control byte, got %q", ln)
			}
		}
	}
}

func TestEditorInsertTextReplacesSelection(t *testing.T) {
	e := NewEditor()
	e.SetText("SELECT 1\nFROM foo\nWHERE x")
	e.CurLine = 0
	e.ExtendSelectionTo(1) // line-wise 0-1
	e.InsertText("SELECT 2")
	if got := e.Text(); got != "SELECT 2\nWHERE x" {
		t.Fatalf("paste must replace selection, got %q", got)
	}
	if _, _, active := e.SelectedRange(); active {
		t.Fatal("paste must clear selection")
	}
}

func TestPasteMsgInsertsIntoEditor(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("SELECT 1")
	m.editor.CurLine, m.editor.CurCol = 0, 8
	u, _ := m.Update(tea.PasteMsg{Content: " FROM foo"})
	m = u.(Model)
	if got := m.editor.Text(); got != "SELECT 1 FROM foo" {
		t.Fatalf("PasteMsg must insert, got %q", got)
	}
}

func TestPasteMsgMultiLine(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("ab")
	m.editor.CurLine, m.editor.CurCol = 0, 1
	u, _ := m.Update(tea.PasteMsg{Content: "X\nY"})
	m = u.(Model)
	if got := m.editor.Text(); got != "aX\nYb" {
		t.Fatalf("multi-line PasteMsg must split, got %q", got)
	}
}

func TestPasteMsgIgnoredInResults(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("SELECT 1")
	m.queryFocus = 1 // results focused
	u, _ := m.Update(tea.PasteMsg{Content: "SHOULD NOT APPEAR"})
	m = u.(Model)
	if got := m.editor.Text(); got != "SELECT 1" {
		t.Fatalf("results paste must not touch editor, got %q", got)
	}
}

func TestCtrlVPasteFromClipboard(t *testing.T) {
	old := clipboardReadAll
	clipboardReadAll = func() (string, error) { return " FROM bar", nil }
	defer func() { clipboardReadAll = old }()
	m := queryTabModel()
	m.editor.SetText("SELECT 1")
	m.editor.CurLine, m.editor.CurCol = 0, 8
	u, _ := m.Update(testKey("ctrl+v"))
	m = u.(Model)
	if got := m.editor.Text(); got != "SELECT 1 FROM bar" {
		t.Fatalf("ctrl+v must paste clipboard, got %q", got)
	}
}

func TestCtrlVPasteFailureStatus(t *testing.T) {
	old := clipboardReadAll
	clipboardReadAll = func() (string, error) { return "", errors.New("no clipboard") }
	defer func() { clipboardReadAll = old }()
	m := queryTabModel()
	m.editor.SetText("SELECT 1")
	u, _ := m.Update(testKey("ctrl+v"))
	m = u.(Model)
	if got := m.editor.Text(); got != "SELECT 1" {
		t.Fatalf("failed paste must not change text, got %q", got)
	}
	if m.status == "" {
		t.Fatal("failed paste must set status")
	}
}
