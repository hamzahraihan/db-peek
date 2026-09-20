package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func dragTestModel() Model {
	m := queryTabModel()
	m.editor.SetText("SELECT 1\nFROM foo\nWHERE x")
	m.width, m.height = 120, 40
	m.resizeBrowse()
	return m
}

func testRelease(x, y int) tea.MouseReleaseMsg {
	return tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft}
}

// Press line 0, drag to line 2, release: block 0-2 active + auto-copied.
func TestMouseDragSelectsLineBlock(t *testing.T) {
	var got string
	calls := 0
	old := clipboardWriteAll
	clipboardWriteAll = func(s string) error { got = s; calls++; return nil }
	defer func() { clipboardWriteAll = old }()

	m := dragTestModel()
	x := m.paneX() + 3
	u, _ := m.Update(testClick(x, queryEditorTop()))
	m = u.(Model)
	u, _ = m.Update(testMotion(x, queryEditorTop()+2))
	m = u.(Model)
	lo, hi, active := m.editor.SelectedRange()
	if !active || lo != 0 || hi != 2 {
		t.Fatalf("drag must select 0-2, got %d-%d active=%v", lo, hi, active)
	}
	u, _ = m.Update(testRelease(x, queryEditorTop()+2))
	m = u.(Model)
	if calls != 1 {
		t.Fatalf("release must auto-copy once, got %d calls", calls)
	}
	if got != "SELECT 1\nFROM foo\nWHERE x" {
		t.Fatalf("auto-copy must carry block, got %q", got)
	}
	if m.status == "" {
		t.Fatal("status must report the copy")
	}
}

// Press + release on the same line: no selection, no clipboard spam.
func TestClickWithoutDragNoAutoCopy(t *testing.T) {
	calls := 0
	old := clipboardWriteAll
	clipboardWriteAll = func(string) error { calls++; return nil }
	defer func() { clipboardWriteAll = old }()

	m := dragTestModel()
	x := m.paneX() + 3
	u, _ := m.Update(testClick(x, queryEditorTop()))
	m = u.(Model)
	u, _ = m.Update(testRelease(x, queryEditorTop()))
	m = u.(Model)
	if _, _, active := m.editor.SelectedRange(); active {
		t.Fatal("plain click must not leave a selection")
	}
	if calls != 0 {
		t.Fatalf("plain click must not auto-copy, got %d calls", calls)
	}
}

// Reverse drag (line 2 -> line 0) selects the same block.
func TestMouseDragReverseSelects(t *testing.T) {
	m := dragTestModel()
	x := m.paneX() + 3
	u, _ := m.Update(testClick(x, queryEditorTop()+2))
	m = u.(Model)
	u, _ = m.Update(testMotion(x, queryEditorTop()))
	m = u.(Model)
	lo, hi, active := m.editor.SelectedRange()
	if !active || lo != 0 || hi != 2 {
		t.Fatalf("reverse drag must select 0-2, got %d-%d active=%v", lo, hi, active)
	}
}

// Shift+Left/Right selects precise rune spans on one line.
func TestShiftLeftRightSelectsChars(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("SELECT ord")
	m.editor.CurLine, m.editor.CurCol = 0, 10
	u, _ := m.queryKeys(testKey("shift+left"), "shift+left")
	m = u.(Model)
	if got := m.editor.SelectionText(); got != "d" {
		t.Fatalf("shift+left must select %q, got %q", "d", got)
	}
	u, _ = m.queryKeys(testKey("shift+left"), "shift+left")
	m = u.(Model)
	u, _ = m.queryKeys(testKey("shift+left"), "shift+left")
	m = u.(Model)
	if got := m.editor.SelectionText(); got != "ord" {
		t.Fatalf("3x shift+left must select %q, got %q", "ord", got)
	}
	if m.editor.CurCol != 7 {
		t.Fatalf("cursor must sit at 7, got %d", m.editor.CurCol)
	}
	u, _ = m.queryKeys(testKey("shift+right"), "shift+right")
	m = u.(Model)
	if got := m.editor.SelectionText(); got != "rd" {
		t.Fatalf("shift+right must shrink to %q, got %q", "rd", got)
	}
}

