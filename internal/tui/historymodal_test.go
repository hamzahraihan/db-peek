package tui

import (
	"strings"
	"testing"

	dbpkg "peekerdb/internal/db"
	"peekerdb/internal/saved"
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
	u, _ = m.Update(testKey("ctrl+y"))
	m = u.(Model)
	if !strings.Contains(copied, "SELECT secret FROM vault") {
		t.Fatalf("expected copied query via ctrl+y, got %q", copied)
	}
	if !strings.Contains(m.status, "copied") {
		t.Fatalf("expected copy status, got %q", m.status)
	}
}

func TestHistoryModalClampOffsetOnFilterShrink(t *testing.T) {
	m := queryTabModel()
	m.tab = 3
	m.height = 14 // budget = 4
	m.connStr = "postgres://localhost/testdb"
	m.history = &saved.History{}
	for i := range 10 {
		m.history.Add(strings.Repeat("SELECT ", i+1), saved.Mask(m.connStr))
	}
	m.history.Add("SPECIAL QUERY", saved.Mask(m.connStr))

	u, _ := m.Update(testKey("ctrl+h"))
	m = u.(Model)

	// Move down past initial budget so offset > 0
	for range 8 {
		u, _ = m.Update(testKey("down"))
		m = u.(Model)
	}
	if m.historyOffset == 0 {
		t.Fatalf("expected historyOffset > 0, got %d", m.historyOffset)
	}

	// Now filter to a single query
	m.historyFilterInput.SetValue("SPECIAL")
	m.rebuildHistoryFiltered()
	if len(m.historyFiltered) != 1 {
		t.Fatalf("expected 1 match, got %d", len(m.historyFiltered))
	}
	if m.historyOffset > m.historyCursor {
		t.Fatalf("historyOffset (%d) must not exceed historyCursor (%d)", m.historyOffset, m.historyCursor)
	}
	if m.historyOffset != 0 {
		t.Fatalf("expected historyOffset to be clamped to 0, got %d", m.historyOffset)
	}
}

func TestQuickFilterCellStatus(t *testing.T) {
	m := queryTabModel()
	m.tab = 2
	// With empty sample, quick filter should report empty cell
	st := m.quickFilterCell()
	if st != "cannot filter empty cell" {
		t.Fatalf("expected 'cannot filter empty cell', got %q", st)
	}

	// With NULL cell selected
	m.sample = &dbpkg.Sample{
		Columns: []string{"id", "val"},
		Rows:    [][]string{{"1", "NULL"}},
	}
	m.rowTable.setData(m.sample.Columns, m.sample.Rows)
	m.rowTable.MoveColRight()
	st = m.quickFilterCell()
	if st != "cannot filter NULL cell" {
		t.Fatalf("expected 'cannot filter NULL cell', got %q", st)
	}
}
