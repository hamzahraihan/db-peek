package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	dbpkg "db-peek/internal/db"
)

// A wrapped line pushes every row below it down, silently desyncing mouse
// hit-testing. No rendered line may exceed the terminal width.
func TestViewLinesFitTerminal(t *testing.T) {
	for _, w := range []int{30, 80} {
		m := New("", nil)
		m.width = w
		m.status = strings.Repeat("s", 60)
		m.screen = screenBrowse
		m.table = strings.Repeat("t", 60)
		m.count = 7
		lines := strings.Split(m.detailView(), "\n")
		for i, ln := range lines {
			if lipgloss.Width(ln) > w {
				t.Fatalf("width %d line %d wraps at %d: %q", w, i, lipgloss.Width(ln), ln)
			}
		}
	}
}

func TestBrowseViewRendersTableData(t *testing.T) {
	m := browseModel(t)
	m.table = "users"
	m.cols = []dbpkg.Column{
		{Name: "id", Type: "integer", Nullable: "NO", Default: "auto", Extra: "pk"},
		{Name: "name", Type: "text", Nullable: "YES", Default: "", Extra: ""},
	}
	m.sample = &dbpkg.Sample{
		Columns: []string{"id", "name"},
		Rows:    [][]string{{"1", "Alice"}, {"2", "Bob"}},
	}
	m.count = 2
	m.buildTables()
	m.sizeTables()
	m.loading = false

	// Default tab is schema (tab 0); verify schema data renders.
	view := m.View()
	if !strings.Contains(view, "integer") {
		t.Fatal("schema tab should contain column types")
	}
	if !strings.Contains(view, "text") {
		t.Fatal("schema tab should contain column types")
	}

	// Switch to rows tab and verify sample data renders.
	m.tab = 2
	m.sizeTables()
	view = m.View()
	if !strings.Contains(view, "Alice") {
		t.Fatal("rows tab should contain sample data")
	}
	if !strings.Contains(view, "Bob") {
		t.Fatal("rows tab should contain sample data")
	}
}
