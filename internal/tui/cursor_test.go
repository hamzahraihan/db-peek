package tui

import "testing"

func cursorTable() dataTable {
	tbl := dataTable{}
	tbl.Resize(60, 12)
	tbl.setData([]string{"id", "name", "email"},
		[][]string{{"1", "ada", "a@x"}, {"2", "bob", "b@x"}})
	return tbl
}

// ColAt is the inverse of the column's x-range: every column's cells map
// back to its index, and past the last column is -1.
func TestColAtMapsXRanges(t *testing.T) {
	tbl := cursorTable()
	total := tbl.totalWidth()
	for col, w := range tbl.widths {
		left := tbl.colX(col)
		for x := left; x < left+w; x++ {
			if got := tbl.ColAt(x); got != col {
				t.Fatalf("x=%d should be column %d, got %d", x, col, got)
			}
		}
	}
	if got := tbl.ColAt(total); got != -1 {
		t.Fatalf("past the last column must miss, got %d", got)
	}
	if got := tbl.ColAt(total + 50); got != -1 {
		t.Fatalf("far past the last column must miss, got %d", got)
	}
	if got := tbl.ColAt(0); got != 0 {
		t.Fatalf("x=0 is the first column, got %d", got)
	}
}

// The column cursor moves and clamps at both ends.
func TestColumnCursorMovesAndClamps(t *testing.T) {
	tbl := cursorTable()
	if tbl.Col() != 0 {
		t.Fatalf("the column cursor starts at 0, got %d", tbl.Col())
	}
	tbl.MoveColRight()
	if tbl.Col() != 1 {
		t.Fatalf("want column 1, got %d", tbl.Col())
	}
	tbl.MoveColRight()
	tbl.MoveColRight()
	if tbl.Col() != len(tbl.widths)-1 {
		t.Fatalf("the column cursor must clamp at the last column, got %d", tbl.Col())
	}
	for range 5 {
		tbl.MoveColLeft()
	}
	if tbl.Col() != 0 {
		t.Fatalf("the column cursor must clamp at the first column, got %d", tbl.Col())
	}
	// Fewer columns than the cursor: it must be re-clamped, not panic.
	tbl.setData([]string{"only"}, [][]string{{"x"}})
	if tbl.Col() != 0 {
		t.Fatalf("a narrower grid must clamp the column cursor, got %d", tbl.Col())
	}
}
