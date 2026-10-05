package tui

import (
	"testing"
)

func TestGridFilterSubstringAcrossColumns(t *testing.T) {
	var dt dataTable
	dt.Resize(60, 8)
	dt.setData([]string{"name", "city"}, [][]string{
		{"alice", "Berlin"},
		{"bob", "Paris"},
		{"carol", "berlin Wall"},
	})
	dt.SetFilter("berlin")
	rows := dt.Rows()
	if len(rows) != 2 || rows[0][0] != "alice" || rows[1][0] != "carol" {
		t.Fatalf("filter must match across columns case-insensitive, got %v", rows)
	}
	if got := dt.FilterText(); got != "berlin" {
		t.Fatalf("filter text, got %q", got)
	}
}

func TestGridFilterNullNeverMatchesAndClearRestores(t *testing.T) {
	var dt dataTable
	dt.Resize(60, 8)
	dt.setData([]string{"a"}, [][]string{{"b"}, {"NULL"}, {"a"}})
	dt.SortBy(0) // asc: a, b, NULL last
	dt.SetFilter("a")
	if rows := dt.Rows(); len(rows) != 1 || rows[0][0] != "a" {
		t.Fatalf("filter a must leave one row, got %v", dt.Rows())
	}
	dt.ClearFilter()
	rows := dt.Rows()
	if len(rows) != 3 || rows[0][0] != "a" || rows[1][0] != "b" {
		t.Fatalf("clear must restore sorted order, got %v", rows)
	}
}
