package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

var errTestClipboard = errors.New("no clipboard")

func TestQueryDumpText(t *testing.T) {
	got := queryDumpText("SELECT 1", "boom")
	if !strings.Contains(got, "SELECT 1") || !strings.Contains(got, "boom") {
		t.Fatalf("dump must contain sql and error, got:\n%s", got)
	}
}

func TestCopyQueryFallsBackToFile(t *testing.T) {
	old := clipboardWriteAll
	clipboardWriteAll = func(string) error { return errTestClipboard }
	defer func() { clipboardWriteAll = old }()

	dir := t.TempDir()
	t.Chdir(dir)
	m := queryTabModel()
	m.editor.SetText("SELECT ord")
	m.err = "invalid message format (SQLSTATE 08P01)"
	status := m.copyQueryDump()
	body, err := os.ReadFile(filepath.Join(dir, "db-peek-debug.txt"))
	if err != nil {
		t.Fatalf("fallback file must exist: %v (status %q)", err, status)
	}
	if !strings.Contains(string(body), "SELECT ord") || !strings.Contains(string(body), "08P01") {
		t.Fatalf("file must contain sql and error, got:\n%s", body)
	}
	if !strings.Contains(status, "db-peek-debug.txt") {
		t.Fatalf("status must name the file, got %q", status)
	}
}

func TestCtrlSCopiesEditorText(t *testing.T) {	var got string
	old := clipboardWriteAll
	clipboardWriteAll = func(s string) error { got = s; return nil }
	defer func() { clipboardWriteAll = old }()

	m := queryTabModel()
	m.editor.SetText("SELECT * FROM orders JOIN customers ON 1=1")
	m.editor.CurLine, m.editor.CurCol = 0, 10
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = u.(Model)
	if !strings.Contains(got, "SELECT * FROM orders") {
		t.Fatalf("clipboard must receive editor text, got %q", got)
	}
	if !strings.Contains(m.status, "clipboard") {
		t.Fatalf("status must confirm copy, got %q", m.status)
	}
}

// NUL bytes (e.g. from pasted text or wire-padded driver errors) crash the
// Windows clipboard API, so the dump must be sanitized before copying.
func TestCopyQueryStripsNUL(t *testing.T) {
	var got string
	old := clipboardWriteAll
	clipboardWriteAll = func(s string) error { got = s; return nil }
	defer func() { clipboardWriteAll = old }()

	m := queryTabModel()
	m.editor.SetText("SELECT\x001")
	m.err = "bad\x00error"
	m.copyQueryDump()
	if strings.ContainsRune(got, 0) {
		t.Fatalf("clipboard text must not contain NUL, got %q", got)
	}
	if !strings.Contains(got, "SELECT1") || !strings.Contains(got, "baderror") {
		t.Fatalf("content must survive NUL stripping, got %q", got)
	}
}

func TestCopySelectionOnly(t *testing.T) {
	var got string
	old := clipboardWriteAll
	clipboardWriteAll = func(s string) error { got = s; return nil }
	defer func() { clipboardWriteAll = old }()
	m := queryTabModel()
	m.editor.SetText("SELECT 1\nFROM foo\nWHERE x")
	m.editor.CurLine = 0
	m.editor.ExtendSelectionTo(1)
	m.err = "boom"
	u, _ := m.queryKeys(tea.KeyMsg{Type: tea.KeyCtrlS}, "ctrl+s")
	m = u.(Model)
	if got != "SELECT 1\nFROM foo" {
		t.Fatalf("selection copy must exclude dump header and error, got %q", got)
	}
	if m.status == "" {
		t.Fatal("status must report the copy")
	}
}

// A panicking clipboard backend (atotto on Windows with hostile input)
// must fall back to the file, never crash the TUI.
func TestCopyQuerySurvivesClipboardPanic(t *testing.T) {
	old := clipboardWriteAll
	clipboardWriteAll = func(string) error { panic("string with NUL passed to StringToUTF16") }
	defer func() { clipboardWriteAll = old }()

	dir := t.TempDir()
	t.Chdir(dir)
	m := queryTabModel()
	m.editor.SetText("SELECT 1")
	status := m.copyQueryDump()
	if _, err := os.Stat(filepath.Join(dir, "db-peek-debug.txt")); err != nil {
		t.Fatalf("panic must fall back to file, status %q: %v", status, err)
	}
}
