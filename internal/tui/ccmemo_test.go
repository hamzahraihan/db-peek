package tui

import "testing"

func completionModel(t *testing.T) Model {
	t.Helper()
	m := browseModel(t)
	m.screen = screenBrowse
	m.focusDetail = true
	m.tab = 3
	m.queryFocus = 0
	m.explorer = fixtureExplorer()
	m.explorerGen++
	return m
}

func candidateTexts(items []completeItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Text
	}
	return out
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// The candidate index is rebuilt per keystroke; it must be reused while
// the explorer is unchanged and rebuilt once the tree changes.
func TestCompletionContextMemoFollowsExplorer(t *testing.T) {
	m := completionModel(t)
	tables, colsByTable, _ := m.completionContext()
	if !contains(tables, "orders") {
		t.Fatalf("fixture tables missing: %v", tables)
	}
	first := colsByTable["orders"]

	// Same generation: the same slices come back, no rebuild.
	tables2, colsByTable2, _ := m.completionContext()
	if !contains(tables2, "orders") || &colsByTable2["orders"][0] != &first[0] {
		t.Fatal("an unchanged explorer must reuse the memoized index")
	}

	// Mutating the tree without a bump is the documented contract: the
	// memo wins, so a caller that changes the tree must bump explorerGen.
	m.explorer.Schemas[0].Tables[0].Columns = append(m.explorer.Schemas[0].Tables[0].Columns, ColumnNode{Name: "zzz_new"})
	_, colsByTable3, _ := m.completionContext()
	if contains(colsByTable3["orders"], "zzz_new") {
		t.Fatal("without explorerGen the memo must still be returned")
	}

	m.explorerGen++
	_, colsByTable4, _ := m.completionContext()
	if !contains(colsByTable4["orders"], "zzz_new") {
		t.Fatalf("bumping explorerGen must rebuild the index, got %v", colsByTable4["orders"])
	}
}

// Switching tables must not serve the previous table's column index.
func TestCompletionContextMemoFollowsTable(t *testing.T) {
	m := completionModel(t)
	m.table = "orders"
	_, _, curCols := m.completionContext()
	m.cols = nil
	m.colsGen++
	m.table = "customers"
	_, _, otherCols := m.completionContext()
	if len(otherCols) == len(curCols) && len(otherCols) > 0 {
		t.Fatalf("a different table must not reuse the previous index: %v vs %v", otherCols, curCols)
	}
}

// Dot mode must still resolve to the qualified table under the new arity.
func TestDotModeStillResolvesThroughRefresh(t *testing.T) {
	m := completionModel(t)
	m.editor.SetText("SELECT orders.")
	m.editor.CurLine, m.editor.CurCol = 0, len("SELECT orders.")
	m.refreshCompletion()
	if !m.showComplete {
		t.Fatal("dot mode must open the popup")
	}
	if !contains(candidateTexts(m.completeItems), "status") {
		t.Fatalf("dot mode must list the table's columns, got %+v", m.completeItems)
	}
	for _, it := range m.completeItems {
		if it.Kind != "column" {
			t.Fatalf("dot mode must return columns only, got %+v", it)
		}
	}
}
