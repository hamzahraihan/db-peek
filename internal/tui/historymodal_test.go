package tui

import (
	"strings"
	"testing"

	"db-peek/internal/saved"
)

func TestHistoryModalWorkflow(t *testing.T) {
	m := queryTabModel()
	m.tab = 3
	m.queryFocus = 0 // editor focused
	m.connStr = "postgres://localhost/testdb"
	m.history = &saved.History{}
	m.history.Add("SELECT * FROM users", saved.Mask(m.connStr))
	m.history.Add("SELECT count(*) FROM orders", saved.Mask(m.connStr))
	m.history.Add("INSERT INTO items VALUES (1)", saved.Mask(m.connStr))

	// Press ctrl+h to open history modal
	u, _ := m.Update(testKey("ctrl+h"))
	m = u.(Model)
	if !m.historyModal {
		t.Fatal("ctrl+h must open history modal")
	}
	if len(m.historyEntries) != 3 {
		t.Fatalf("expected 3 history entries, got %d", len(m.historyEntries))
	}
	if len(m.historyFiltered) != 3 {
		t.Fatalf("expected 3 filtered entries initially, got %d", len(m.historyFiltered))
	}

	// Filter by typing "users"
	m.historyFilterInput.SetValue("users")
	m.rebuildHistoryFiltered()
	if len(m.historyFiltered) != 1 {
		t.Fatalf("expected 1 filtered entry for 'users', got %d", len(m.historyFiltered))
	}

	// Press Enter to load the filtered query into editor
	u, _ = m.Update(testKey("enter"))
	m = u.(Model)
	if m.historyModal {
		t.Fatal("enter must close history modal")
	}
	if !strings.Contains(m.editor.Text(), "SELECT * FROM users") {
		t.Fatalf("editor must contain selected query, got %q", m.editor.Text())
	}
	if !strings.Contains(m.status, "loaded query from history") {
		t.Fatalf("status must report query loaded, got %q", m.status)
	}

	// Open again, navigate and dismiss with Esc
	u, _ = m.Update(testKey("ctrl+h"))
	m = u.(Model)
	if !m.historyModal {
		t.Fatal("ctrl+h must open history modal again")
	}
	u, _ = m.Update(testKey("down"))
	m = u.(Model)
	u, _ = m.Update(testKey("esc"))
	m = u.(Model)
	if m.historyModal {
		t.Fatal("esc must dismiss history modal")
	}
}

func TestHistoryModalCopy(t *testing.T) {
	var copied string
	old := clipboardWriteAll
	clipboardWriteAll = func(s string) error {
		copied = s
		return nil
	}
	defer func() { clipboardWriteAll = old }()

	m := queryTabModel()
	m.tab = 3
	m.queryFocus = 0
	m.connStr = "postgres://localhost/testdb"
	m.history = &saved.History{}
	m.history.Add("SELECT secret FROM vault", saved.Mask(m.connStr))

	u, _ := m.Update(testKey("ctrl+h"))
	m = u.(Model)
	status := m.copySelectedHistory()
	if !strings.Contains(copied, "SELECT secret FROM vault") {
		t.Fatalf("expected copied query, got %q", copied)
	}
	if !strings.Contains(status, "copied") {
		t.Fatalf("expected copy status, got %q", status)
	}
}
