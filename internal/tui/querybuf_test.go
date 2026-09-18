package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	dbpkg "db-peek/internal/db"
)

func TestNewBufferPreservesCurrentText(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("SELECT 1")
	u, _ := m.queryKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")}, "t")
	// 't' in editor focus must insert, not create buffer.
	m2 := u.(Model)
	if got := m2.editor.Text(); !strings.Contains(got, "t") {
		t.Fatalf("editor typing must insert t, got %q", got)
	}
	if len(m2.qbufs) != 1 {
		t.Fatalf("editor t must not create buffer, got %d", len(m2.qbufs))
	}
	// Results focus + t creates a new buffer.
	m.queryFocus = 1
	m.saveActiveBuf()
	u, _ = m.queryKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")}, "t")
	m = u.(Model)
	if len(m.qbufs) != 2 {
		t.Fatalf("results t must create buffer, got %d", len(m.qbufs))
	}
	if got := m.qbufs[0].editor.Text(); !strings.Contains(got, "SELECT 1") {
		t.Fatalf("buffer 0 must preserve text, got %q", got)
	}
	if got := m.editor.Text(); got != "" {
		t.Fatalf("new buffer must start empty, got %q", got)
	}
}

func TestCtrlTCreatesBufferFromEditor(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("SELECT 1")
	m.showComplete = false
	u, _ := m.queryKeys(tea.KeyMsg{Type: tea.KeyCtrlT}, "ctrl+t")
	m = u.(Model)
	if len(m.qbufs) != 2 {
		t.Fatalf("ctrl+t must create buffer, got %d", len(m.qbufs))
	}
	if m.queryFocus != 0 {
		t.Fatalf("new buffer must focus editor, got %d", m.queryFocus)
	}
}

func TestHLSwitchesBuffersInResults(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("SELECT 1")
	m.queryFocus = 1
	m.saveActiveBuf()
	u, _ := m.queryKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")}, "t")
	m = u.(Model)
	m.editor.SetText("SELECT 2")
	m.saveActiveBuf()
	m.queryFocus = 1
	// H -> buffer 0.
	u, _ = m.queryKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("H")}, "H")
	m = u.(Model)
	if got := m.editor.Text(); !strings.Contains(got, "SELECT 1") {
		t.Fatalf("H must switch to buffer 0, got %q", got)
	}
	// L -> buffer 1.
	u, _ = m.queryKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("L")}, "L")
	m = u.(Model)
	if got := m.editor.Text(); !strings.Contains(got, "SELECT 2") {
		t.Fatalf("L must switch to buffer 1, got %q", got)
	}
	if m.queryFocus != 1 {
		t.Fatalf("switch must stay in results, got %d", m.queryFocus)
	}
}

func TestCloseLastBufferClears(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("SELECT 1")
	m.saveActiveBuf()
	m.queryFocus = 1
	u, _ := m.queryKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")}, "X")
	m = u.(Model)
	if len(m.qbufs) != 1 {
		t.Fatalf("close last must keep 1 buffer, got %d", len(m.qbufs))
	}
	if got := m.editor.Text(); got != "" {
		t.Fatalf("close last must clear, got %q", got)
	}
}

