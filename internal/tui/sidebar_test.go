package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

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
	m.list.SetItems([]list.Item{tableItem{name: "users", icon: tableIcon}, tableItem{name: "orders", icon: tableIcon}})
	m.resizeBrowse()
	m.list.SetSize(30, 20)
	return m
}

func TestSidebarClickPreviewsInDetail(t *testing.T) {
	m := browseModel(t)
	m.list.SetItems([]list.Item{tableItem{name: "users", icon: tableIcon}, tableItem{name: "orders", icon: tableIcon}})
	// First item sits at y=5 (1 header row + 4 list chrome rows).
	u, cmd := m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 2, Y: 5})
	m = u.(Model)
	if cmd == nil || m.table != "users" || !m.focusDetail || m.detailSeq != 1 {
		t.Fatalf("want users previewed seq 1, got %q seq %d focus=%v", m.table, m.detailSeq, m.focusDetail)
	}
	// Second click on another row previews it immediately: table updates
	// and the seq bumps so the stale users reply is ignored on arrival.
	// loading is still true after click 1; reset to allow click 2.
	m.loading = false
	u2, cmd2 := m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 2, Y: 6})
	m = u2.(Model)
	if cmd2 == nil || m.table != "orders" || m.detailSeq != 2 {
		t.Fatalf("want orders re-selected seq 2, got %q seq %d", m.table, m.detailSeq)
	}
}

func TestDetailClickUsesPaneOffset(t *testing.T) {
	m := browseModel(t)
	m.list.SetItems([]list.Item{tableItem{name: "users", icon: tableIcon}})
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
	m.list.SetItems([]list.Item{tableItem{name: "users", icon: tableIcon}, tableItem{name: "orders", icon: tableIcon}})
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
	m.list.SetItems([]list.Item{tableItem{name: "users", icon: tableIcon}})
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

func TestRegexFiltering(t *testing.T) {
	m := browseModel(t)
	items := []list.Item{
		tableItem{name: "users", icon: tableIcon},
		tableItem{name: "orders", icon: tableIcon},
		tableItem{name: "user_roles", icon: tableIcon},
	}
	m.list.SetItems(items)
	// Apply regex filter matching tables starting with "user"
	m.list.SetFilterState(list.Filtering)
	m.list.SetFilterText("^user")
	vis := m.list.VisibleItems()
	if len(vis) != 2 {
		t.Fatalf("want 2 items matching regex ^user, got %d", len(vis))
	}
}

func TestFilteredMouseChrome(t *testing.T) {
	m := browseModel(t)
	m.list.SetItems([]list.Item{tableItem{name: "users", icon: tableIcon}})
	m.list.SetFilterText("users")
	m.list.SetFilterState(list.FilterApplied)
	m.resizeBrowse()
	// The filter input replaces the title block, so the first item
	// stays at y = 5 in every filter state.
	idx, ok := m.listIndexAtRaw(5, screenBrowse)
	if !ok || idx != 0 {
		t.Fatalf("want index 0 at y=5 when filtered, got idx=%d ok=%v", idx, ok)
	}
}

func TestSidebarFilterTypingKeepsSidebar(t *testing.T) {
	m := browseModel(t)
	m.list.SetItems([]list.Item{tableItem{name: "cache", icon: tableIcon}, tableItem{name: "users", icon: tableIcon}})
	m.list.SetFilterState(list.Filtering)
	// Typing "c" while the filter is focused must type into the filter,
	// not trigger the sidebar "c conns" disconnect action.
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = u.(Model)
	if m.screen != screenBrowse {
		t.Fatalf("typing in filter must not leave browse, got screen %d", m.screen)
	}
	if got := m.list.FilterValue(); got != "c" {
		t.Fatalf("want filter value %q, got %q", "c", got)
	}
}

func TestDetailDimFollowsFocus(t *testing.T) {
	// Force ANSI output: without a TTY lipgloss strips all styling,
	// which would make focused and dimmed renders identical.
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	newDetail := func(focusDetail bool) Model {
		m := browseModel(t)
		m.list.SetItems([]list.Item{tableItem{name: "users", icon: tableIcon}})
		m.table = "users"
		m.focusDetail = focusDetail
		m.cols = []db.Column{{Name: "id"}, {Name: "name"}}
		m.buildTables()
		m.sizeTables()
		return m
	}
	focused := newDetail(true).detailView()
	unfocused := newDetail(false).detailView()
	if focused == unfocused {
		t.Fatal("detail view must differ between focused and sidebar-focused states")
	}
	// Focused pane keeps the bright selection highlight; the dimmed pane
	// must not contain it, but must keep the muted one.
	if !strings.Contains(focused, "48;5;62m") {
		t.Fatal("focused detail must use the bright selection background")
	}
	if strings.Contains(unfocused, "48;5;62m") {
		t.Fatal("sidebar-focused detail must not use the bright selection background")
	}
	if !strings.Contains(unfocused, "48;5;236m") {
		t.Fatal("sidebar-focused detail must use the dimmed selection background")
	}
}

func TestBrowseViewSidebarFitsWidth(t *testing.T) {
	m := browseModel(t)
	m.list.SetItems([]list.Item{tableItem{name: "cache_locks", icon: tableIcon}, tableItem{name: "migrations", icon: tableIcon}})
	v := m.View()
	for i, ln := range strings.Split(v, "\n") {
		j := strings.Index(ln, "│")
		if j < 0 {
			continue // footer / header lines span full width
		}
		left := ln[:j]
		if lipgloss.Width(left) > m.sidebarW {
			t.Fatalf("line %d sidebar width %d exceeds %d: %q", i, lipgloss.Width(left), m.sidebarW, ln)
		}
	}
}
