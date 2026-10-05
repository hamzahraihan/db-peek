package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestGridFindPromptFiltersLive(t *testing.T) {
	m := queryTabModel()
	m.tab = 2
	m.focusDetail = true
	m.width, m.height = 120, 40
	m.resizeBrowse()
	m.rowTable.setData([]string{"name"}, [][]string{{"alice"}, {"bob"}})
	u, _ := m.Update(testKey("/"))
	m = u.(Model)
	if !m.gridFiltering {
		t.Fatal("/ must open grid find prompt")
	}
	// Type "ali" into the prompt.
	for _, r := range "ali" {
		var msg tea.KeyPressMsg
		msg = tea.KeyPressMsg{Code: r, Text: string(r)}
		u, _ = m.Update(msg)
		m = u.(Model)
	}
	if rows := m.rowTable.Rows(); len(rows) != 1 || rows[0][0] != "alice" {
		t.Fatalf("live filter must shrink to alice, got %v", rows)
	}
	// esc clears and closes.
	u, _ = m.Update(testKey("esc"))
	m = u.(Model)
	if m.gridFiltering {
		t.Fatal("esc must close prompt")
	}
	if rows := m.rowTable.Rows(); len(rows) != 2 {
		t.Fatalf("esc must restore rows, got %v", rows)
	}
}
