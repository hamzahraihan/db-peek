package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	dbpkg "db-peek/internal/db"
)

func focusFixture() ([]erTable, []dbpkg.ForeignKey) {
	tables := []erTable{
		{name: "customers", cols: []dbpkg.Column{{Name: "id", Extra: "PK(1)"}, {Name: "name"}}, pk: map[string]bool{"id": true}},
		{name: "orders", cols: []dbpkg.Column{{Name: "id", Extra: "PK(1)"}, {Name: "customer_id"}, {Name: "status"}}, pk: map[string]bool{"id": true}, fk: map[string]bool{"customer_id": true}},
		{name: "order_items", cols: []dbpkg.Column{{Name: "id", Extra: "PK(1)"}, {Name: "order_id"}}, pk: map[string]bool{"id": true}, fk: map[string]bool{"order_id": true}},
		{name: "unrelated", cols: []dbpkg.Column{{Name: "id", Extra: "PK(1)"}}, pk: map[string]bool{"id": true}},
	}
	links := []dbpkg.ForeignKey{
		{FromTable: "orders", FromColumn: "customer_id", ToTable: "customers", ToColumn: "id"},
		{FromTable: "order_items", FromColumn: "order_id", ToTable: "orders", ToColumn: "id"},
	}
	return tables, links
}

func TestERFocusedSubset(t *testing.T) {
	tables, links := focusFixture()
	ft, fl := erFocusedSubset("orders", tables, links)
	if len(ft) != 3 {
		t.Fatalf("want 3 focused tables, got %v", ft)
	}
	for _, ftb := range ft {
		if ftb.name == "unrelated" {
			t.Fatal("unrelated table must be excluded")
		}
	}
	if len(fl) != 2 {
		t.Fatalf("want 2 focused links, got %v", fl)
	}
}

func TestERFocusLayoutColumns(t *testing.T) {
	tables, links := focusFixture()
	// A pane as wide as the layout leaves nothing to center.
	pos := erFocusLayout("orders", tables, links, 400, 400)
	// Incoming (order_items) left of center (orders) left of outgoing (customers).
	if pos["order_items"].x >= pos["orders"].x {
		t.Fatalf("incoming must be left of center: %+v", pos)
	}
	if pos["orders"].x >= pos["customers"].x {
		t.Fatalf("center must be left of outgoing: %+v", pos)
	}
	// Narrower pane: the whole layout shifts in, keeping the column order.
	pos = erFocusLayout("orders", tables, links, 80, 24)
	if pos["order_items"].x >= pos["orders"].x || pos["orders"].x >= pos["customers"].x {
		t.Fatalf("centering must preserve column order: %+v", pos)
	}
	if pos["orders"].x <= 0 || pos["orders"].y <= 0 {
		t.Fatalf("a layout that fits must be pushed off the corner: %+v", pos["orders"])
	}
}

func TestERFocusBoxesPKFK(t *testing.T) {
	tables, _ := focusFixture()
	joined := strings.Join(erFocusBoxLinesEx(tables[1], 3, false, false), "\n")
	if !strings.Contains(joined, "PK") || !strings.Contains(joined, "FK") {
		t.Fatalf("focused box must show PK/FK text labels:\n%s", joined)
	}
	if strings.Contains(joined, "🔑") || strings.Contains(joined, "➤") {
		t.Fatalf("focused box must not use icon markers:\n%s", joined)
	}
	if !strings.Contains(joined, "╭") || !strings.Contains(joined, "─") {
		t.Fatalf("focused box must keep bordered style:\n%s", joined)
	}
}

func TestERFocusedViewExcludesUnrelated(t *testing.T) {
	m := browseModel(t)
	tables, links := focusFixture()
	m.setERSchema(erSchemaState{loaded: true, tables: tables, links: links})
	m.erFocus = true
	m.erCenter = "orders"
	m.erSel = "orders"
	m.table = "orders"
	m.width = 120
	m.sidebarW = 34
	out := m.erView(80, 20)
	for _, want := range []string{"diagram · orders", "orders", "customers", "order_items", "PK", "FK"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in focused view:\n%s", want, out)
		}
	}
	if strings.Contains(out, "unrelated") {
		t.Fatalf("focused view must exclude unrelated tables:\n%s", out)
	}
	if !strings.Contains(out, "─") {
		t.Fatalf("focused view must draw connectors:\n%s", out)
	}
}

