package tui

import (
	"testing"
)

// The sidebar tree viewport must agree with mouse hit-testing: Render
// clamps a stale Offset locally without writing it back, so hit-testing
// must use the same clamped start. Otherwise, after the row list shrinks
// (e.g. collapsing a schema/table while scrolled), clicks and hovers map
// to rows that are not on screen.

// scrolledShrunkModel returns a model with 31 rows shrunk to 31→31? No:
// bigExplorer (31 rows) scrolled to the bottom, then the first table
// (expanded with 5 columns) collapsed via Toggle — exactly what a mouse
// click does today (no ensureVisible). Rows go 36 → 31 with a stale Offset.
func scrolledShrunkModel(t *testing.T) Model {
	t.Helper()
	m := browseModel(t)
	e := bigExplorer()
	e.Schemas[0].Tables[0].Expanded = true
	e.Schemas[0].Tables[0].Columns = []ColumnNode{
		{Name: "c0"}, {Name: "c1"}, {Name: "c2"}, {Name: "c3"}, {Name: "c4"},
	}
	m.explorer = e
	m.height = 20 // treeH = 20-3-2-4 = 11; rows = 36
	m.loading = false
	m.focusDetail = false
	for i := 0; i < 40; i++ {
		m.explorer.MoveDown()
	}
	m.explorer.ensureVisible(m.sidebarTreeH())
	if m.explorer.Offset == 0 {
		t.Fatal("setup must scroll the viewport")
	}
	// Collapse like clickExplorer does (Toggle, no viewport fixup).
	m.explorer.Cursor = 1 // t00 table row
	m.explorer.Toggle()
	return m
}

func TestSidebarClickAfterShrinkHitsVisibleRow(t *testing.T) {
	m := scrolledShrunkModel(t)
	want := m.explorer.visibleStart(m.sidebarTreeH())
	u, _ := m.Update(testClick(2, explorerFirstRow))
	m = u.(Model)
	if m.explorer.Cursor != want {
		t.Fatalf("click on first visible line must select displayed row %d, got %d (offset %d)",
			want, m.explorer.Cursor, m.explorer.Offset)
	}
	// The viewport itself must be canonical again.
	if m.explorer.Offset != want {
		t.Fatalf("click must repair stale offset to %d, got %d", want, m.explorer.Offset)
	}
}

func TestSidebarHoverAfterShrinkFollowsMouse(t *testing.T) {
	m := scrolledShrunkModel(t)
	start := m.explorer.visibleStart(m.sidebarTreeH())
	u, _ := m.Update(testMotion(2, explorerFirstRow+2))
	m = u.(Model)
	if want := start + 2; m.explorer.Cursor != want {
		t.Fatalf("hover two lines down must select displayed row %d, got %d", want, m.explorer.Cursor)
	}
}

func TestExplorerToggleKeepsOffsetValid(t *testing.T) {
	m := browseModel(t)
	m.explorer = bigExplorer()
	m.height = 20
	m.explorer.Cursor = 30
	m.explorer.ensureVisible(m.sidebarTreeH())
	// Collapse the only schema via the mouse path.
	m.loading = false
	m.focusDetail = false
	u, _ := m.Update(testClick(2, explorerFirstRow))
	m = u.(Model)
	n := len(m.explorer.VisibleRows())
	maxOff := n - m.sidebarTreeH()
	if maxOff < 0 {
		maxOff = 0
	}
	if m.explorer.Offset < 0 || m.explorer.Offset > maxOff {
		t.Fatalf("offset %d out of range [0,%d] after collapse (rows=%d)",
			m.explorer.Offset, maxOff, n)
	}
}
