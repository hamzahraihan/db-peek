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
	// Bordered layout: first tree row at explorerFirstRow=5, orders idx1 → y=6.
	u, cmd := m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 2, Y: 6})
	m = u.(Model)
	if cmd == nil || m.table != "orders" {
		t.Fatalf("want orders previewed, got %q", m.table)
	}
}

func TestSidebarClickPreviewsInDetail(t *testing.T) {
	m := browseModel(t)
	// Fixture rows: 0=schema public, 1=orders, 2=col id, 3=col status,
	// 4=customers. First tree row lands at explorerFirstRow=5, so
	// orders sits at y=6 and customers at y=9.
	u, cmd := m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 2, Y: 6})
	m = u.(Model)
	if cmd == nil || m.table != "orders" || !m.focusDetail || m.detailSeq != 1 {
		t.Fatalf("want orders previewed seq 1, got %q seq %d focus=%v", m.table, m.detailSeq, m.focusDetail)
	}
	// Second click on another row previews it immediately: table updates
	// and the seq bumps so the stale orders reply is ignored on arrival.
	// loading is still true after click 1; reset to allow click 2.
	// NOTE: click 1 toggled orders collapsed, so customers slid from
	// idx 4 to idx 2 (y=7).
	m.loading = false
	u2, cmd2 := m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 2, Y: 7})
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
	// Explorer equivalent: the first tree row lands at explorerFirstRow=5
	// in every filter state (border+title+conn+separator chrome above it).
	m := browseModel(t)
	m.explorer.SetFilter("cust")
	m.resizeBrowse()
	r, ok := m.explorer.RowAt(5 - explorerFirstRow)
	if !ok || r.Kind != RowSchema {
		t.Fatalf("want schema row at y=5 when filtered, got %+v ok=%v", r, ok)
	}
	r, ok = m.explorer.RowAt(6 - explorerFirstRow)
	if !ok || r.Kind != RowTable || r.Table != "customers" {
		t.Fatalf("want customers at y=6 when filtered, got %+v ok=%v", r, ok)
	}
}

// NOTE: TestSidebarFilterTypingKeepsSidebar deleted — it covered the legacy
// list filter input focus. Its explorer successor is
// TestSidebarFilterTyping below: while filtering, keystrokes belong to the
// filter input and must not trigger sidebar actions.

func TestSidebarFilterTypingFiltersTables(t *testing.T) {
	m := browseModel(t)
	// "/" opens the filter input.
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = u.(Model)
	if !m.filtering {
		t.Fatal("pressing / must open the sidebar filter input")
	}
	// Typing filters live; single-letter keys must not trigger actions
	// (q would quit, c would disconnect, r would reload).
	for _, r := range "cust" {
		u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
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
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = u.(Model)
	if m.filtering || m.explorer.Filter != "cust" {
		t.Fatalf("enter must exit filtering keeping filter, filtering=%v filter=%q", m.filtering, m.explorer.Filter)
	}
}

func TestSidebarFilterEscClears(t *testing.T) {
	m := browseModel(t)
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = u.(Model)
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = u.(Model)
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
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
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = u.(Model)
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = u.(Model)
	if m.screen != screenBrowse {
		t.Fatalf("typing q in filter must not quit, got screen %d", m.screen)
	}
	if m.explorer.Filter != "q" {
		t.Fatalf("want filter %q, got %q", "q", m.explorer.Filter)
	}
	// "?" must type into the filter, not open the help overlay.
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = u.(Model)
	if m.showHelp {
		t.Fatal("? in filter must not open help")
	}
	if m.explorer.Filter != "q?" {
		t.Fatalf("want filter %q, got %q", "q?", m.explorer.Filter)
	}
}

func TestExplorerConnRowClickIsNoop(t *testing.T) {
	// Clicks on the conn row (y==3, shifted down by the top border) away
	// from × are no-ops.
	m := browseModel(t)
	m.loading = false
	u, cmd := m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 2, Y: 3})
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
	u, _ = m2.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: m2.sidebarW - 2, Y: 3})
	m2 = u.(Model)
	if m2.screen != screenConns {
		t.Fatalf("× click must disconnect, got screen=%d", m2.screen)
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
	u, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = u.(Model)
	if m.table != "orders" || cmd == nil {
		t.Fatalf("column enter must preview parent table, got %q cmd=%v", m.table, cmd)
	}
}

func TestColumnsErrCollapsesTable(t *testing.T) {
	m := browseModel(t)
	// orders starts expanded in the fixture; an err must collapse it.
	u, _ := m.Update(columnsLoadedMsg{schema: "public", table: "orders", err: errTestCount})
	m = u.(Model)
	if m.err == "" {
		t.Fatal("err must set m.err")
	}
	tb, ok := m.explorer.tableByName("public", "orders")
	if !ok || tb.Expanded {
		t.Fatalf("err must collapse table node, got %+v ok=%v", tb, ok)
	}
}

func TestSidebarFooterAndEmptyStates(t *testing.T) {
	m := browseModel(t)
	v := m.View()
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
	if v := m2.View(); !strings.Contains(v, "(empty)") || !strings.Contains(v, "2 schemas") {
		t.Fatalf("want (empty) + '2 schemas', got:\n%s", v)
	}
	// Zero tables overall renders (no tables).
	m3 := browseModel(t)
	m3.explorer = NewExplorer("shop", []string{"public"})
	m3.resizeBrowse()
	if v := m3.View(); !strings.Contains(v, "(no tables)") {
		t.Fatalf("want (no tables), got:\n%s", v)
	}
}

func TestDetailDimFollowsFocus(t *testing.T) {
	// Force ANSI output: without a TTY lipgloss strips all styling,
	// which would make focused and dimmed renders identical.
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
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
	v := m.View()
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
	u, _ := m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x, Y: y})
	m = u.(Model)
	if !m.focusDetail {
		t.Fatal("click inside detail must focus detail")
	}
	// Click inside sidebar box focuses sidebar.
	u, _ = m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 2, Y: 5})
	m = u.(Model)
	if m.focusDetail {
		t.Fatal("click inside sidebar must focus sidebar")
	}
	// Border clicks select nothing but must still switch the pane.
	u, _ = m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: m.paneX(), Y: 10})
	m = u.(Model)
	if !m.focusDetail {
		t.Fatal("click on detail border must focus detail")
	}
	u, _ = m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 0, Y: 10})
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
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
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
	stale := queryDoneMsg{sql: "select 1", seq: 4, sample: &db.Sample{Columns: []string{"a"}, Rows: [][]string{{"1"}}}}
	u, _ := m.Update(stale)
	m = u.(Model)
	if m.querySample != nil {
		t.Fatal("stale query reply must not apply")
	}
	fresh := queryDoneMsg{sql: "select 1", seq: 5, sample: &db.Sample{Columns: []string{"a"}, Rows: [][]string{{"1"}}}, ms: 3}
	u, _ = m.Update(fresh)
	m = u.(Model)
	if m.querySample == nil || m.queryMs != 3 {
		t.Fatal("fresh query reply must apply")
	}
}

func TestFocusedBorderGold(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	m := browseModel(t)
	m.loading = false
	m.focusDetail = true
	v := m.View()
	// #EAB308 quantizes to 178 under ANSI256 in this lipgloss version
	// (brief said 220); assert on the border corner to distinguish from
	// the gold explorer title text.
	if !strings.Contains(v, "38;5;178m╭") {
		t.Fatalf("focused pane must draw gold border:\n%s", v)
	}
}
