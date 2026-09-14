package tui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

func TestSidebarScrollbarOverlay(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	e := bigExplorer() // 31 rows
	out := e.Render(32, 10)
	lines := strings.Split(out, "\n")
	if !strings.Contains(lines[2], "█") {
		t.Fatalf("top of overflow must show thumb on first tree line:\n%s", out)
	}
	n := 0
	for _, ln := range lines {
		if strings.Contains(ln, "█") {
			n++
		}
	}
	if n != 3 { // 10*10/31 = 3
		t.Fatalf("want thumb height 3, got %d:\n%s", n, out)
	}
	small := NewExplorer("t", []string{"s"})
	small.Schemas[0].Expanded = true
	small.Schemas[0].Tables = []TableNode{{Schema: "s", Name: "only", CountOK: true}}
	if out := small.Render(32, 10); strings.Contains(out, "█") || strings.Contains(out, "│") {
		t.Fatalf("no scrollbar without overflow:\n%s", out)
	}
}

func TestSidebarKeyboardScrollsViewport(t *testing.T) {
	m := browseModel(t)
	m.height = 12 // tree viewport = 12-3-2-4 = 3 rows; fixture has 5 rows
	m.loading = false
	down := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}
	for i := 0; i < 4; i++ {
		u, _ := m.Update(down)
		m = u.(Model)
	}
	if m.explorer.Cursor != 4 {
		t.Fatalf("want cursor 4, got %d", m.explorer.Cursor)
	}
	if m.explorer.Offset != 2 { // 4-3+1
		t.Fatalf("want offset 2, got %d", m.explorer.Offset)
	}
}

func TestSidebarWheelScrollsViewport(t *testing.T) {
	m := browseModel(t)
	m.height = 12
	m.loading = false
	m.focusDetail = false
	u, _ := m.Update(tea.MouseMsg{Type: tea.MouseWheelDown})
	m = u.(Model)
	if m.explorer.Offset == 0 && m.explorer.Cursor >= m.sidebarTreeH() {
		t.Fatalf("wheel must pull viewport along: cursor=%d offset=%d treeH=%d",
			m.explorer.Cursor, m.explorer.Offset, m.sidebarTreeH())
	}
}

func TestSidebarClickUsesOffset(t *testing.T) {
	m := browseModel(t)
	m.height = 12 // tree viewport 3 rows; fixture rows 0..4
	m.loading = false
	m.focusDetail = false
	m.explorer.Offset = 2
	// First visible tree row (y=5) is fixture row 2 (col id → previews orders).
	u, _ := m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 2, Y: 5})
	m = u.(Model)
	if m.explorer.Cursor != 2 {
		t.Fatalf("want cursor 2, got %d", m.explorer.Cursor)
	}
	if m.table != "orders" {
		t.Fatalf("want orders previewed, got %q", m.table)
	}
	if m.focusDetail {
		t.Fatal("sidebar click must not steal focus")
	}
}

func TestSidebarHoverUsesOffset(t *testing.T) {
	m := browseModel(t)
	m.height = 12
	m.loading = false
	m.explorer.Offset = 2
	u, _ := m.Update(tea.MouseMsg{Type: tea.MouseMotion, X: 2, Y: 6})
	m = u.(Model)
	if m.explorer.Cursor != 3 {
		t.Fatalf("want cursor 3, got %d", m.explorer.Cursor)
	}
}
