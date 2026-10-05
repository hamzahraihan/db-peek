package tui

import (
	"testing"
)

func TestSortKeyOSortsRows(t *testing.T) {
	m := queryTabModel()
	m.tab = 2
	m.focusDetail = true
	m.rowTable.Resize(60, 10)
	m.rowTable.setData([]string{"name"}, [][]string{{"bob"}, {"alice"}})
	m.rowTable.SetCol(0)
	u, _ := m.Update(testKey("o"))
	m = u.(Model)
	rows := m.rowTable.Rows()
	if rows[0][0] != "alice" {
		t.Fatalf("o must sort asc, got %v", rows)
	}
	u, _ = m.Update(testKey("o"))
	m = u.(Model)
	rows = m.rowTable.Rows()
	if rows[0][0] != "bob" {
		t.Fatalf("second o must sort desc, got %v", rows)
	}
}
