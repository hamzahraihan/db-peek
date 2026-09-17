package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	tea "github.com/charmbracelet/bubbletea"
)

func keyMsg(k string) tea.KeyMsg {
	if k == "tab" || k == "enter" {
		return tea.KeyMsg{Type: tea.KeyTab}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

func TestCompleteWordBasic(t *testing.T) {
	prefix, start := completeWord("SELECT ord", 10)
	if prefix != "ord" || start != 7 {
		t.Fatalf("got %q,%d", prefix, start)
	}
}

func TestCompleteDotContext(t *testing.T) {
	prefix, start := completeWord("SELECT orders.", 14)
	if prefix != "" || start != 14 {
		t.Fatalf("dot should reset prefix, got %q,%d", prefix, start)
	}
}

func TestCompleteRanking(t *testing.T) {
	tables := []string{"orders", "customers"}
	cols := map[string][]string{"orders": {"id", "status"}}
	got := completeCandidates("ord", "orders", "SELECT ord", tables, cols, []string{"id", "status"})
	if len(got) == 0 || got[0].Text != "orders" {
		t.Fatalf("tables first, got %+v", got)
	}
}

func TestCompleteDotColumns(t *testing.T) {
	tables := []string{"orders", "customers"}
	cols := map[string][]string{"orders": {"id", "status"}}
	got := completeCandidates("", "orders", "SELECT orders.", tables, cols, []string{"id"})
	if len(got) != 2 {
		t.Fatalf("dot should list orders columns, got %+v", got)
	}
}

func TestApplyCompletion(t *testing.T) {
	line, col := applyCompletion("SELECT ord", 10, completeItem{Text: "orders"})
	if line != "SELECT orders" || col != 13 {
		t.Fatalf("got %q,%d", line, col)
	}
}

func TestQueryEditorLineCountWithPopup(t *testing.T) {
	m := New("", nil)
	m.width, m.height = 120, 40
	m.focusDetail, m.tab, m.queryFocus = true, 3, 0
	m.editor.SetText("SELECT ord")
	m.editor.CurLine, m.editor.CurCol = 0, 10
	m.explorer = fixtureExplorer()
	m.table = "orders"
	m.refreshCompletion()
	if !m.showComplete {
		t.Fatalf("popup should open for 'ord'")
	}
	lines := strings.Split(m.queryEditorView(), "\n")
	if len(lines) != queryEditorH {
		t.Fatalf("editor must stay %d lines, got %d", queryEditorH, len(lines))
	}
}

func TestCompleteAcceptAndEsc(t *testing.T) {
	m := New("", nil)
	m.width, m.height = 120, 40
	m.focusDetail, m.tab, m.queryFocus = true, 3, 0
	m.explorer = fixtureExplorer()
	m.table = "orders"
	m.editor.SetText("SELECT ord")
	m.editor.CurLine, m.editor.CurCol = 0, 10
	m.refreshCompletion()
	if !m.showComplete {
		t.Fatalf("popup should be open")
	}
	mm, _ := m.queryKeys(keyMsg("tab"), "tab")
	m = mm.(Model)
	if got := m.editor.Text(); got != "SELECT orders" {
		t.Fatalf("tab should accept, got %q", got)
	}
	if m.showComplete {
		t.Fatalf("popup should close after accept")
	}
}

func queryTabModel() Model {
	m := New("", nil)
	m.screen = screenBrowse
	m.width, m.height = 120, 40
	m.focusDetail, m.tab, m.queryFocus = true, 3, 0
	m.explorer = fixtureExplorer()
	m.table = "orders"
	return m
}

// Tab with the popup open must accept the suggestion, not switch panes.
func TestUpdateTabAcceptsPopup(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("SELECT ord")
	m.editor.CurLine, m.editor.CurCol = 0, 10
	m.refreshCompletion()
	if !m.showComplete {
		t.Fatalf("popup should be open")
	}
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = u.(Model)
	if got := m.editor.Text(); got != "SELECT orders" {
		t.Fatalf("tab through Update should accept, got %q", got)
	}
	if !m.focusDetail {
		t.Fatalf("tab must not switch panes while completing")
	}
}

// Tab while editing with no popup inserts two spaces, staying on the pane.
func TestUpdateTabIndentsWhileEditing(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("SELECT 1")
	m.editor.CurLine, m.editor.CurCol = 0, 7
	m.refreshCompletion()
	if m.showComplete {
		t.Fatalf("no popup expected for 'SELECT 1'")
	}
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = u.(Model)
	if got := m.editor.Text(); got != "SELECT   1" {
		t.Fatalf("tab should indent, got %q", got)
	}
	if !m.focusDetail {
		t.Fatalf("tab must not switch panes while editing")
	}
}

// Tab outside the query editor still switches panes.
func TestUpdateTabSwitchesPanesOutsideEditor(t *testing.T) {
	m := queryTabModel()
	m.tab, m.queryFocus = 2, 0 // rows tab: not typing a query
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = u.(Model)
	if m.focusDetail {
		t.Fatalf("tab outside the editor must switch panes")
	}
}

// The popup renders as a bordered box and the editor stays 8 lines with
// no line overflowing the pane — at any cursor row, including the bottom
// (where the box flips above the cursor).
func TestPopupBoxChromeAndBudget(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	for _, curLine := range []int{0, 3, 7} {
		m := queryTabModel()
		m.editor.SetText("SELECT ord\nSELECT ord\nSELECT ord\nSELECT ord\nSELECT ord\nSELECT ord\nSELECT ord\nSELECT ord")
		m.editor.CurLine, m.editor.CurCol = curLine, 10
		m.refreshCompletion()
		if !m.showComplete {
			t.Fatalf("line %d: popup should be open", curLine)
		}
		out := m.queryEditorView()
		if !strings.Contains(out, "╭") || !strings.Contains(out, "╰") {
			t.Fatalf("line %d: popup must have a rounded border, got:\n%s", curLine, out)
		}
		lines := strings.Split(out, "\n")
		if len(lines) != queryEditorH {
			t.Fatalf("line %d: editor must stay %d lines, got %d", curLine, queryEditorH, len(lines))
		}
		if w := m.paneInnerW(); w > 0 {
			for _, ln := range lines {
				if lipgloss.Width(ln) > w {
					t.Fatalf("line %d: overflow %q (width %d > %d)", curLine, ln, lipgloss.Width(ln), w)
				}
			}
		}
	}
}

// Closed popup renders no border.
func TestPopupClosedHasNoBorder(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("SELECT 1")
	out := m.queryEditorView()
	if strings.Contains(out, "╭") {
		t.Fatalf("closed popup must not render a border:\n%s", out)
	}
}

// Wheel-scrolling the editor viewport past the cursor (OffY > CurLine)
// must not panic the popup overlay: placement clamps to the visible rows.
func TestPopupScrolledViewportNoPanic(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("SELECT ord")
	m.editor.CurLine, m.editor.CurCol = 0, 10
	m.refreshCompletion()
	if !m.showComplete {
		t.Fatalf("popup should be open")
	}
	m.editor.OffY = 2 // cursor scrolled off-screen, as mouse-wheel allows
	lines := strings.Split(m.queryEditorView(), "\n")
	if len(lines) != queryEditorH {
		t.Fatalf("editor must stay %d lines, got %d", queryEditorH, len(lines))
	}
}
