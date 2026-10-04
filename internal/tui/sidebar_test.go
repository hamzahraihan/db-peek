package tui

import (
	"errors"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"db-peek/internal/db"
	"db-peek/internal/saved"
)

var errTestCount = errors.New("count failed")

func browseModel(t *testing.T) Model {
	t.Helper()
	s := &saved.Store{Path: "./nonexistent-connections.json"}
	m := New("", s)
	m.history = &saved.History{} // never read the developer's real history
	m.screen = screenBrowse
	m.loading = false
	m.width, m.height = 100, 30
	m.db = &db.DB{Driver: db.SQLite, Display: "test"}
	m.explorer = fixtureExplorer()
	m.resizeBrowse()
	return m
}

func TestCountsLoadedMsgApplies(t *testing.T) {
	m := browseModel(t)
	m.explorer = fixtureExplorer()
	u, _ := m.Update(countsLoadedMsg{
		schema: "public",
		counts: map[string]int64{"public.customers": 777, "public.orders": 12},
		errs:   map[string]bool{},
	})
	m = u.(Model)
	for _, s := range m.explorer.Schemas {
		for _, tb := range s.Tables {
			switch tb.Name {
			case "customers", "orders":
				if !tb.CountOK || tb.CountErr {
					t.Fatalf("count not applied: %+v", tb)
				}
			}
			if m.counts[db.QualTable{Schema: s.Name, Name: tb.Name}.String()] != tb.Count {
				t.Fatalf("m.counts must mirror the sidebar: %+v vs %d", tb, m.counts[tb.Name])
			}
		}
	}
}

func TestCountsLoadedMsgLatchesErr(t *testing.T) {
	m := browseModel(t)
	m.explorer = fixtureExplorer()
	u, cmd := m.Update(countsLoadedMsg{
		schema: "public",
		counts: map[string]int64{"public.orders": 3},
		errs:   map[string]bool{"public.customers": true},
	})
	m = u.(Model)
	tb, ok := m.explorer.tableByName("public", "customers")
	if !ok || !tb.CountErr {
		t.Fatalf("err must latch CountErr: %+v ok=%v", tb, ok)
	}
	if _, cached := m.counts["public.customers"]; cached {
		t.Fatal("a failed count must not enter the cache")
	}
	if out := m.explorer.Render(34, 20); !strings.Contains(out, "?") {
		t.Fatalf("errored count must render ?: \n%s", out)
	}
	if cmd != nil {
		t.Fatal("the batch handler issues no follow-up command (want nil)")
	}
}

func TestCountsLoadedMsgStaleConnDropped(t *testing.T) {
	m := browseModel(t)
	m.explorer = fixtureExplorer()
	m.connSeq = 3
	u, _ := m.Update(countsLoadedMsg{
		schema: "public",
		counts: map[string]int64{"customers": 777},
		errs:   map[string]bool{},
		conn:   2,
	})
	m = u.(Model)
	if tb, _ := m.explorer.tableByName("public", "customers"); tb.Count == 777 {
		t.Fatalf("stale-conn counts must be dropped: %+v", tb)
	}
}

// The exact count arrives after the grid, so a stale or mismatched reply
// must leave m.count at the unknown marker instead of lying about the size.
func TestDetailCountMsgAppliesOnlyForOpenTable(t *testing.T) {
	m := browseModel(t)
	m.table = "orders"
	m.detailSeq = 4
	m.count = -1

	u, _ := m.Update(detailCountMsg{table: "orders", count: 5, seq: 3, conn: m.connSeq})
	if m2 := u.(Model); m2.count != -1 {
		t.Fatalf("stale seq must not apply, got %d", m2.count)
	}
	u, _ = m.Update(detailCountMsg{table: "customers", count: 5, seq: 4, conn: m.connSeq})
	if m2 := u.(Model); m2.count != -1 {
		t.Fatalf("a different table must not apply, got %d", m2.count)
	}
	u, _ = m.Update(detailCountMsg{table: "orders", count: 5, seq: 4, conn: m.connSeq + 1})
	if m2 := u.(Model); m2.count != -1 {
		t.Fatalf("a stale connection must not apply, got %d", m2.count)
	}
	u, _ = m.Update(detailCountMsg{table: "orders", count: 5, seq: 4, conn: m.connSeq})
	if m2 := u.(Model); m2.count != 5 {
		t.Fatalf("matching reply must apply, got %d", m2.count)
	}
}

