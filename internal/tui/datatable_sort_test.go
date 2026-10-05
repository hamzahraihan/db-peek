package tui

import (
	"strings"
	"testing"
)

func TestSortByAscText(t *testing.T) {
	var dt dataTable
	dt.Resize(60, 8)
	dt.setData([]string{"name", "age"}, [][]string{
		{"bob", "30"},
		{"alice", "25"},
		{"carol", "22"},
	})
	dt.SortBy(0)
	rows := dt.Rows()
	if len(rows) != 3 || rows[0][0] != "alice" || rows[1][0] != "bob" || rows[2][0] != "carol" {
		t.Fatalf("asc sort by col 0, got %v", rows)
	}
	if got := dt.SortState(); got != "0:asc" {
		t.Fatalf("sort state, got %q", got)
	}
	header := strings.Split(dt.View(), "\n")[0]
	if !strings.Contains(header, "▲") {
		t.Fatalf("header must show asc marker, got %q", header)
	}
}

func TestSortNumericAndNullsLast(t *testing.T) {
	var dt dataTable
	dt.Resize(60, 8)
	dt.setData([]string{"n"}, [][]string{{"30"}, {"NULL"}, {"9"}, {""}, {"100"}})
	dt.SortBy(0)
	rows := dt.Rows()
	if rows[0][0] != "9" || rows[1][0] != "30" || rows[2][0] != "100" {
		t.Fatalf("numeric asc, got %v", rows)
	}
	// Nulls (NULL + empty) sort last in both directions.
	if rows[3][0] == "9" || rows[4][0] == "9" {
		t.Fatalf("nulls must be last, got %v", rows)
	}
	dt.SortBy(0) // desc
	rows = dt.Rows()
	if rows[0][0] != "100" || rows[1][0] != "30" || rows[2][0] != "9" {
		t.Fatalf("numeric desc, got %v", rows)
	}
}

func TestSortToggleClearsRestores(t *testing.T) {
	var dt dataTable
	dt.Resize(60, 8)
	dt.setData([]string{"a"}, [][]string{{"b"}, {"a"}, {"c"}})
	dt.SortBy(0)
	if got := dt.SortState(); got != "0:asc" {
		t.Fatalf("want asc, got %q", got)
	}
	dt.SortBy(0)
	if got := dt.SortState(); got != "0:desc" {
		t.Fatalf("want desc, got %q", got)
	}
	if rows := dt.Rows(); rows[0][0] != "c" {
		t.Fatalf("desc first must be c, got %v", rows)
	}
	dt.SortBy(0) // third click clears
	if got := dt.SortState(); got != "" {
		t.Fatalf("third click must clear, got %q", got)
	}
	rows := dt.Rows()
	if rows[0][0] != "b" || rows[1][0] != "a" || rows[2][0] != "c" {
		t.Fatalf("clear must restore original order, got %v", rows)
	}
}
