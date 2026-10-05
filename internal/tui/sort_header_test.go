package tui

import (
	"testing"
)

func TestSortHeaderClick(t *testing.T) {
	m := queryTabModel()
	m.tab = 2
	m.focusDetail = true
	m.width, m.height = 120, 40
	m.resizeBrowse()
	m.rowTable.setData([]string{"name"}, [][]string{{"bob"}, {"alice"}})
	// Header row for rows tab is one below detailTableTop (pager line).
	y := detailTableTop + 1
	x := m.paneX() + 1
	u, _ := m.Update(testClick(x, y))
	m = u.(Model)
	rows := m.rowTable.Rows()
	if rows[0][0] != "alice" {
		t.Fatalf("header click must sort asc, got %v", rows)
	}
}