// A table whose count the sidebar already fetched opens with no query at
// all: the command must answer from m.counts even on a closed database.
func TestLoadCountUsesSidebarCache(t *testing.T) {
	m := browseModel(t)
	m.db = openMemoryDB(t)
	m.db.Close()
	m.counts = map[string]int64{"orders": 42}
	msg, ok := m.loadCount(t.Context(), "", "orders")().(detailCountMsg)
	if !ok {
		t.Fatalf("want a detailCountMsg, got %T", msg)
	}
	if msg.count != 42 || msg.err != nil {
		t.Fatalf("cached count must answer without a query: %+v", msg)
	}
}

func TestExplorerClickPreviews(t *testing.T) {
	m := browseModel(t)
	m.explorer = fixtureExplorer()
	m.loading = false
	// Bordered layout: orders is the first tree row, one below
	// explorerFirstRow.
	u, cmd := m.Update(testClick(2, explorerFirstRow+1))
	m = u.(Model)
	if cmd == nil || m.table != "orders" {
		t.Fatalf("want orders previewed, got %q", m.table)
	}
}

func TestSidebarClickPreviewsInDetail(t *testing.T) {
	m := browseModel(t)
	// Fixture rows: 0=schema public, 1=orders, 2=col id, 3=col status,
	// 4=customers. The tree starts on explorerFirstRow, so orders sits
	// one row below it and customers three rows below.
	u, cmd := m.Update(testClick(2, explorerFirstRow+1))
	m = u.(Model)
	if cmd == nil || m.table != "orders" || m.focusDetail || m.detailSeq != 1 {
		t.Fatalf("want orders previewed seq 1 staying in sidebar, got %q seq %d focus=%v", m.table, m.detailSeq, m.focusDetail)
	}
	// Second click on another row previews it immediately: table updates
	// and the seq bumps so the stale orders reply is ignored on arrival.
	// loading is still true after click 1; reset to allow click 2.
	// NOTE: click 1 toggled orders collapsed, so customers slid from
	// idx 4 to idx 2.
	m.loading = false
	u2, cmd2 := m.Update(testClick(2, explorerFirstRow+2))
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
	u, _ := m.Update(testClick(x, y))
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
	v := m.View().Content
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
	// Explorer equivalent: the first tree row lands on
	// explorerFirstRow in every filter state (conn + framed field above).
	m := browseModel(t)
	m.explorer.SetFilter("cust")
	m.resizeBrowse()
	r, ok := m.explorer.RowAt(0)
	if !ok || r.Kind != RowSchema {
		t.Fatalf("want schema row at the first tree row when filtered, got %+v ok=%v", r, ok)
	}
	r, ok = m.explorer.RowAt(1)
	if !ok || r.Kind != RowTable || r.Table != "customers" {
		t.Fatalf("want customers in the second tree row, got %+v ok=%v", r, ok)
	}
}

// NOTE: TestSidebarFilterTypingKeepsSidebar deleted — it covered the legacy
// list filter input focus. Its explorer successor is
// TestSidebarFilterTyping below: while filtering, keystrokes belong to the
// filter input and must not trigger sidebar actions.

func TestSidebarFilterTypingFiltersTables(t *testing.T) {
	m := browseModel(t)
	// "/" opens the filter input.
	u, _ := m.Update(testKey("/"))
	m = u.(Model)
	if !m.filtering {
		t.Fatal("pressing / must open the sidebar filter input")
	}
	// Typing filters live; single-letter keys must not trigger actions
	// (q would quit, c would disconnect, r would reload).
	for _, r := range "cust" {
		u, _ = m.Update(testKey(string(r)))
		m = u.(Model)
	}
	if m.screen != screenBrowse {
		t.Fatalf("typing in filter must not leave browse, got screen %d", m.screen)
	}
	if got := m.explorer.Filter; got != "cust" {
		t.Fatalf("want filter %q, got %q", "cust", got)
	}
	if n := len(m.explorer.VisibleRows()); n != 2 {
		t.Fatalf("want 2 visible rows when filtered, got %d", n)
	}
	// enter applies the filter and exits, keeping the text.
	u, _ = m.Update(testKey("enter"))
	m = u.(Model)
	if m.filtering || m.explorer.Filter != "cust" {
		t.Fatalf("enter must exit filtering keeping filter, filtering=%v filter=%q", m.filtering, m.explorer.Filter)
	}
}

