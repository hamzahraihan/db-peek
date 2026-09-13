package tui

import (
	"reflect"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestEditorInsertAndText(t *testing.T) {
	e := NewEditor()
	for _, r := range "select 1" {
		e.Insert(r)
	}
	if got := e.Text(); got != "select 1" {
		t.Fatalf("want %q got %q", "select 1", got)
	}
	e.Newline()
	e.Insert('x')
	if got := e.Text(); got != "select 1\nx" {
		t.Fatalf("want newline split, got %q", got)
	}
	e.MoveUp()
	e.MoveDown()
	e.Home()
	e.Backspace() // at col 0 joins lines
	if got := e.Text(); got != "select 1x" {
		t.Fatalf("want join, got %q", got)
	}
}

func TestHighlightSQLKeywords(t *testing.T) {
	cells := HighlightSQL("SELECT * FROM users -- hi\nWHERE id = 1")
	flat := ""
	for i, ln := range cells {
		if i > 0 {
			flat += "\n"
		}
		for _, c := range ln {
			flat += c.Text
		}
	}
	if flat != "SELECT * FROM users -- hi\nWHERE id = 1" {
		t.Fatalf("highlight must preserve source, got %q", flat)
	}
	foundKw, foundPlain := false, false
	var kwStyle, plainStyle lipgloss.Style
	for _, c := range cells[0] {
		if c.Text == "SELECT" {
			kwStyle, foundKw = c.Style, true
		}
		if c.Text == "users" {
			plainStyle, foundPlain = c.Style, true
		}
	}
	if !foundKw {
		t.Fatal("SELECT token missing")
	}
	if !foundPlain {
		t.Fatal("users token missing")
	}
	if !reflect.DeepEqual(kwStyle, sqlKeyword) {
		t.Fatal("SELECT must use keyword style")
	}
	if reflect.DeepEqual(kwStyle, plainStyle) {
		t.Fatal("keyword must differ in style from identifier")
	}
}
