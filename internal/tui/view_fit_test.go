package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// A wrapped line pushes every row below it down, silently desyncing mouse
// hit-testing. No rendered line may exceed the terminal width.
func TestViewLinesFitTerminal(t *testing.T) {
	for _, w := range []int{30, 80} {
		m := New("", nil)
		m.width = w
		m.status = strings.Repeat("s", 60)
		m.screen = screenDetail
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