func TestERFocusToggle(t *testing.T) {
	m := browseModel(t)
	tables, links := focusFixture()
	m.setERSchema(erSchemaState{loaded: true, tables: tables, links: links})
	m.erFocus = true
	m.erCenter = "orders"
	m.erSel = "orders"
	m.table = "orders"
	m.tab = 4
	m.focusDetail = true
	if out := m.erView(80, 20); strings.Contains(out, "unrelat") {
		t.Fatalf("focused default must hide unrelated:\n%s", out)
	}
	nm, _ := m.erKeys(tea.KeyPressMsg{}, "f")
	m = nm.(Model)
	if m.erFocus {
		t.Fatal("f must toggle to full schema")
	}
	if out := m.erView(140, 20); !strings.Contains(out, "unrelated") {
		t.Fatalf("full view must show unrelated:\n%s", out)
	}
	nm, _ = m.erKeys(tea.KeyPressMsg{}, "f")
	m = nm.(Model)
	if !m.erFocus {
		t.Fatal("second f must return to focused")
	}
}

func TestERRecenterStaysOnER(t *testing.T) {
	m := browseModel(t)
	tables, links := focusFixture()
	m.setERSchema(erSchemaState{loaded: true, tables: tables, links: links})
	m.erFocus = true
	m.erCenter = "orders"
	m.erSel = "customers"
	m.table = "orders"
	m.tab = 4
	m.focusDetail = true
	nm, _ := m.erKeys(tea.KeyPressMsg{}, "enter")
	got := nm.(Model)
	if got.tab != 4 {
		t.Fatalf("enter must stay on ER tab, got %d", got.tab)
	}
	if got.erCenter != "customers" || got.erSel != "customers" {
		t.Fatalf("enter must recenter to selection: %+v", got)
	}
	if !got.erFocus {
		t.Fatal("enter must keep focused mode")
	}
	if got.erPanX != 0 || got.erPanY != 0 {
		t.Fatal("recenter must reset pan")
	}
	out := got.erView(80, 20)
	if !strings.Contains(out, "diagram · customers") {
		t.Fatalf("view must follow new center:\n%s", out)
	}
	if strings.Contains(out, "order_items") {
		t.Fatalf("order_items is 2 hops from customers and must drop out:\n%s", out)
	}
}

func TestERFocusedHit(t *testing.T) {
	m := browseModel(t)
	tables, links := focusFixture()
	m.setERSchema(erSchemaState{loaded: true, tables: tables, links: links})
	m.erFocus = true
	m.erCenter = "orders"
	m.erSel = "orders"
	m.table = "orders"
	m.width = 120
	m.sidebarW = 34
	// Header row never hits.
	if _, ok := m.erHit(1, detailTableTop); ok {
		t.Fatal("header row must miss")
	}
	// First canvas row below header should hit some focused box.
	hit := ""
	for x := 0; x < 60 && hit == ""; x++ {
		for y := detailTableTop + 1; y < detailTableTop+12 && hit == ""; y++ {
			if name, ok := m.erHit(x, y); ok {
				hit = name
			}
		}
	}
	if hit == "" {
		t.Fatal("expected a box hit below the header")
	}
	if hit == "unrelated" {
		t.Fatal("unrelated must never hit in focused mode")
	}
}

func TestERNextBoxUsesVisible(t *testing.T) {
	m := browseModel(t)
	tables, links := focusFixture()
	m.setERSchema(erSchemaState{loaded: true, tables: tables, links: links})
	m.erFocus = true
	m.erCenter = "orders"
	m.erSel = "orders"
	next := erNextBox(m.erVisibleTables(), "orders", 1)
	if next == "unrelated" || next == "" || next == "orders" {
		t.Fatalf("n must cycle within focused subset, got %q", next)
	}
}

