package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestResultsFillerShowsViewState(t *testing.T) {
	m := queryTabModel()
	m.tab, m.focusDetail = 3, true
	m.queryFocus = 1
	m.width, m.height = 120, 40
	m.resizeBrowse()
	m.queryTable.setData([]string{"name", "age"}, [][]string{{"bob", "30"}, {"alice", "25"}})

	// Clean: filler stays blank.
	if line := resultsFillerLine(m.detailView()); line != "" {
		t.Fatalf("clean filler must be blank, got %q", line)
	}

	m.queryTable.SortBy(1)
	m.queryTable.SetFilter("ali")
	line := resultsFillerLine(m.detailView())
	if !strings.Contains(line, "age") || !strings.Contains(line, "ali") {
		t.Fatalf("filler must show sort+filter state, got %q", line)
	}
}

// resultsFillerLine extracts the editor-hint row (first blank-or-hint line
// directly below the editor panel bottom border) from a detail view.
func resultsFillerLine(view string) string {
	lines := strings.Split(view, "\n")
	for i, ln := range lines {
		if strings.Contains(ln, "╰") && i+1 < len(lines) {
			return strings.TrimSpace(ansi.Strip(lines[i+1]))
		}
	}
	return "<panel-not-found>"
}
