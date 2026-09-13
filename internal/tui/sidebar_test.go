package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"db-peek/internal/db"
	"db-peek/internal/saved"
)

var errTestCount = errors.New("count failed")

func browseModel(t *testing.T) Model {
	t.Helper()
	s := &saved.Store{Path: "./nonexistent-connections.json"}
	m := New("", s)
	m.screen = screenBrowse
	m.loading = false
	m.width, m.height = 100, 30
	m.db = &db.DB{Driver: db.SQLite, Display: "test"}
	m.explorer = fixtureExplorer()
	m.resizeBrowse()
	return m
}

func TestTableCountMsgApplies(t *testing.T) {
	m := browseModel(t)
	m.explorer = fixtureExplorer()
	u, _ := m.Update(tableCountMsg{schema: "public", table: "customers", count: 777})
	m = u.(Model)
	for _, s := range m.explorer.Schemas {
		for _, tb := range s.Tables {
			if tb.Name == "customers" && (!tb.CountOK || tb.Count != 777) {
				t.Fatalf("count not applied: %+v", tb)
			}
		}
	}
}

func TestTableCountErrLatchesNoRetry(t *testing.T) {
	m := browseModel(t)
	m.explorer = fixtureExplorer()
	// Make customers the only pending count; orders stays OK.
	for si := range m.explorer.Schemas {
		for ti := range m.explorer.Schemas[si].Tables {
			if m.explorer.Schemas[si].Tables[ti].Name == "customers" {
				m.explorer.Schemas[si].Tables[ti].Count = -1
				m.explorer.Schemas[si].Tables[ti].CountOK = false
				m.explorer.Schemas[si].Tables[ti].CountErr = false
			}
		}
	}
	u, cmd := m.Update(tableCountMsg{schema: "public", table: "customers", err: errTestCount})
	m = u.(Model)
	tb, ok := m.explorer.tableByName("public", "customers")
	if !ok || !tb.CountErr {
		t.Fatalf("err must latch CountErr: %+v ok=%v", tb, ok)
	}
	if out := m.explorer.Render(34, 20); !strings.Contains(out, "?") {
		t.Fatalf("errored count must render ?: \n%s", out)
	}
	if cmd != nil {
		t.Fatal("errored count must not re-queue a load (want nil cmd)")
	}
	// A second identical err must also dispatch no further cmd.
	u2, cmd2 := m.Update(tableCountMsg{schema: "public", table: "customers", err: errTestCount})
	_ = u2
	if cmd2 != nil {
		t.Fatal("second err must not retry either (want nil cmd)")
	}
}

func TestExplorerClickPreviews(t *testing.T) {
	m := browseModel(t)
	m.explorer = fixtureExplorer()
	m.loading = false
	u, cmd := m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 2, Y: 5})
	m = u.(Model)
	if cmd == nil || m.table != "orders" {
		t.Fatalf("want orders previewed, got %q", m.table)
	}
}

func TestSidebarClickPreviewsInDetail(t *testing.T) {
	m := browseModel(t)
	// Fixture rows: 0=schema public, 1=orders, 2=col id, 3=col status,
	// 4=customers. First tree row lands at explorerFirstRow=4, so
	// orders sits at y=5 and customers at y=8.
	u, cmd := m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 2, Y: 5})
	m = u.(Model)
	if cmd == nil || m.table != "orders" || !m.focusDetail || m.detailSeq != 1 {
		t.Fatalf("want orders previewed seq 1, got %q seq %d focus=%v", m.table, m.detailSeq, m.focusDetail)
	}
	// Second click on another row previews it immediately: table updates
	// and the seq bumps so the stale orders reply is ignored on arrival.
	// loading is still true after click 1; reset to allow click 2.
	// NOTE: click 1 toggled orders collapsed, so customers slid from
	// idx 4 to idx 2 (y=6).
	m.loading = false
	u2, cmd2 := m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 2, Y: 6})
	m = u2.(Model)
	if cmd2 == nil || m.table != "customers" || m.detailSeq != 2 {
		t.Fatalf("want customers re-selected seq 2, got %q seq %d", m.table, m.detailSeq)
	}
}

func TestDetailClickUsesPaneOffset(t *testing.T) {
	m := browseModel(t)
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
	// Explorer equivalent: substring filter keeps matching tables with
	// their parent schema row (fixture: orders+2 cols, customers).
	m := browseModel(t)
	m.explorer.SetFilter("cust")
	rows := m.explorer.VisibleRows()
	if len(rows) != 2 {
		t.Fatalf("want 2 rows (schema+customers) matching cust, got %d: %v", len(rows), rows)
	}
	if rows[1].Kind != RowTable || rows[1].Table != "customers" {
		t.Fatalf("want customers table row, got %+v", rows[1])
	}
}

func TestFilteredMouseChrome(t *testing.T) {
	// Explorer equivalent: the first tree row lands at explorerFirstRow=4
	// in every filter state (title+conn+separator chrome above it).
	m := browseModel(t)
	m.explorer.SetFilter("cust")
	m.resizeBrowse()
	r, ok := m.explorer.RowAt(4 - explorerFirstRow)
	if !ok || r.Kind != RowSchema {
		t.Fatalf("want schema row at y=4 when filtered, got %+v ok=%v", r, ok)
	}
	r, ok = m.explorer.RowAt(5 - explorerFirstRow)
	if !ok || r.Kind != RowTable || r.Table != "customers" {
		t.Fatalf("want customers at y=5 when filtered, got %+v ok=%v", r, ok)
	}
}

// NOTE: TestSidebarFilterTypingKeepsSidebar deleted — it covered the legacy
// list filter input focus (typing "c" must not trigger disconnect). No
// explorer filter input exists in this task ("/" just clears the filter,
// update.go shim retained per scope), so there is no filter-focused state;
// "c" on the sidebar now disconnects by design (sidebarKeys).

func TestExplorerConnRowClickIsNoop(t *testing.T) {
	// Clicks on the conn row (y==2) are no-ops: no preview, no disconnect.
	m := browseModel(t)
	m.loading = false
	u, cmd := m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 2, Y: 2})
	m = u.(Model)
	if cmd != nil || m.table != "" || m.screen != screenBrowse {
		t.Fatalf("conn click must be noop, got table=%q screen=%d cmd=%v", m.table, m.screen, cmd)
	}
}

func TestDetailDimFollowsFocus(t *testing.T) {
	// Force ANSI output: without a TTY lipgloss strips all styling,
	// which would make focused and dimmed renders identical.
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	newDetail := func(focusDetail bool) Model {
		m := browseModel(t)
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
