package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestHelpRegistryCoversHandlers(t *testing.T) {
	handled := []string{"up", "k", "down", "j", "left", "right", "enter", "/", "r", "c", "esc", "q", "tab", "1", "2", "3", "4", "5", "n", "p", "s", "pgup", "pgdown", "ctrl+u", "ctrl+d", "ctrl+r", "g", "G", "home", "end", "f5", "?", "a", "e", "d", "backspace", "h", "l", "shift+tab"}
	have := map[string]bool{}
	for _, b := range keyRegistry {
		if b.Key == "/" {
			have["/"] = true // lone "/" never survives the Split below
			continue
		}
		if strings.Contains(b.Key, "..") {
			continue
		}
		for _, k := range strings.Split(b.Key, "/") {
			have[strings.ToLower(strings.TrimSpace(k))] = true
		}
	}
	var missing []string
	for _, k := range handled {
		if !have[strings.ToLower(k)] {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("registry missing keys: %v", missing)
	}
}

func TestHelpToggle(t *testing.T) {
	m := browseModel(t)
	m.loading = false
	u, _ := m.Update(testKey("?"))
	m = u.(Model)
	if !m.showHelp {
		t.Fatal("? must open help")
	}
	u, _ = m.Update(testKey("esc"))
	m = u.(Model)
	if m.showHelp {
		t.Fatal("esc must close help")
	}
}

func TestSpliceCellsPreservesSides(t *testing.T) {
	base := titleStyle.Render("LEFTPART12") + strings.Repeat(" ", 20) + errStyle.Render("RIGHTPART!")
	// 10 + 20 + 10 = 40 cells.
	out := spliceCells(base, "BOX", 14, 8, 40)
	if got := lipgloss.Width(out); got != 40 {
		t.Fatalf("splice must stay 40 wide, got %d: %q", got, out)
	}
	if !strings.Contains(out, "BOX") {
		t.Fatalf("overlay segment must land:\n%q", out)
	}
	if left := ansi.Strip(ansi.Cut(out, 0, 14)); left != ansi.Strip(ansi.Cut(base, 0, 14)) {
		t.Fatalf("left background must survive:\nbase %q\nout  %q", base, out)
	}
	wantR := strings.TrimRight(ansi.Strip(ansi.Cut(base, 22, 40)), " ")
	gotR := strings.TrimRight(ansi.Strip(ansi.Cut(out, 22, 40)), " ")
	if gotR != wantR {
		t.Fatalf("right background must survive:\nbase %q\nout  %q", base, out)
	}
	// Styling (not just text) must survive on both sides (v2 renders
	// full-fidelity SGR: bright-blue 94, bright-red 91).
	if !strings.Contains(out, "94m") {
		t.Fatalf("left styling must survive:\n%q", out)
	}
	if !strings.Contains(out, "91m") {
		t.Fatalf("right styling must survive:\n%q", out)
	}
}

func TestHelpViewKeepsBackground(t *testing.T) {
	m := browseModel(t)
	base := m.viewString()
	m.showHelp = true
	out := m.helpView()
	x, y, w, h := m.helpRect()
	if h >= m.height || w >= m.width {
		t.Fatalf("help box must stay smaller than the screen (got %dx%d on %dx%d) so background remains visible",
			w, h, m.width, m.height)
	}
	baseLines := strings.Split(base, "\n")
	outLines := strings.Split(out, "\n")
	if len(baseLines) != len(outLines) {
		t.Fatalf("overlay must not add/remove rows: %d vs %d", len(baseLines), len(outLines))
	}
	side := func(s string, from, to int) string {
		return strings.TrimRight(ansi.Strip(ansi.Cut(s, from, to)), " ")
	}
	for i := range baseLines {
		if i < y || i >= y+h {
			if outLines[i] != baseLines[i] {
				t.Fatalf("row %d outside the box must be identical:\nbase %q\nout  %q", i, baseLines[i], outLines[i])
			}
			continue
		}
		if got, want := side(outLines[i], 0, x), side(baseLines[i], 0, x); got != want {
			t.Fatalf("row %d: UI left of the box must survive:\nbase %q\nout  %q", i, baseLines[i], outLines[i])
		}
		if got, want := side(outLines[i], x+w, m.width), side(baseLines[i], x+w, m.width); got != want {
			t.Fatalf("row %d: UI right of the box must survive:\nbase %q\nout  %q", i, baseLines[i], outLines[i])
		}
	}
}
