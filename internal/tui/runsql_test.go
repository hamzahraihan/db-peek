package tui

import (
	"testing"
)

func TestRunSQLPrefersSelection(t *testing.T) {
	e := NewEditor()
	e.SetText("SELECT 1;\nSELECT 2;")
	e.CurLine, e.CurCol = 1, 0
	e.ExtendSelectionToPos(1, 8) // selects "SELECT 2"
	if got := e.RunSQL(); got != "SELECT 2" {
		t.Fatalf("selection must win, got %q", got)
	}
}

func TestRunSQLStatementAtCursor(t *testing.T) {
	e := NewEditor()
	e.SetText("SELECT 1;\n\nSELECT 2 WHERE x = ';';\nSELECT 3;")
	e.CurLine, e.CurCol = 2, 4 // inside the quoted-semicolon statement
	if got := e.RunSQL(); got != "SELECT 2 WHERE x = ';'" {
		t.Fatalf("must run statement at cursor, got %q", got)
	}
	e.CurLine, e.CurCol = 0, 0
	if got := e.RunSQL(); got != "SELECT 1" {
		t.Fatalf("must run first statement, got %q", got)
	}
}

func TestRunSQLSingleStatementIsBuffer(t *testing.T) {
	e := NewEditor()
	e.SetText("SELECT 1")
	e.CurLine, e.CurCol = 0, 3
	if got := e.RunSQL(); got != "SELECT 1" {
		t.Fatalf("single statement must equal buffer, got %q", got)
	}
}
