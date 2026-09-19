package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

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
	pos := erFocusLayout("orders", tables, links)
	// Incoming (order_items) left of center (orders) left of outgoing (customers).
	if pos["order_items"].x >= pos["orders"].x {
		t.Fatalf("incoming must be left of center: %+v", pos)
	}
	if pos["orders"].x >= pos["customers"].x {
		t.Fatalf("center must be left of outgoing: %+v", pos)
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
	m.erSchema = erSchemaState{loaded: true, tables: tables, links: links}
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
	if !strings.Contains(out, "▶") && !strings.Contains(out, "┄") {
		t.Fatalf("focused view must draw connectors:\n%s", out)
	}
}

func TestERFocusToggle(t *testing.T) {
	m := browseModel(t)
	tables, links := focusFixture()
	m.erSchema = erSchemaState{loaded: true, tables: tables, links: links}
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
	m.erSchema = erSchemaState{loaded: true, tables: tables, links: links}
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
	m.erSchema = erSchemaState{loaded: true, tables: tables, links: links}
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
	m.erSchema = erSchemaState{loaded: true, tables: tables, links: links}
	m.erFocus = true
	m.erCenter = "orders"
	m.erSel = "orders"
	next := erNextBox(m.erVisibleTables(), "orders", 1)
	if next == "unrelated" || next == "" || next == "orders" {
		t.Fatalf("n must cycle within focused subset, got %q", next)
	}
}

