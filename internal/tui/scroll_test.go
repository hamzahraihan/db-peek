package tui

import (
	"fmt"
	"strings"
	"testing"
)

func bigExplorer() Explorer {
	e := NewExplorer("test", []string{"s"})
	e.Schemas[0].Expanded = true
	for i := 0; i < 30; i++ {
		e.Schemas[0].Tables = append(e.Schemas[0].Tables,
			TableNode{Schema: "s", Name: fmt.Sprintf("t%02d", i), CountOK: true, Count: int64(i)})
	}
	return e
}

func TestExplorerEnsureVisibleFollowsCursor(t *testing.T) {
	e := bigExplorer() // 31 rows: 1 schema + 30 tables
	e.Cursor = 29
	e.ensureVisible(10)
	if e.Offset != 20 {
		t.Fatalf("want offset 20, got %d", e.Offset)
	}
	e.Cursor = 0
	e.ensureVisible(10)
	if e.Offset != 0 {
		t.Fatalf("want offset 0, got %d", e.Offset)
	}
}

func TestRenderShowsWindowAndClampsPurely(t *testing.T) {
	e := bigExplorer()
	e.Cursor = 29
	e.ensureVisible(10)
	out := e.Render(32, 10)
	lines := strings.Split(out, "\n")
	if !strings.Contains(lines[2], "t19") {
		t.Fatalf("first visible tree row must be t19, got %q", lines[2])
	}
	if strings.Contains(out, "t00") {
		t.Fatalf("t00 must be scrolled out:\n%s", out)
	}
	e.Offset = 999
	_ = e.Render(32, 10) // stale offset: clamp locally, do NOT mutate
	if e.Offset != 999 {
		t.Fatalf("Render must stay pure, offset mutated to %d", e.Offset)
	}
}