func TestSidebarFilterEscClears(t *testing.T) {
	m := browseModel(t)
	u, _ := m.Update(testKey("/"))
	m = u.(Model)
	u, _ = m.Update(testKey("x"))
	m = u.(Model)
	u, _ = m.Update(testKey("esc"))
	m = u.(Model)
	if m.filtering || m.explorer.Filter != "" {
		t.Fatalf("esc must exit and clear filter, filtering=%v filter=%q", m.filtering, m.explorer.Filter)
	}
	if m.screen != screenBrowse {
		t.Fatalf("esc in filter must not disconnect, got screen %d", m.screen)
	}
}

func TestSidebarFilterQDoesNotQuit(t *testing.T) {
	m := browseModel(t)
	u, _ := m.Update(testKey("/"))
	m = u.(Model)
	u, _ = m.Update(testKey("q"))
	m = u.(Model)
	if m.screen != screenBrowse {
		t.Fatalf("typing q in filter must not quit, got screen %d", m.screen)
	}
	if m.explorer.Filter != "q" {
		t.Fatalf("want filter %q, got %q", "q", m.explorer.Filter)
	}
	// "?" must type into the filter, not open the help overlay.
	u, _ = m.Update(testKey("?"))
	m = u.(Model)
	if m.showHelp {
		t.Fatal("? in filter must not open help")
	}
	if m.explorer.Filter != "q?" {
		t.Fatalf("want filter %q, got %q", "q?", m.explorer.Filter)
	}
}

func TestExplorerConnRowClickIsNoop(t *testing.T) {
	// Clicks on the conn row away from × are no-ops.
	m := browseModel(t)
	m.loading = false
	u, cmd := m.Update(testClick(2, sideConnRow))
	m = u.(Model)
	if cmd != nil || m.table != "" || m.screen != screenBrowse {
		t.Fatalf("conn click must be noop, got table=%q screen=%d cmd=%v", m.table, m.screen, cmd)
	}
	// Clicks with x >= sidebarW-2 on the conn row hit × and disconnect
	// (× sits at interior x == sidebarW-2; x == sidebarW-1 is the border).
	// (db=nil: fixture DB has no live SQL handle; disconnect's screen
	// transition is what this asserts — Close is exercised in prod.)
	m2 := browseModel(t)
	m2.loading = false
	m2.db = nil
	u, _ = m2.Update(testClick(m2.sidebarW-2, sideConnRow))
	m2 = u.(Model)
	if m2.screen != screenConns {
		t.Fatalf("× click must disconnect, got screen=%d", m2.screen)
	}
}

// The search field is always on screen, so a click on it must behave
// like "/": focus the input, and its × clears the applied filter.
func TestSidebarSearchRowClickFocusesAndClears(t *testing.T) {
	m := browseModel(t)
	m.loading = false
	u, _ := m.Update(testClick(4, sideSearchRow))
	m = u.(Model)
	if !m.filtering || !m.filterInput.Focused() {
		t.Fatalf("clicking the search row must focus the field, filtering=%v", m.filtering)
	}
	for _, r := range "cust" {
		u, _ = m.Update(testKey(string(r)))
		m = u.(Model)
	}
	u, _ = m.Update(testKey("enter")) // commit: filter stays applied, blurred
	m = u.(Model)
	if m.explorer.Filter != "cust" {
		t.Fatalf("setup: want applied filter, got %q", m.explorer.Filter)
	}
	// × sits in the frame's last content cell, inside the field box.
	u, _ = m.Update(testClick(m.searchClearX(), sideSearchRow))
	m = u.(Model)
	if m.filtering || m.explorer.Filter != "" || m.filterInput.Value() != "" {
		t.Fatalf("× must clear the filter, filtering=%v filter=%q input=%q",
			m.filtering, m.explorer.Filter, m.filterInput.Value())
	}
	if n := len(m.explorer.VisibleRows()); n != 5 {
		t.Fatalf("cleared filter must restore every row, got %d", n)
	}
}

