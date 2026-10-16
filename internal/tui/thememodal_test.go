package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestThemeModalOpenAndCancelWithEsc(t *testing.T) {
	t.Cleanup(func() { ApplyTheme("default") })
	m := browseModel(t)
	m.applyTheme("default")

	// Press 'T' to open modal
	u, _ := m.Update(testKey("T"))
	m = u.(Model)
	if !m.themeModal {
		t.Fatal("T must open theme modal")
	}
	if m.themeCursor != 0 {
		t.Fatalf("expected initial cursor 0, got %d", m.themeCursor)
	}

	// Press 'down' or 'j' to navigate to Kanagawa
	u, _ = m.Update(testKey("down"))
	m = u.(Model)
	if m.themeCursor != 1 {
		t.Fatalf("down must step cursor to 1, got %d", m.themeCursor)
	}
	// Live preview must have applied Kanagawa
	if CurrentTheme.ID != "kanagawa" {
		t.Fatalf("navigating must live-preview theme, got %s", CurrentTheme.ID)
	}

	// Press 'esc' to cancel
	u, _ = m.Update(testKey("esc"))
	m = u.(Model)
	if m.themeModal {
		t.Fatal("esc must close theme modal")
	}
	// Theme must be restored back to default
	if m.theme != "default" || CurrentTheme.ID != "default" {
		t.Fatalf("esc must restore original theme, got model=%s current=%s", m.theme, CurrentTheme.ID)
	}
}

func TestThemeModalOpenAndConfirmWithEnter(t *testing.T) {
	t.Cleanup(func() { ApplyTheme("default") })
	m := browseModel(t)
	m.applyTheme("default")

	// Press 'T'
	u, _ := m.Update(testKey("T"))
	m = u.(Model)

	// Step to Kanagawa (index 1)
	u, _ = m.Update(testKey("j"))
	m = u.(Model)
	if m.themeCursor != 1 {
		t.Fatalf("j must step cursor to 1, got %d", m.themeCursor)
	}

	// Press 'enter' to confirm
	u, _ = m.Update(testKey("enter"))
	m = u.(Model)
	if m.themeModal {
		t.Fatal("enter must close theme modal")
	}
	if m.theme != "kanagawa" || CurrentTheme.ID != "kanagawa" {
		t.Fatalf("enter must commit theme, got model=%s current=%s", m.theme, CurrentTheme.ID)
	}
	if !strings.Contains(m.status, "Kanagawa") {
		t.Fatalf("status should mention Kanagawa, got %q", m.status)
	}
}

func TestThemeModalOpenWithF3(t *testing.T) {
	t.Cleanup(func() { ApplyTheme("default") })
	m := browseModel(t)

	u, _ := m.Update(testKey("f3"))
	m = u.(Model)
	if !m.themeModal {
		t.Fatal("f3 must open theme modal")
	}

	u, _ = m.Update(testKey("q"))
	m = u.(Model)
	if m.themeModal {
		t.Fatal("q must close theme modal")
	}
}

func TestThemeModalMouseInteraction(t *testing.T) {
	t.Cleanup(func() { ApplyTheme("default") })
	m := browseModel(t)
	m.applyTheme("default")
	m.openThemeModal()

	x, y, w, h := m.themeRect()

	// Clicking outside the box cancels the modal
	u, _ := m.Update(testClick(0, 0))
	m = u.(Model)
	if m.themeModal {
		t.Fatal("click outside must dismiss theme modal")
	}

	// Open again
	m.openThemeModal()

	// Theme rows are at row y+3 + index. Click Catppuccin (index 2)
	catpIdx := 2
	targetY := y + 3 + catpIdx
	targetX := x + 5
	u, _ = m.Update(testClick(targetX, targetY))
	m = u.(Model)
	if m.themeModal {
		t.Fatal("click on theme row must commit and close modal")
	}
	if m.theme != "catppuccin" {
		t.Fatalf("expected catppuccin committed, got %s", m.theme)
	}

	_ = w
	_ = h
}

func TestThemeModalMouseWheel(t *testing.T) {
	t.Cleanup(func() { ApplyTheme("default") })
	m := browseModel(t)
	m.applyTheme("default")
	m.openThemeModal()

	// Scroll down
	u, _ := m.wheel(1)
	m = u.(Model)
	if m.themeCursor != 1 {
		t.Fatalf("wheel down must advance cursor to 1, got %d", m.themeCursor)
	}
	if CurrentTheme.ID != "kanagawa" {
		t.Fatalf("wheel must live-preview theme, got %s", CurrentTheme.ID)
	}

	// Scroll up
	u, _ = m.wheel(-1)
	m = u.(Model)
	if m.themeCursor != 0 {
		t.Fatalf("wheel up must step cursor back to 0, got %d", m.themeCursor)
	}
}

func TestThemeModalTypingGuard(t *testing.T) {
	t.Cleanup(func() { ApplyTheme("default") })
	m := browseModel(t)
	m.filtering = true
	m.filterInput.Focus()

	// Pressing 'T' while filtering must NOT open modal (it types T)
	u, _ := m.Update(tea.KeyPressMsg{Code: 'T', Text: "T"})
	m = u.(Model)
	if m.themeModal {
		t.Fatal("typing T in search box must not open theme modal")
	}
}

func TestThemeModalViewRendersCorrectly(t *testing.T) {
	t.Cleanup(func() { ApplyTheme("default") })
	m := browseModel(t)
	m.openThemeModal()

	view := m.themeModalView()
	if !strings.Contains(view, "Color Schemes") {
		t.Fatal("modal view must show header 'Color Schemes'")
	}
	if !strings.Contains(view, "Kanagawa") || !strings.Contains(view, "Catppuccin") || !strings.Contains(view, "One Dark") || !strings.Contains(view, "Night Owl") {
		t.Fatal("modal view must list all themes")
	}
}
