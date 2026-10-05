package tui

import (
	"testing"
)

func TestReadOnlyBlocksWriteRun(t *testing.T) {
	m := queryTabModel()
	m.tab, m.queryFocus, m.focusDetail = 3, 0, true
	m.readOnly = true
	m.editor.SetText("DELETE FROM users")
	u, _ := m.Update(testKey("ctrl+r"))
	m = u.(Model)
	if m.loading {
		t.Fatal("read-only must not start a write run")
	}
	if m.status == "" {
		t.Fatal("read-only block must set status")
	}
}

func TestReadOnlyAllowsSelectRun(t *testing.T) {
	m := queryTabModel()
	m.tab, m.queryFocus, m.focusDetail = 3, 0, true
	m.readOnly = true
	m.editor.SetText("SELECT 1")
	u, _ := m.Update(testKey("ctrl+r"))
	m = u.(Model)
	if !m.loading {
		t.Fatalf("read-only must allow SELECT, status %q", m.status)
	}
}

func TestReadOnlyBlocksCellEdit(t *testing.T) {
	m := queryTabModel()
	m.tab, m.focusDetail = 2, true
	m.readOnly = true
	u, _ := m.Update(testKey("e"))
	m = u.(Model)
	if m.cellEditing {
		t.Fatal("read-only must not open the cell editor")
	}
}