// A stacked link has no facing edges: it must leave and re-enter on the
// same side. When that side is walled off by another box the path has to
// take the other one — routing behind a box made the line reappear on
// the far side, so it read as if it terminated inside that box.
func TestERStackedConnectorRoutesAroundBoxes(t *testing.T) {
	pos := map[string]erRect{
		"a": {x: 10, y: 0, w: 8, h: 4}, // source, upper box
		"b": {x: 10, y: 8, w: 8, h: 4}, // target, same column
		"c": {x: 18, y: 2, w: 8, h: 8}, // blocks the right-hand channel
	}
	grid := map[[2]int]rune{}
	set := func(x, y int, r rune) { grid[[2]int{x, y}] = r }
	erConnector(set, erBlocks(pos, nil), erOccupied{}, pos["a"], pos["b"], 2, 10, 0)

	for p, r := range grid {
		if r == ' ' {
			continue
		}
		c := pos["c"]
		if p[0] >= c.x && p[0] < c.x+c.w && p[1] >= c.y && p[1] < c.y+c.h {
			t.Fatalf("connector cell %q at %v runs through the blocking box", r, p)
		}
		if p[0] >= pos["a"].x && p[0] < pos["a"].x+pos["a"].w && p[1] >= pos["a"].y && p[1] < pos["a"].y+pos["a"].h {
			t.Fatalf("connector cell %q at %v runs through its own source", r, p)
		}
		if p[0] >= pos["b"].x && p[0] < pos["b"].x+pos["b"].w && p[1] >= pos["b"].y && p[1] < pos["b"].y+pos["b"].h {
			t.Fatalf("connector cell %q at %v runs through its own target", r, p)
		}
	}
	// Entered from the left on the target's border, with an arrow head.
	if r := grid[[2]int{pos["b"].x - 1, 10}]; r != '▶' {
		t.Fatalf("stacked link must meet the target's left edge with %q, got %q", '▶', r)
	}
	// Both turns are marked, so the three segments read as one line.
	turns := 0
	for _, r := range grid {
		if r == '┌' || r == '┐' || r == '└' || r == '┘' {
			turns++
		}
	}
	if turns != 2 {
		t.Fatalf("a three-segment path must mark both turns, got %d", turns)
	}
}

// Every foreign key is drawn, and every turn it takes is marked, so a
// line never appears to stop for no reason.
func TestERConnectorsDrawnWithCorners(t *testing.T) {
	tables, links := focusFixture()
	ft, fl := erFocusedSubset("orders", tables, links)
	pos := erFocusLayout("orders", ft, fl, 82, 24)
	canvas := strings.Join(erRenderWithPos(ft, fl, pos, erFocusBoxLinesEx, "orders", ""), "\n")
	if !strings.ContainsAny(canvas, "┌┐└┘") {
		t.Fatalf("paths that turn must mark their corners:\n%s", canvas)
	}
	for _, l := range fl {
		if _, ok := pos[l.FromTable]; !ok {
			continue
		}
		if _, ok := pos[l.ToTable]; !ok {
			continue
		}
		if !strings.Contains(canvas, "─") {
			t.Fatalf("link %s.%s -> %s.%s drew no line:\n%s", l.FromTable, l.FromColumn, l.ToTable, l.ToColumn, canvas)
		}
	}
}

// Connectors are plain arrows: no cardinality text, no crow's-foot fans,
// bars or circles. Every link draws one continuous line ending in ▶/◀.
func TestERArrowNoCardinality(t *testing.T) {
	tables, links := gridFixture()
	pos := erGridLayout(tables, links, "users", 96, 30)
	canvas := strings.Join(erRenderWithPos(tables, links, pos, erFocusBoxLinesEx, "users", ""), "\n")
	if !strings.Contains(canvas, "▶") && !strings.Contains(canvas, "◀") {
		t.Fatalf("arrow connectors must end in ▶/◀:\n%s", canvas)
	}
	for _, bad := range []string{"0..", "1..n", "╱", "╲", "○"} {
		if strings.Contains(canvas, bad) {
			t.Fatalf("arrow mode must not contain %q:\n%s", bad, canvas)
		}
	}
}

