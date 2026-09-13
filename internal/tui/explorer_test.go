package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func fixtureExplorer() Explorer {
	e := NewExplorer("shop", []string{"public"})
	e.Schemas[0].Expanded = true
	e.Schemas[0].Tables = []TableNode{
		{Schema: "public", Name: "orders", Count: 4000, CountOK: true, Expanded: true, Columns: []ColumnNode{{Name: "id", DataType: "integer", IsPK: true}, {Name: "status", DataType: "text"}}},
		{Schema: "public", Name: "customers", Count: 500, CountOK: true},
	}
	return e
}

func TestVisibleRowsFlatten(t *testing.T) {
	e := fixtureExplorer()
	rows := e.VisibleRows()
	if len(rows) != 5 {
		t.Fatalf("want 5 rows (schema+orders+2 cols+customers), got %d: %v", len(rows), rows)
	}
	if rows[0].Kind != RowSchema || rows[1].Kind != RowTable || rows[2].Kind != RowColumn {
		t.Fatalf("bad order: %v", rows)
	}
}

func TestToggleCollapse(t *testing.T) {
	e := fixtureExplorer()
	e.Cursor = 1
	e.Toggle()
	if len(e.VisibleRows()) != 3 {
		t.Fatalf("collapse orders should hide 2 cols, got %d", len(e.VisibleRows()))
	}
}

func TestHumanizeCount(t *testing.T) {
	cases := map[int64]string{999: "999", 500: "500", 4000: "4.0k", 12000: "12.0k", 2000000: "2.0M"}
	for n, want := range cases {
		if got := humanizeCount(n); got != want {
			t.Fatalf("humanizeCount(%d)=%q want %q", n, got, want)
		}
	}
}

func TestFilterKeepsParents(t *testing.T) {
	e := fixtureExplorer()
	e.SetFilter("cust")
	rows := e.VisibleRows()
	if len(rows) != 2 {
		t.Fatalf("want schema+customers, got %v", rows)
	}
}

func TestExplorerRenderGoldSelection(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	e := fixtureExplorer()
	out := e.Render(34, 20)
	if !strings.Contains(out, "explorer") {
		t.Fatalf("missing header:\n%s", out)
	}
	if !strings.Contains(out, "48;5;178m") {
		t.Fatalf("sidebar selection must use gold bg 178, got:\n%s", out)
	}
	if !strings.Contains(out, "4.0k") || !strings.Contains(out, "integer") {
		t.Fatalf("missing count/type:\n%s", out)
	}
	for _, ln := range strings.Split(out, "\n") {
		if lipgloss.Width(ln) > 34 {
			t.Fatalf("line exceeds width: %q", ln)
		}
	}
}
