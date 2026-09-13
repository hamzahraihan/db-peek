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
