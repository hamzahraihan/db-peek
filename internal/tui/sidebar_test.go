package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"

	"db-peek/internal/db"
	"db-peek/internal/saved"
)

func browseModel(t *testing.T) Model {
	t.Helper()
	s := &saved.Store{Path: "./nonexistent-connections.json"}
	m := New("", s)
	m.screen = screenBrowse
	m.loading = false
	m.width, m.height = 100, 30
	m.db = &db.DB{Driver: db.SQLite, Display: "test"}
	m.list.SetItems([]list.Item{tableItem{"users"}, tableItem{"orders"}})
	m.resizeBrowse()
	m.list.SetSize(30, 20)
	return m
}

func TestSidebarClickPreviewsInDetail(t *testing.T) {
	m := browseModel(t)
	m.list.SetItems([]list.Item{tableItem{"users"}, tableItem{"orders"}})
	// Sidebar row 0 (first item at y=4): click starts preview load.
	u, cmd := m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 2, Y: 4})
	m = u.(Model)
	if cmd == nil || m.table != "users" || !m.focusDetail || m.detailSeq != 1 {
		t.Fatalf("want users previewed seq 1, got %q seq %d focus=%v", m.table, m.detailSeq, m.focusDetail)
	}
	// Second click on another row previews it immediately: table updates
	// and the seq bumps so the stale users reply is ignored on arrival.
	// loading is still true after click 1; reset to allow click 2.
	m.loading = false
	u2, cmd2 := m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 2, Y: 5})
	m = u2.(Model)
	if cmd2 == nil || m.table != "orders" || m.detailSeq != 2 {
		t.Fatalf("want orders re-selected seq 2, got %q seq %d", m.table, m.detailSeq)
	}
}

func TestDetailClickUsesPaneOffset(t *testing.T) {
	m := browseModel(t)
	m.list.SetItems([]list.Item{tableItem{"users"}})
	m.table = "users"
	m.focusDetail = true
	m.cols = []db.Column{{Name: "id"}, {Name: "name"}}
	m.buildTables()
	m.sizeTables()

	// Pane starts at x = sidebarW+1; clicking inside the pane selects a grid row.
	x := m.paneX() + 2
	y := m.tabStripRow() + 2 + 2 // tabs + blank + first data row
	u, _ := m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x, Y: y})
	m = u.(Model)
	if m.colTable.Cursor() != 0 {
		t.Fatalf("want cursor 0, got %d", m.colTable.Cursor())
	}
}

func TestSeqGuardDropsStaleReply(t *testing.T) {
	m := browseModel(t)
	m.list.SetItems([]list.Item{tableItem{"users"}, tableItem{"orders"}})
	m.table = "orders"
	m.detailSeq = 5
	stale := detailLoadedMsg{table: "users", seq: 4, cols: []db.Column{{Name: "a"}}}
	m.cols = []db.Column{{Name: "z"}}
	u, _ := m.Update(stale)
	m = u.(Model)
	if len(m.cols) != 1 || m.cols[0].Name != "z" {
		t.Fatal("stale reply must not overwrite state")
	}
	fresh := detailLoadedMsg{table: "orders", seq: 5, cols: []db.Column{{Name: "b"}}}
	u, _ = m.Update(fresh)
	m = u.(Model)
	if len(m.cols) != 1 || m.cols[0].Name != "b" {
		t.Fatal("fresh reply must apply")
	}
}

func TestBrowseViewFitsTerminal(t *testing.T) {
	m := browseModel(t)
	m.list.SetItems([]list.Item{tableItem{"users"}})
	m.table = "users"
	m.cols = []db.Column{{Name: "id"}}
	m.buildTables()
	m.sizeTables()
	v := m.View()
	for i, ln := range strings.Split(v, "\n") {
		if lipgloss.Width(ln) > m.width {
			t.Fatalf("line %d wraps at width %d: %q", i, m.width, ln)
		}
	}
}
