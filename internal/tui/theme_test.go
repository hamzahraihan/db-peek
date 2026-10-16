package tui

import (
	"testing"
)

func TestThemesExist(t *testing.T) {
	required := []string{"default", "kanagawa", "catppuccin", "onedark", "nightowl"}
	for _, req := range required {
		th := GetTheme(req)
		if th.ID != req {
			t.Fatalf("theme %q not found, got %+v", req, th)
		}
		if th.Name == "" || th.Description == "" {
			t.Fatalf("theme %q must have non-empty name and description", req)
		}
		if th.Palette.Primary == "" || th.Palette.Text == "" || th.Palette.BorderActive == "" {
			t.Fatalf("theme %q has incomplete palette: %+v", req, th.Palette)
		}
	}
}

func TestCanonicalThemeID(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"kanagawa", "kanagawa"},
		{"Kanagawa", "kanagawa"},
		{"  KANAGAWA  ", "kanagawa"},
		{"catpuccin", "catppuccin"}, // user spelling
		{"catppuccin", "catppuccin"},
		{"Catppuccin Mocha", "catppuccin"},
		{"mocha", "catppuccin"},
		{"one dark", "onedark"},
		{"One-Dark", "onedark"},
		{"onedark", "onedark"},
		{"atom", "onedark"},
		{"nightowl", "nightowl"},
		{"Night Owl", "nightowl"},
		{"night-owl", "nightowl"},
		{"default", "default"},
		{"classic", "default"},
		{"", "default"},
		{"some-random-theme", "default"},
	}

	for _, c := range cases {
		got := CanonicalThemeID(c.input)
		if got != c.want {
			t.Errorf("CanonicalThemeID(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestApplyTheme(t *testing.T) {
	t.Cleanup(func() {
		ApplyTheme("default")
	})

	th := ApplyTheme("kanagawa")
	if th.ID != "kanagawa" {
		t.Fatalf("expected kanagawa, got %q", th.ID)
	}
	if CurrentTheme.ID != "kanagawa" {
		t.Fatalf("CurrentTheme.ID expected kanagawa, got %q", CurrentTheme.ID)
	}
	if CurrentTheme.Palette.BorderActive != "#E6C384" {
		t.Fatalf("expected Kanagawa border active #E6C384, got %q", CurrentTheme.Palette.BorderActive)
	}

	th = ApplyTheme("catpuccin")
	if th.ID != "catppuccin" {
		t.Fatalf("expected catppuccin, got %q", th.ID)
	}
	if CurrentTheme.Palette.BorderActive != "#F9E2AF" {
		t.Fatalf("expected Catppuccin border active #F9E2AF, got %q", CurrentTheme.Palette.BorderActive)
	}

	th = ApplyTheme("one dark")
	if th.ID != "onedark" {
		t.Fatalf("expected onedark, got %q", th.ID)
	}

	th = ApplyTheme("nightowl")
	if th.ID != "nightowl" {
		t.Fatalf("expected nightowl, got %q", th.ID)
	}

	th = ApplyTheme("default")
	if th.ID != "default" {
		t.Fatalf("expected default, got %q", th.ID)
	}
	if CurrentTheme.Palette.BorderActive != "#EAB308" {
		t.Fatalf("expected default border active #EAB308, got %q", CurrentTheme.Palette.BorderActive)
	}
}

func TestDefaultPaletteMatchesOriginal(t *testing.T) {
	if PaletteDefault.Primary != "#EAB308" {
		t.Fatalf("default primary must be #EAB308, got %s", PaletteDefault.Primary)
	}
	if PaletteDefault.ActiveTabBg != "#005FD7" {
		t.Fatalf("default active tab bg must be #005FD7, got %s", PaletteDefault.ActiveTabBg)
	}
	if PaletteDefault.ExplorerSelBg != "#CA8A04" {
		t.Fatalf("default explorer sel bg must be #CA8A04, got %s", PaletteDefault.ExplorerSelBg)
	}
}
