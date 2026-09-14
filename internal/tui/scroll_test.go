package tui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func bigExplorer() Explorer {
	e := NewExplorer("test", []string{"s"})
	e.Schemas[0].Expanded = true
	for i := 0; i < 30; i++ {
		e.Schemas[0].Tables = append(e.Schemas[0].Tables,
			TableNode{Schema: "s", Name: fmt.Sprintf("t%02d", i), CountOK: true, Count: int64(i)})
	}
	return e
}

func TestExplorerEnsureVisibleFollowsCursor(t *testing.T) {
	e := bigExplorer() // 31 rows: 1 schema + 30 tables
	e.Cursor = 29
	e.ensureVisible(10)
	if e.Offset != 20 {
		t.Fatalf("want offset 20, got %d", e.Offset)
	}
	e.Cursor = 0
	e.ensureVisible(10)
	if e.Offset != 0 {
		t.Fatalf("want offset 0, got %d", e.Offset)
	}
}

func TestRenderShowsWindowAndClampsPurely(t *testing.T) {
	e := bigExplorer()
	e.Cursor = 29
	e.ensureVisible(10)
	out := e.Render(32, 10)
	lines := strings.Split(out, "\n")
	if !strings.Contains(lines[2], "t19") {
		t.Fatalf("first visible tree row must be t19, got %q", lines[2])
	}
	if strings.Contains(out, "t00") {
		t.Fatalf("t00 must be scrolled out:\n%s", out)
	}
	e.Offset = 999
	_ = e.Render(32, 10) // stale offset: clamp locally, do NOT mutate
	if e.Offset != 999 {
		t.Fatalf("Render must stay pure, offset mutated to %d", e.Offset)
	}
}

func TestSidebarNarrowRenderHasNoBrokenEscapes(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	e := NewExplorer("postgres", []string{"auth"})
	e.Schemas[0].Expanded = true
	e.Schemas[0].Tables = []TableNode{
		{Schema: "auth", Name: "mfa_factors", Expanded: true, CountErr: true, Columns: []ColumnNode{
			{Name: "id", DataType: "uuid", IsPK: true},
			{Name: "status", DataType: "USER-DEFINED"},
			{Name: "last_webauthn_challenge_xyz", DataType: "text"},
		}},
	}
	re := regexp.MustCompile("\x1b\\[[0-9;]*[A-Za-z]")
	for _, w := range []int{18, 20, 24, 32} {
		out := e.Render(w, 40)
		for i, ln := range strings.Split(out, "\n") {
			if lipgloss.Width(ln) > w {
				t.Fatalf("width %d line %d exceeds: %q", w, i, ln)
			}
		}
		if rest := re.ReplaceAllString(out, ""); strings.Contains(rest, "\x1b") {
			t.Fatalf("width %d has broken escape sequence:\n%q", w, out)
		}
	}
}
