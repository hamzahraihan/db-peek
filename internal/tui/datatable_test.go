package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestZebraEvenRows(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	var dt dataTable
	dt.Resize(40, 8)
	dt.setData([]string{"a"}, [][]string{{"r0"}, {"r1"}, {"r2"}, {"r3"}})
	dt.SetCursor(99) // clamp away: no selection highlight in output
	dt.cursor = -1
	out := dt.View()
	lines := strings.Split(out, "\n")
	if len(lines) != 6 { // header + border + 4 rows
		t.Fatalf("want 6 lines, got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[3], "48;5;235m") { // 2nd data row (even-numbered) darker
		t.Fatalf("even row missing zebra bg:\n%s", out)
	}
	if strings.Contains(lines[2], "48;5;235m") {
		t.Fatalf("odd row must not carry zebra bg:\n%s", out)
	}
}