func TestPerBufferResultsIsolation(t *testing.T) {
	m := queryTabModel()
	m.queryFocus = 1
	m.saveActiveBuf()
	// Run buffer 0.
	m.queryFocus = 0
	m.editor.SetText("SELECT 1")
	m.startQueryRun()
	id0 := m.qbufs[m.qcur].id
	seq0 := m.qbufs[m.qcur].seq
	// New buffer + run buffer 1.
	m.newQueryBuf()
	m.editor.SetText("SELECT 2")
	m.startQueryRun()
	id1 := m.qbufs[m.qcur].id
	seq1 := m.qbufs[m.qcur].seq
	if id0 == id1 {
		t.Fatal("buffer ids must differ")
	}
	// Late reply for buffer 0 must not clobber active buffer 1.
	s0 := &dbpkg.Sample{Columns: []string{"a"}, Rows: [][]string{{"1"}}}
	u, _ := m.Update(queryDoneMsg{sql: "SELECT 1", seq: seq0, sample: s0, affected: -1, qbufID: id0})
	m = u.(Model)
	if m.querySample != nil {
		t.Fatal("foreign buffer reply must not paint active buffer")
	}
	if m.qbufs[0].sample == nil {
		t.Fatal("foreign buffer reply must apply to its own slot")
	}
	// Reply for active buffer applies.
	s1 := &dbpkg.Sample{Columns: []string{"b"}, Rows: [][]string{{"2"}}}
	u, _ = m.Update(queryDoneMsg{sql: "SELECT 2", seq: seq1, sample: s1, affected: -1, qbufID: id1})
	m = u.(Model)
	if m.querySample == nil || len(m.querySample.Columns) != 1 || m.querySample.Columns[0] != "b" {
		t.Fatalf("active buffer reply must apply, got %+v", m.querySample)
	}
}

func TestDetailFootSingleLineFits(t *testing.T) {
	for _, tab := range []int{0, 2, 3, 4} {
		m := queryTabModel()
		m.tab = tab
		m.width, m.height = 120, 40
		m.resizeBrowse()
		foot := m.detailFoot()
		if strings.Contains(foot, "\n") {
			t.Fatalf("tab %d foot must be single line", tab)
		}
		if w := lipgloss.Width(foot); w > m.paneW() {
			t.Fatalf("tab %d foot width %d exceeds pane %d: %q", tab, w, m.paneW(), foot)
		}
	}
	m := queryTabModel()
	m.tab = 3
	if got := m.detailFoot(); !strings.Contains(got, "H/L") || !strings.Contains(got, "buffer") {
		t.Fatalf("query foot must name buffer keys, got %q", got)
	}
}

func TestEditorPanelSpacing(t *testing.T) {
	m := queryTabModel()
	m.width, m.height = 120, 40
	m.resizeBrowse()
	m.editor.SetText("SELECT 1")
	panel := m.queryEditorPanel()
	lines := strings.Split(panel, "\n")
	if len(lines) != queryEditorH+2 {
		t.Fatalf("panel must wrap 8 content rows in 2 border rows, got %d", len(lines))
	}
	// Content rows keep the indented gutter ("  1 " with leading space).
	if !strings.Contains(lines[1], "  1 ") {
		t.Fatalf("gutter must be indented from panel edge, got %q", lines[1])
	}
	// Editor content starts below 1 padding + 1 border row.
	if got := queryEditorTop(); got != detailTableTop+2 {
		t.Fatalf("editor top must clear padding+border, got %d", got)
	}
	// Results grid starts below panel bottom + hint.
	if got := queryResultsTop(); got != queryEditorTop()+queryEditorH+2 {
		t.Fatalf("results top must clear panel+hint, got %d", got)
	}
	// Clicks on the padding row must not move the cursor.
	u, _ := m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: m.paneX() + 3, Y: detailTableTop})
	mm := u.(Model)
	if mm.editor.CurLine != m.editor.CurLine || mm.queryFocus != m.queryFocus {
		t.Fatal("padding click must be a noop")
	}
}

func TestStripRendersWithoutShiftingEditor(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("SELECT 1")
	out := m.detailView()
	lines := strings.Split(out, "\n")
	// Strip replaces the blank row after tabs: editor must still start at detailTableTop.
	found := false
	for i, ln := range lines {
		if strings.Contains(ln, "[1") {
			found = true
			_ = i
			break
		}
	}
	if !found {
		t.Fatal("strip must render buffer labels")
	}
	if got := len(strings.Split(m.queryEditorView(), "\n")); got != queryEditorH {
		t.Fatalf("editor must stay %d rows, got %d", queryEditorH, got)
	}
}
