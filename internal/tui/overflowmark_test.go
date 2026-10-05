package tui

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestRenderKeyPairsMarksOverflow(t *testing.T) {
	pairs := [][2]string{{"a", "alpha"}, {"b", "beta"}, {"c", "gamma"}}
	full := renderKeyPairs(pairs, 0)
	if w := lipgloss.Width(full); w <= 0 {
		t.Fatalf("full render must have width, got %d", w)
	}
	// Narrow enough to drop at least one pair but fit the rest + marker.
	narrow := renderKeyPairs(pairs, 20)
	if narrow == full {
		t.Skip("width already fits, marker n/a")
	}
	if !containsStr(narrow, "…") {
		t.Fatalf("dropped pairs must show overflow marker, got %q", narrow)
	}
	if w := lipgloss.Width(narrow); w > 20 {
		t.Fatalf("marked line must still fit width, got %d > 20: %q", w, narrow)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
