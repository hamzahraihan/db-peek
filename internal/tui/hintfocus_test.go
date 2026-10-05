package tui

import (
	"strings"
	"testing"
)

func TestQueryHintsFollowFocus(t *testing.T) {
	m := queryTabModel()
	m.tab, m.focusDetail = 3, true
	m.width, m.height = 120, 40
	m.resizeBrowse()

	m.queryFocus = 0
	view := m.detailView()
	if !strings.Contains(view, "ctrl+r") {
		t.Fatalf("editor focus must show editor keys, got:\n%s", view)
	}
	if foot := m.detailFoot(); strings.Contains(foot, "H/L") {
		t.Fatalf("editor focus footer must not show results keys, got %q", foot)
	}

	m.queryFocus = 1
	view = m.detailView()
	if strings.Contains(view, "ctrl+r run") {
		t.Fatalf("results focus must hide editor keys, got:\n%s", view)
	}
	if foot := m.detailFoot(); !strings.Contains(foot, "H/L") {
		t.Fatalf("results focus footer must show buffer keys, got %q", foot)
	}
}