// Shift+Left at (0,0) must not leave a selection.
func TestShiftLeftAtStartNoSelection(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("SELECT 1")
	m.editor.CurLine, m.editor.CurCol = 0, 0
	u, _ := m.queryKeys(testKey("shift+left"), "shift+left")
	m = u.(Model)
	if _, _, active := m.editor.SelectedRange(); active {
		t.Fatal("shift+left at origin must not select")
	}
}

// Horizontal drag on one line selects the precise span and auto-copies.
func TestMouseDragHorizontalSelectsPartial(t *testing.T) {
	var got string
	calls := 0
	old := clipboardWriteAll
	clipboardWriteAll = func(s string) error { got = s; calls++; return nil }
	defer func() { clipboardWriteAll = old }()

	m := dragTestModel() // "SELECT 1\nFROM foo\nWHERE x"
	m.editor.SetText("SELECT 1")
	// Absolute X for a target column: paneX + 1 (border) + 5 (gutter) + col.
	atCol := func(col int) int { return m.paneX() + 6 + col }
	y := queryEditorTop()
	u, _ := m.Update(testClick(atCol(7), y)) // press on '1'
	m = u.(Model)
	u, _ = m.Update(testMotion(atCol(2), y)) // drag back to 'L'
	m = u.(Model)
	if got := m.editor.SelectionText(); got != "LECT " {
		t.Fatalf("horizontal drag must select %q, got %q", "LECT ", got)
	}
	u, _ = m.Update(testRelease(atCol(2), y))
	m = u.(Model)
	if calls != 1 || got != "LECT " {
		t.Fatalf("release must auto-copy %q once, got %q x%d", "LECT ", got, calls)
	}
}

// Shift+Left across a line boundary selects precisely, not whole lines.
func TestShiftLeftWrapAcrossLines(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("AB\nCD")
	m.editor.CurLine, m.editor.CurCol = 1, 0
	u, _ := m.queryKeys(testKey("shift+left"), "shift+left")
	m = u.(Model)
	if m.editor.CurLine != 0 || m.editor.CurCol != 2 {
		t.Fatalf("cursor must wrap to 0:2, got %d:%d", m.editor.CurLine, m.editor.CurCol)
	}
	if got := m.editor.SelectionText(); got != "\n" {
		t.Fatalf("wrapped selection must be newline only, got %q", got)
	}
}

// A character-precise selection visibly changes the editor render.
func TestCharSelectionRendersHighlighted(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("SELECT ord")
	m.editor.CurLine, m.editor.CurCol = 0, 10
	plain := m.queryEditorView()
	u, _ := m.queryKeys(testKey("shift+left"), "shift+left")
	m = u.(Model)
	if m.queryEditorView() == plain {
		t.Fatal("char selection must change the editor render")
	}
}

// Ctrl+S copies a partial selection verbatim.
func TestCtrlSCopiesPartialSelection(t *testing.T) {
	var got string
	old := clipboardWriteAll
	clipboardWriteAll = func(s string) error { got = s; return nil }
	defer func() { clipboardWriteAll = old }()

	m := queryTabModel()
	m.editor.SetText("SELECT ord")
	m.editor.CurLine, m.editor.CurCol = 0, 10
	u, _ := m.queryKeys(testKey("shift+left"), "shift+left")
	m = u.(Model)
	u, _ = m.queryKeys(testKey("shift+left"), "shift+left")
	m = u.(Model)
	u, _ = m.queryKeys(testKey("ctrl+s"), "ctrl+s")
	m = u.(Model)
	if got != "rd" {
		t.Fatalf("ctrl+s must copy partial %q, got %q", "rd", got)
	}
}
