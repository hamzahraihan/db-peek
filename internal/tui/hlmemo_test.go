package tui

import (
	"strings"
	"testing"
)

// Highlighting is a full-buffer lex pass, but the editor draws 8 rows.
// The cache must return identical output for the same text and must
// still follow edits.
func TestQueryEditorViewMemoizesHighlighting(t *testing.T) {
	m := browseModel(t)
	m.screen = screenBrowse
	m.focusDetail = true
	m.tab = 3
	m.queryFocus = 0
	m.editor.SetText(strings.Repeat("select id from users\n", 200))

	first := m.queryEditorView()
	if m.hlMemo.text != m.editor.Text() {
		t.Fatalf("the memo must retain the highlighted text, got %q", m.hlMemo.text)
	}
	cells := m.hlMemo.cells
	if second := m.queryEditorView(); first != second {
		t.Fatal("repeated renders of an unchanged buffer must be identical")
	}
	if &m.hlMemo.cells[0] != &cells[0] {
		t.Fatal("an unchanged buffer must reuse the cached cells, not re-lex")
	}

	m.editor.SetText("select 1")
	if third := m.queryEditorView(); third == first {
		t.Fatal("an edited buffer must re-highlight")
	}
	if m.hlMemo.text != "select 1" {
		t.Fatalf("the memo must follow the edit, got %q", m.hlMemo.text)
	}
}
