package tui

import (
	"strings"
	"testing"
)

func TestQueryTitleShowsFocusCue(t *testing.T) {
	m := queryTabModel()
	m.tab, m.focusDetail = 3, true
	m.width, m.height = 120, 40
	m.resizeBrowse()

	m.queryFocus = 0
	if got := m.detailView(); !strings.Contains(got, "● editor") {
		t.Fatalf("editor focus must cue in title, got:\n%s", got)
	}
	m.queryFocus = 1
	if got := m.detailView(); !strings.Contains(got, "● results") {
		t.Fatalf("results focus must cue in title, got:\n%s", got)
	}

	m.tab = 2
	if got := m.detailView(); strings.Contains(got, "● editor") || strings.Contains(got, "● results") {
		t.Fatalf("other tabs must not cue query focus, got:\n%s", got)
	}
}