// The field is framed, always on screen, and the tree keeps starting on
// explorerFirstRow whether or not the field has focus.
func TestSidebarSearchFieldFramedAndAlwaysVisible(t *testing.T) {
	m := browseModel(t)
	lines := func() []string {
		out := strings.Split(m.viewString(), "\n")
		for i, ln := range out {
			out[i] = ansi.Strip(ln)
		}
		return out
	}
	for _, filtering := range []bool{false, true} {
		m.filtering = filtering
		l := lines()
		if !strings.Contains(l[sideSearchRow], "filter tables") {
			t.Fatalf("filtering=%v: field row must show the input:\n%s", filtering, l[sideSearchRow])
		}
		// The frame wraps the field: box corners on the rules, verticals
		// on the content row, and the rules carry the pane's inner width.
		for _, want := range []string{"╭", "╮", "╰", "╯"} {
			if !strings.Contains(l[sideSearchTop], want) && !strings.Contains(l[sideSearchRow+1], want) {
				t.Fatalf("filtering=%v: field must be framed, missing %q:\n%s|%s",
					filtering, want, l[sideSearchTop], l[sideSearchRow+1])
			}
		}
		if w := lipgloss.Width(ansi.Cut(l[sideSearchTop], 1, m.sidebarW-1)); w != m.sidebarW-2 {
			t.Fatalf("field frame width %d must match the sidebar interior %d", w, m.sidebarW-2)
		}
		if !strings.Contains(l[explorerFirstRow], "public") {
			t.Fatalf("filtering=%v: tree must start on row %d:\n%s", filtering, explorerFirstRow, l[explorerFirstRow])
		}
	}
	// A committed filter stays readable on the field, with its ×.
	m.filtering = false
	u, _ := m.Update(testKey("/"))
	m = u.(Model)
	for _, r := range "cust" {
		u, _ = m.Update(testKey(string(r)))
		m = u.(Model)
	}
	u, _ = m.Update(testKey("enter"))
	m = u.(Model)
	if row := lines()[sideSearchRow]; !strings.Contains(row, "cust") || !strings.Contains(row, "×") {
		t.Fatalf("committed filter and its × must stay on screen: %q", row)
	}
}

func TestColumnEnterPreviewsParent(t *testing.T) {
	m := browseModel(t)
	m.loading = false
	// Fixture rows: 0=schema, 1=orders, 2=col id → cursor on column row.
	m.explorer.Cursor = 2
	if r, ok := m.explorer.RowAt(m.explorer.Cursor); !ok || r.Kind != RowColumn {
		t.Fatalf("fixture setup: want column row at cursor 2, got %+v ok=%v", r, ok)
	}
	u, cmd := m.Update(testKey("enter"))
	m = u.(Model)
	if m.table != "orders" || cmd == nil {
		t.Fatalf("column enter must preview parent table, got %q cmd=%v", m.table, cmd)
	}
}

func TestColumnsErrCollapsesTable(t *testing.T) {
	m := browseModel(t)
	// orders starts expanded in the fixture; a column failure must collapse it.
	u, _ := m.Update(detailLoadedMsg{
		table:   "orders",
		schema:  "public",
		err:     errTestCount,
		colsErr: errTestCount,
	})
	m = u.(Model)
	if m.err == "" {
		t.Fatal("err must set m.err")
	}
	tb, ok := m.explorer.tableByName("public", "orders")
	if !ok || tb.Expanded {
		t.Fatalf("column failure must collapse table node, got %+v ok=%v", tb, ok)
	}
}

// Only a column failure collapses the node: a rows/indexes failure must
// leave the tree alone, or an unrelated error silently drops the schema.
func TestNonColumnErrKeepsTableExpanded(t *testing.T) {
	m := browseModel(t)
	u, _ := m.Update(detailLoadedMsg{table: "orders", schema: "public", err: errTestCount})
	m = u.(Model)
	if m.err == "" {
		t.Fatal("err must set m.err")
	}
	tb, ok := m.explorer.tableByName("public", "orders")
	if !ok || !tb.Expanded {
		t.Fatalf("a non-column failure must not collapse the node, got %+v ok=%v", tb, ok)
	}
}

func TestSidebarFooterAndEmptyStates(t *testing.T) {
	m := browseModel(t)
	v := m.View().Content
	if !strings.Contains(v, "1 schema") {
		t.Fatalf("want gold footer '1 schema', got:\n%s", v)
	}
	// Empty schema renders a dim (empty) line after tree rows.
	m2 := browseModel(t)
	m2.explorer = NewExplorer("shop", []string{"public", "empty_s"})
	m2.explorer.Schemas[0].Expanded = true
	m2.explorer.Schemas[0].Tables = []TableNode{{Schema: "public", Name: "t", CountOK: true}}
	m2.explorer.Schemas[1].Expanded = true
	m2.resizeBrowse()
	if v := m2.View().Content; !strings.Contains(v, "(empty)") || !strings.Contains(v, "2 schemas") {
		t.Fatalf("want (empty) + '2 schemas', got:\n%s", v)
	}
	// Zero tables overall renders (no tables).
	m3 := browseModel(t)
	m3.explorer = NewExplorer("shop", []string{"public"})
	m3.resizeBrowse()
	if v := m3.View().Content; !strings.Contains(v, "(no tables)") {
		t.Fatalf("want (no tables), got:\n%s", v)
	}
}

