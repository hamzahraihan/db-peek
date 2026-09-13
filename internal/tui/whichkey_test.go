package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = u.(Model)
	if !m.showHelp {
		t.Fatal("? must open help")
	}
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = u.(Model)
	if m.showHelp {
		t.Fatal("esc must close help")
	}
}
