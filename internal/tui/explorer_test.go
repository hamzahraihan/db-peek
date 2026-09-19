package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
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

func TestNewExplorerExpandsPublicFirst(t *testing.T) {
	// Supabase returns schemas alphabetically (auth before public);
	// explorer must still default to public like TablePlus/DBeaver.
	e := NewExplorer("supabase", []string{"auth", "public", "storage"})
	if len(e.Schemas) != 3 {
		t.Fatalf("want 3 schemas, got %v", e.Schemas)
	}
	for _, s := range e.Schemas {
		if s.Name == "public" && !s.Expanded {
			t.Fatalf("public should be expanded, got %+v", e.Schemas)
		}
		if s.Name != "public" && s.Expanded {
			t.Fatalf("only public should be expanded, got %+v", e.Schemas)
		}
	}
	// No public: fall back to first schema so tree isn't fully collapsed.
	e2 := NewExplorer("x", []string{"app", "other"})
	if !e2.Schemas[0].Expanded || e2.Schemas[1].Expanded {
		t.Fatalf("want first expanded only, got %+v", e2.Schemas)
	}
}

func TestExplorerRenderGoldSelection(t *testing.T) {
	e := fixtureExplorer()
	out := render256(e.Render(34, 20))
	if !strings.Contains(out, "explorer") {
		t.Fatalf("missing header:\n%s", out)
	}
	if !strings.Contains(out, "48;5;172m") {
		t.Fatalf("sidebar selection must use gold bg 172 (#CA8A04), got:\n%s", out)
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
