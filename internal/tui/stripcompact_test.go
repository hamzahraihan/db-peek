package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestStripCompactMarksTruncation(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("SELECT a very long query text here")
	m.saveActiveBuf()
	m.newQueryBuf()
	m.editor.SetText("SELECT another long query text here")
	m.saveActiveBuf()
	out := m.queryStripView(20)
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "[1]") || !strings.Contains(plain, "[2]") {
		t.Fatalf("narrow strip must fall back to numbers, got %q", plain)
	}
	if !strings.Contains(plain, "…") {
		t.Fatalf("compact fallback must mark truncation, got %q", plain)
	}
}

func TestStripActiveChipUnderlined(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("SELECT 1")
	m.saveActiveBuf()
	out := m.queryStripView(120)
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "[1 SELECT 1]") {
		t.Fatalf("strip must keep title, got %q", plain)
	}
	if !strings.Contains(out, ";4m") {
		t.Fatalf("active chip must be underline-styled, got %q", out)
	}
}
