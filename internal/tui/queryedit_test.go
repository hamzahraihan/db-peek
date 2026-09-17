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

func TestEditorBlockDeleteAndComment(t *testing.T) {
	e := NewEditor()
	e.SetText("SELECT 1\nFROM foo\nWHERE x")
	e.CurLine = 0
	e.ExtendSelectionTo(1)
	lo, hi, active := e.SelectedRange()
	if !active || lo != 0 || hi != 1 {
		t.Fatalf("want active 0-1, got %d-%d active=%v", lo, hi, active)
	}
	if got := e.SelectionText(); got != "SELECT 1\nFROM foo" {
		t.Fatalf("selection text, got %q", got)
	}
	e.ToggleCommentRange()
	if e.Lines[0] != "-- SELECT 1" || e.Lines[1] != "-- FROM foo" {
		t.Fatalf("commented, got %q", e.Lines)
	}
	e.ToggleCommentRange()
	if e.Lines[0] != "SELECT 1" || e.Lines[1] != "FROM foo" {
		t.Fatalf("uncommented, got %q", e.Lines)
	}
	e.DeleteRange()
	if got := e.Text(); got != "WHERE x" {
		t.Fatalf("after delete, got %q", got)
	}
	if _, _, active := e.SelectedRange(); active {
		t.Fatal("delete must clear selection")
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
