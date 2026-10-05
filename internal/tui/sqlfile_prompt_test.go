package tui

import (
	"os"
	"testing"
)

func TestCtrlOOpensSQLPrompt(t *testing.T) {
	m := queryTabModel()
	m.tab, m.queryFocus, m.focusDetail = 3, 0, true
	u, _ := m.Update(testKey("ctrl+o"))
	m = u.(Model)
	if !m.sqlFiling {
		t.Fatal("ctrl+o must open SQL file prompt")
	}
	if m.sqlFileMode != "open" {
		t.Fatalf("mode must be open, got %q", m.sqlFileMode)
	}
}

func TestSQLSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/q.sql"
	m := queryTabModel()
	m.tab, m.queryFocus, m.focusDetail = 3, 0, true
	m.editor.SetText("SELECT 1")
	m.sqlFiling, m.sqlFileMode = true, "save"
	m.sqlFileInput.SetValue(path)
	m.confirmSQLFile()
	if m.status == "" || m.lastSQLPath != path {
		t.Fatalf("save must set status+path, got %q %q", m.status, m.lastSQLPath)
	}
	m2 := queryTabModel()
	m2.tab, m2.queryFocus, m2.focusDetail = 3, 0, true
	m2.sqlFiling, m2.sqlFileMode = true, "open"
	m2.sqlFileInput.SetValue(path)
	m2.confirmSQLFile()
	if got := m2.editor.Text(); got != "SELECT 1" {
		t.Fatalf("load must restore text, got %q", got)
	}
}

func TestSQLOpenTooLargeKeepsBuffer(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/big.sql"
	big := make([]byte, maxSQLFileBytes+1)
	for i := range big {
		big[i] = 'x'
	}
	if err := os.WriteFile(path, big, 0o600); err != nil {
		t.Fatal(err)
	}
	m := queryTabModel()
	m.tab = 3
	m.editor.SetText("")
	m.sqlFiling, m.sqlFileMode = true, "open"
	m.sqlFileInput.SetValue(path)
	m.confirmSQLFile()
	if got := m.editor.Text(); got != "" {
		t.Fatalf("oversize open must keep buffer, got %q", got)
	}
	if m.err == "" {
		t.Fatal("oversize open must report error")
	}
}