func TestDetailDimFollowsFocus(t *testing.T) {
	// lipgloss v2 always renders full styling (no TTY-dependent
	// stripping), so focused and dimmed renders differ unconditionally.
	// The asserted backgrounds are indexed colors, unaffected by
	// downsampling.
	newDetail := func(focusDetail bool) Model {
		m := browseModel(t)
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
	v := m.View().Content
	lines := strings.Split(v, "\n")
	if !strings.Contains(v, "╭") {
		t.Fatalf("bordered layout must draw box corners:\n%s", v)
	}
	for i, ln := range lines {
		if lipgloss.Width(ln) > m.width {
			t.Fatalf("line %d wraps at width %d: %q", i, m.width, ln)
		}
	}
	// Sidebar box top border (line y=1) outer width must equal sidebarW.
	if len(lines) > 1 && lipgloss.Width(lines[1]) >= len("╭") {
		leftBox := strings.SplitN(lines[1], " ", 2)[0]
		// leftBox is the sidebar top border up to the gap; strip ANSI.
		if w := lipgloss.Width(leftBox); w != m.sidebarW {
			t.Fatalf("sidebar box outer width %d != sidebarW %d: %q", w, m.sidebarW, lines[1])
		}
	}
}

func TestPaneClickSwitchesFocus(t *testing.T) {
	m := browseModel(t)
	m.loading = false
	m.focusDetail = false
	m.table = "users"
	m.cols = []db.Column{{Name: "id"}}
	m.buildTables()
	m.sizeTables()
	// Click inside detail box grid area focuses detail.
	x := m.paneX() + 2
	y := 6 + 2 + 2 // detailTableTop + header(2) + first data row
	u, _ := m.Update(testClick(x, y))
	m = u.(Model)
	if !m.focusDetail {
		t.Fatal("click inside detail must focus detail")
	}
	// Click inside sidebar box focuses sidebar.
	u, _ = m.Update(testClick(2, 5))
	m = u.(Model)
	if m.focusDetail {
		t.Fatal("click inside sidebar must focus sidebar")
	}
	// Border clicks select nothing but must still switch the pane.
	u, _ = m.Update(testClick(m.paneX(), 10))
	m = u.(Model)
	if !m.focusDetail {
		t.Fatal("click on detail border must focus detail")
	}
	u, _ = m.Update(testClick(0, 10))
	m = u.(Model)
	if m.focusDetail {
		t.Fatal("click on sidebar border must focus sidebar")
	}
}

func TestDetailTabLabelsNarrowPane(t *testing.T) {
	m := browseModel(t) // width 100 → paneInnerW 62, full strip wider
	if got := m.detailTabLabels(); len(got) != 5 || got[0] != "1" || got[4] != "5" {
		t.Fatalf("narrow pane must use short labels, got %q", got)
	}
	m.width = 250 // paneInnerW 212 fits the full strip
	if got := m.detailTabLabels(); got[0] != "1 schema" || got[3] != "4 query" {
		t.Fatalf("wide pane must use full labels, got %q", got)
	}
	m.width = 0 // unknown width still yields full labels
	if got := m.detailTabLabels(); got[0] != "1 schema" {
		t.Fatalf("unknown width must use full labels, got %q", got)
	}
}

func TestQueryEditorQuestionMarkInserts(t *testing.T) {
	m := browseModel(t)
	m.screen = screenBrowse
	m.focusDetail = true
	m.tab = 3
	m.queryFocus = 0
	u, _ := m.Update(testKey("?"))
	m = u.(Model)
	if !strings.Contains(m.editor.Text(), "?") {
		t.Fatalf("typing ? in editor must insert text, got %q", m.editor.Text())
	}
}
func TestQueryRunSeqGuard(t *testing.T) {
	m := browseModel(t)
	m.table = "users"
	m.tab = 3
	m.querySeq = 5
	stale := queryDoneMsg{sql: "select 1", seq: 4, sample: &db.Sample{Columns: []string{"a"}, Rows: [][]string{{"1"}}}, affected: -1}
	u, _ := m.Update(stale)
	m = u.(Model)
	if m.querySample != nil {
		t.Fatal("stale query reply must not apply")
	}
	fresh := queryDoneMsg{sql: "select 1", seq: 5, sample: &db.Sample{Columns: []string{"a"}, Rows: [][]string{{"1"}}}, affected: -1, ms: 3}
	u, _ = m.Update(fresh)
	m = u.(Model)
	if m.querySample == nil || m.queryMs != 3 {
		t.Fatal("fresh query reply must apply")
	}
}

func TestQueryWriteShowsAffected(t *testing.T) {
	m := browseModel(t)
	m.table = "users"
	m.tab = 3
	m.focusDetail = true
	m.queryFocus = 1
	m.querySeq = 6
	m.width, m.height = 120, 40
	m.resizeBrowse()
	u, _ := m.Update(queryDoneMsg{sql: "insert", seq: 6, affected: 2, ms: 4})
	m = u.(Model)
	if m.querySample != nil || m.queryAffected != 2 {
		t.Fatalf("write reply must clear grid and store count, got %+v", m.querySample)
	}
	if v := m.View().Content; !strings.Contains(v, "2 rows affected") {
		t.Fatalf("view must show rows affected, got:\n%s", v)
	}
}

func TestQueryWritePreviewApplies(t *testing.T) {
	m := browseModel(t)
	m.table = "users"
	m.tab = 3
	m.focusDetail = true
	m.queryFocus = 1
	m.querySeq = 7
	m.width, m.height = 120, 40
	m.resizeBrowse()
	prev := &db.Sample{Columns: []string{"id"}, Rows: [][]string{{"1"}}}
	u, _ := m.Update(queryDoneMsg{sql: "insert into users values (1)", seq: 7, sample: prev, affected: 1, previewTable: "users", ms: 5})
	m = u.(Model)
	if m.querySample == nil || m.queryAffected != 1 {
		t.Fatalf("preview reply must store grid and count, got %+v affected=%d", m.querySample, m.queryAffected)
	}
	if m.queryPreviewTable != "users" {
		t.Fatalf("preview reply must store preview table, got %q", m.queryPreviewTable)
	}
	if len(m.queryTable.cols) != 1 || m.queryTable.cols[0] != "id" {
		t.Fatalf("preview reply must build grid columns, got %+v", m.queryTable.cols)
	}
}

func TestQueryWritePreviewStaleDropped(t *testing.T) {
	m := browseModel(t)
	m.tab = 3
	m.querySeq = 7
	prev := &db.Sample{Columns: []string{"id"}, Rows: [][]string{{"1"}}}
	u, _ := m.Update(queryDoneMsg{sql: "insert", seq: 6, sample: prev, affected: 1, previewTable: "users"})
	m = u.(Model)
	if m.querySample != nil {
		t.Fatal("stale preview reply must not apply")
	}
}

func TestFocusedBorderGold(t *testing.T) {
	m := browseModel(t)
	m.loading = false
	m.focusDetail = true
	// #EAB308 is hex: downsample full-fidelity v2 output to ANSI256
	// before asserting on the quantized code.
	v := render256(m.View().Content)
	// #EAB308 quantizes to 178 under ANSI256 in this lipgloss version
	// (brief said 220); assert on the border corner to distinguish from
	// the gold explorer title text.
	if !strings.Contains(v, "38;5;178m╭") {
		t.Fatalf("focused pane must draw gold border:\n%s", v)
	}
}

func TestQueryPreviewFooterNeedsTable(t *testing.T) {
	// Pins the singular/plural preview footer. Fails until view_detail.go
	// renders queryPreviewTable.
	m := browseModel(t)
	m.table = "users"
	m.tab = 3
	m.focusDetail = true
	m.queryFocus = 1
	m.querySeq = 8
	m.width, m.height = 120, 40
	m.resizeBrowse()
	prev := &db.Sample{Columns: []string{"id"}, Rows: [][]string{{"1"}, {"2"}}}
	u, _ := m.Update(queryDoneMsg{sql: "insert", seq: 8, sample: prev, affected: 2, previewTable: "users", ms: 6})
	m = u.(Model)
	if v := m.View().Content; !strings.Contains(v, "2 rows affected • preview of users") {
		t.Fatalf("plural preview footer missing, got:\n%s", v)
	}
}