// A relationship must meet its target box with an arrow head: ▶ just
// left of the PK box, or ◀ just right of it. A line that ends against
// the wrong box, or without its head, reads as a wire into nowhere.
func TestERArrowEndsMeetTheirBoxes(t *testing.T) {
	tables, links := gridFixture()
	for _, focused := range []bool{false, true} {
		ft, fl := tables, links
		pos := erGridLayout(tables, links, "users", 96, 30)
		if focused {
			ft, fl = erFocusedSubset("users", tables, links)
			pos = erFocusLayout("users", ft, fl, 96, 30)
		}
		canvas := erRenderWithPos(ft, fl, pos, erFocusBoxLinesEx, "users", "")
		for _, l := range links {
			_, okSrc := pos[l.FromTable]
			_, okTgt := pos[l.ToTable]
			if !okSrc || !okTgt {
				continue
			}
			if l.FromTable != l.ToTable && !erHasArrow(canvas, pos[l.ToTable]) {
				t.Fatalf("focused=%v: no arrow head touches %s, the target of %s.%s -> %s.%s:\n%s",
					focused, l.ToTable, l.FromTable, l.FromColumn, l.ToTable, l.ToColumn,
					strings.Join(canvas, "\n"))
			}
		}
	}
}

// erHasArrow reports whether an arrow head touches a box edge: ▶ just
// left of the border or ◀ just right of it.
func erHasArrow(canvas []string, r erRect) bool {
	for y := r.y; y < r.y+r.h+3; y++ {
		if erCell(canvas, r.x-1, y) == '▶' {
			return true
		}
		if erCell(canvas, r.x+r.w, y) == '◀' {
			return true
		}
	}
	return false
}

// erCell is the rune drawn at one canvas cell.
func erCell(canvas []string, x, y int) rune {
	if x < 0 || y < 0 || y >= len(canvas) {
		return 0
	}
	for _, r := range ansi.Strip(ansi.Cut(canvas[y], x, x+1)) {
		return r
	}
	return 0
}

// gridFixture mirrors the reported schema: a row of tables where one
// link skips two columns, one table stacks below, and one table
// references itself.
func gridFixture() ([]erTable, []dbpkg.ForeignKey) {
	tbl := func(name string, pk, fk []string, rest ...string) erTable {
		t := erTable{name: name, pk: map[string]bool{}, fk: map[string]bool{}}
		for _, c := range append(append(append([]string{}, pk...), fk...), rest...) {
			t.cols = append(t.cols, dbpkg.Column{Name: c, Type: "TEXT"})
		}
		for _, c := range pk {
			t.pk[c] = true
		}
		for _, c := range fk {
			t.fk[c] = true
		}
		return t
	}
	tables := []erTable{
		tbl("users", []string{"id"}, nil, "email"),
		tbl("comments", []string{"id"}, []string{"user_id", "post_id"}),
		tbl("posts", []string{"id"}, []string{"user_id"}),
		tbl("products", []string{"id"}, []string{"category_id"}),
		tbl("categories", []string{"id"}, []string{"parent_id"}),
	}
	links := []dbpkg.ForeignKey{
		{FromTable: "categories", FromColumn: "parent_id", ToTable: "categories", ToColumn: "id"},
		{FromTable: "comments", FromColumn: "user_id", ToTable: "users", ToColumn: "id"},
		{FromTable: "comments", FromColumn: "post_id", ToTable: "posts", ToColumn: "id"},
		{FromTable: "posts", FromColumn: "user_id", ToTable: "users", ToColumn: "id"},
		{FromTable: "products", FromColumn: "category_id", ToTable: "categories", ToColumn: "id"},
	}
	return tables, links
}
