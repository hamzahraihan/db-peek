package tui

import (
	"strings"
	"testing"

	dbpkg "db-peek/internal/db"
)

func TestERViewThreeBoxes(t *testing.T) {
	m := browseModel(t)
	m.table = "orders"
	m.tab = 4
	m.cols = []dbpkg.Column{{Name: "id", Extra: "PK(1)"}, {Name: "customer_id"}}
	m.erLinks = []dbpkg.ForeignKey{
		{FromTable: "orders", FromColumn: "customer_id", ToTable: "customers", ToColumn: "id"},
		{FromTable: "refunds", FromColumn: "order_id", ToTable: "orders", ToColumn: "id"},
	}
	out := m.erView(80, 20)
	for _, want := range []string{"orders", "customers", "refunds", "──▶", "◀──"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestERCanvasBoxes(t *testing.T) {
	tables := []erTable{
		{name: "customers", cols: []dbpkg.Column{{Name: "id", Type: "integer", Extra: "PK(1)"}, {Name: "name", Type: "text"}}, pk: map[string]bool{"id": true}},
		{name: "orders", cols: []dbpkg.Column{{Name: "id", Type: "integer", Extra: "PK(1)"}, {Name: "customer_id", Type: "integer"}}, pk: map[string]bool{"id": true}, fk: map[string]bool{"customer_id": true}},
	}
	pos := erGridLayout(tables, 80, 20)
	if len(pos) != 2 {
		t.Fatalf("want 2 boxes, got %v", pos)
	}
	lines := erBoxLines(tables[1], 2, false)
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"orders", "🔑", "➤", "123"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in:\n%s", want, joined)
		}
	}
}
func TestERCanvasConnectors(t *testing.T) {
	tables := []erTable{
		{name: "customers", cols: []dbpkg.Column{{Name: "id", Extra: "PK(1)"}}, pk: map[string]bool{"id": true}},
		{name: "orders", cols: []dbpkg.Column{{Name: "id", Extra: "PK(1)"}, {Name: "customer_id"}}, pk: map[string]bool{"id": true}, fk: map[string]bool{"customer_id": true}},
	}
	links := []dbpkg.ForeignKey{{FromTable: "orders", FromColumn: "customer_id", ToTable: "customers", ToColumn: "id"}}
	canvas := erRenderCanvas(tables, links, 80, 20, "orders")
	joined := strings.Join(canvas, "\n")
	if !strings.Contains(joined, "┄") && !strings.Contains(joined, "┆") {
		t.Fatalf("missing connector chars in:\n%s", joined)
	}
	for _, want := range []string{"customers", "orders"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing box %q", want)
		}
	}
}

func TestERPanClamp(t *testing.T) {
	m := browseModel(t)
	m.erPanX, m.erPanY = 9999, 9999
	out := m.erCanvasView(40, 10)
	if out == "" {
		t.Fatal("viewport should render even when pan is out of range")
	}
}

func TestERSchemaStateDefaults(t *testing.T) {
	m := browseModel(t)
	if m.erSchema.loaded {
		t.Fatal("erSchema should start unloaded")
	}
	if m.erPanX != 0 || m.erPanY != 0 {
		t.Fatal("pan should start at 0,0")
	}
}

func TestERSchemaLoadedMsgApplies(t *testing.T) {
	m := browseModel(t)
	m.table = "orders"
	m.erSeq = 7
	msg := erSchemaLoadedMsg{
		tables: []erTable{{name: "orders"}, {name: "customers"}},
		links:  []dbpkg.ForeignKey{{FromTable: "orders", FromColumn: "customer_id", ToTable: "customers", ToColumn: "id"}},
		seq:    7,
	}
	nm, _ := m.Update(msg)
	got := nm.(Model)
	if !got.erSchema.loaded || len(got.erSchema.tables) != 2 || len(got.erSchema.links) != 1 {
		t.Fatalf("schema not applied: %+v", got.erSchema)
	}
	if got.erSel != "orders" {
		t.Fatalf("erSel = %q, want orders", got.erSel)
	}
	// stale seq dropped
	stale := erSchemaLoadedMsg{tables: []erTable{{name: "x"}}, seq: 6}
	nm2, _ := got.Update(stale)
	if len(nm2.(Model).erSchema.tables) != 2 {
		t.Fatal("stale msg should be dropped")
	}
}
