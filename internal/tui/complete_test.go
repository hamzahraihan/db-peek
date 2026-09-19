package tui

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

// testKey builds a v2 KeyPressMsg whose String() yields name: single
// printable runes (letters, digits, "?", "/", " ") carry Text, while
// control combos and special keys use Code+Mod.
func testKey(name string) tea.KeyPressMsg {
	switch name {
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "delete":
		return tea.KeyPressMsg{Code: tea.KeyDelete}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "shift+up":
		return tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift}
	case "shift+down":
		return tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift}
	case "f5":
		return tea.KeyPressMsg{Code: tea.KeyF5}
	}
	if strings.HasPrefix(name, "ctrl+") && len(name) == len("ctrl+")+1 {
		return tea.KeyPressMsg{Code: rune(name[len("ctrl+")]), Mod: tea.ModCtrl}
	}
	if r := []rune(name); len(r) == 1 {
		return tea.KeyPressMsg{Code: r[0], Text: name}
	}
	panic("testKey: unknown key name " + name)
}

// testClick builds a v2 left-press message at (x, y).
func testClick(x, y int) tea.MouseClickMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

// testWheelDown builds a v2 wheel-down message.
func testWheelDown() tea.MouseWheelMsg {
	return tea.MouseWheelMsg{Button: tea.MouseWheelDown}
}

// testMotion builds a v2 mouse-motion message at (x, y).
func testMotion(x, y int) tea.MouseMotionMsg {
	return tea.MouseMotionMsg{X: x, Y: y}
}

// render256 downsamples full-fidelity lipgloss v2 output to ANSI256 for
// assertions on quantized color codes. v1 downsampled inside Render via
// the global color profile; v2 only downsamples at print time.
func render256(s string) string {
	var buf bytes.Buffer
	w := &colorprofile.Writer{Forward: &buf, Profile: colorprofile.ANSI256}
	fmt.Fprint(w, s)
	return buf.String()
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
	mm, _ := m.queryKeys(testKey("tab"), "tab")
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
	u, _ := m.Update(testKey("tab"))
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
	u, _ := m.Update(testKey("tab"))
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
	u, _ := m.Update(testKey("tab"))
	m = u.(Model)
	if m.focusDetail {
		t.Fatalf("tab outside the editor must switch panes")
	}
}

// The popup renders as a bordered box and the editor stays 8 lines with
// no line overflowing the pane — at any cursor row, including the bottom
// (where the box flips above the cursor).
func TestPopupBoxChromeAndBudget(t *testing.T) {
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

// The open popup must not eat the editor text beside it: gutter and code
// left of the box and the line tail right of it survive the splice.
func TestPopupKeepsEditorText(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("SELECT ord FROM t WHERE marker_xyz_123 = 1")
	m.editor.CurLine, m.editor.CurCol = 0, 10
	m.refreshCompletion()
	if !m.showComplete {
		t.Fatal("popup should be open")
	}
	line := ansi.Strip(strings.Split(m.queryEditorView(), "\n")[0])
	if !strings.Contains(line, "SELECT") {
		t.Fatalf("editor head left of the popup must survive, got:\n%q", line)
	}
	if !strings.Contains(line, "marker_xyz_123 = 1") {
		t.Fatalf("editor tail right of the popup must survive, got:\n%q", line)
	}
}
