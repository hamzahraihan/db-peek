package tui

import (
	"strings"
	"testing"
)

func TestEditorHintRowCompressed(t *testing.T) {
	m := queryTabModel()
	m.tab, m.focusDetail = 3, true
	m.queryFocus = 0
	m.width, m.height = 120, 40
	m.resizeBrowse()
	view := m.detailView()
	for _, want := range []string{"ctrl+r", "esc", "keys"} {
		if !strings.Contains(view, want) {
			t.Fatalf("editor hint must keep %q, got:\n%s", want, view)
		}
	}
	for _, drop := range []string{"ctrl+s", "ctrl+o", "F2", "ctrl+d", "ctrl+/"} {
		if strings.Contains(view, drop) {
			t.Fatalf("editor hint must not carry %q, got:\n%s", drop, view)
		}
	}
}
