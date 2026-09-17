# Query Block Ops Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add line-wise block select, delete line/block, copy selection to system clipboard, and `--` comment toggle to the TUI query editor.

**Architecture:** Extend `Editor` with anchor state plus pure line-range helpers; map new keys in `queryKeys`; thread mouse `Shift` through `clickTable`/`clickQuery`; highlight with the existing selection style inside the fixed 8-row editor; reuse the `clipboardWriteAll` seam and file fallback for copy.

**Tech Stack:** Go 1.25, bubbletea v1.3.10 (`tea.KeyMsg`/`tea.MouseMsg`), lipgloss styles in `internal/tui`.

## Global Constraints

- Line-wise selection only — no char-precise ranges.
- Scope is query editor only (`focusDetail && tab == 3 && queryFocus == 0`); results grid, other tabs, CLI `--query` untouched.
- Editor keeps its 8-row budget; `queryResultsTop` mouse math unchanged.
- `Ctrl+C` stays global quit and is never reused for copy.
- Comment prefix is always column 0 (`-- `); toggle is idempotent.
- `Editor.Lines` is never left empty — single-line delete resets to `[""]`.
- Clipboard writes use the existing never-panic path (strip NUL, recover to `db-peek-debug.txt` fallback).
- Key strings follow bubbletea v1.3.10: `shift+up`, `shift+down`, `ctrl+d`, and comment on both `ctrl+/` and `ctrl+_` (terminals report `Ctrl+/` as `0x1F`/`ctrl+_`).

---

### Task 1: Editor selection state and pure ops

**Files:**
- Modify: `internal/tui/queryedit.go`
- Test: `internal/tui/queryedit_test.go`

**Interfaces:**
- Consumes: existing `Editor{Lines, CurLine, CurCol, OffY}`, `min` builtin.
- Produces (used by Tasks 2–4):
  - `func (e *Editor) ClearSelection()`
  - `func (e *Editor) SelectedRange() (lo, hi int, active bool)`
  - `func (e *Editor) ExtendSelectionTo(line int)`
  - `func (e *Editor) DeleteRange()`
  - `func (e *Editor) SelectionText() string`
  - `func (e *Editor) ToggleCommentRange()`
  - `func (e *Editor) ToggleCommentLine()`

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run TestEditorBlockDeleteAndComment -v`
Expected: FAIL with undefined `ExtendSelectionTo` (compile error).

- [ ] **Step 3: Write minimal implementation**

```go
func (e *Editor) ClearSelection() { e.selActive = false }

func (e *Editor) SelectedRange() (int, int, bool) {
	if !e.selActive {
		return 0, 0, false
	}
	lo, hi := e.selAnchor, e.CurLine
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo < 0 {
		lo = 0
	}
	if hi >= len(e.Lines) {
		hi = len(e.Lines) - 1
	}
	return lo, hi, true
}

func (e *Editor) ExtendSelectionTo(line int) {
	if !e.selActive {
		e.selAnchor, e.selActive = e.CurLine, true
	}
	if line < 0 {
		line = 0
	}
	if line >= len(e.Lines) {
		line = len(e.Lines) - 1
	}
	e.CurLine = line
	rs := []rune(e.Lines[e.CurLine])
	if e.CurCol > len(rs) {
		e.CurCol = len(rs)
	}
}

func (e *Editor) SelectionText() string {
	lo, hi, active := e.SelectedRange()
	if !active {
		return ""
	}
	return strings.Join(e.Lines[lo:hi+1], "\n")
}

func (e *Editor) DeleteRange() {
	lo, hi, active := e.SelectedRange()
	if !active {
		return
	}
	e.Lines = append(e.Lines[:lo], e.Lines[hi+1:]...)
	if len(e.Lines) == 0 {
		e.Lines = []string{""}
	}
	e.CurLine = min(lo, len(e.Lines)-1)
	e.CurCol = 0
	e.selActive = false
}

func commentLine(s string) string { return "-- " + s }

func uncommentLine(s string) (string, bool) {
	if strings.HasPrefix(s, "-- ") {
		return s[3:], true
	}
	if strings.HasPrefix(s, "--") {
		return s[2:], true
	}
	return s, false
}

func (e *Editor) ToggleCommentRange() {
	lo, hi, active := e.SelectedRange()
	if !active {
		e.ToggleCommentLine()
		return
	}
	commented := true
	for _, ln := range e.Lines[lo : hi+1] {
		if !strings.HasPrefix(ln, "--") {
			commented = false
			break
		}
	}
	for i := lo; i <= hi; i++ {
		if commented {
			if u, _ := uncommentLine(e.Lines[i]); true {
				e.Lines[i] = u
			}
		} else {
			e.Lines[i] = commentLine(e.Lines[i])
		}
	}
}

func (e *Editor) ToggleCommentLine() {
	u, ok := uncommentLine(e.Lines[e.CurLine])
	if ok || strings.HasPrefix(e.Lines[e.CurLine], "--") {
		e.Lines[e.CurLine] = u
		return
	}
	e.Lines[e.CurLine] = commentLine(e.Lines[e.CurLine])
}
```

Also add fields to the struct:

```go
type Editor struct {
	Lines     []string
	CurLine   int
	CurCol    int // rune offset within line
	OffY      int // first visible line
	selAnchor int
	selActive bool
}
```

Note: the single `commented` scan makes the toggle idempotent — blank lines carry `-- ` after commenting, so re-toggle uncomments the whole block.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run TestEditorBlockDeleteAndComment -v`
Expected: PASS.

- [ ] **Step 5: Run the package suite**

Run: `go test ./internal/tui 2>&1 | tail -5`
Expected: PASS (no regressions).

- [ ] **Step 6: Commit**

```bash
git add internal/tui/queryedit.go internal/tui/queryedit_test.go
git commit -m "feat(tui): add editor block selection and comment helpers"
```

### Task 2: Keyboard wiring in the query editor

**Files:**
- Modify: `internal/tui/update.go`
- Test: `internal/tui/queryblock_test.go` (create)

**Interfaces:**
- Consumes: Task 1 helpers (`ExtendSelectionTo`, `ClearSelection`, `DeleteRange`, `ToggleCommentRange`, `ToggleCommentLine`, `SelectedRange`), `clampEditorScroll`, `refreshCompletion`, `runQuery`/`querySeq`/`loading`/`err` patterns.
- Produces: key behavior used by Tasks 4–5 (selection state observable via `SelectedRange`; `Ctrl+S` selection branch in Task 5).

- [ ] **Step 1: Write the failing test**

```go
func TestShiftUpExtendsAndCtrlDDeletesLine(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("SELECT 1\nFROM foo\nWHERE x")
	m.editor.CurLine, m.editor.CurCol = 2, 0
	m.refreshCompletion()
	m.showComplete = false
	u, _ := m.queryKeys(tea.KeyMsg{Type: tea.KeyShiftUp}, "shift+up")
	m = u.(Model)
	if _, _, active := m.editor.SelectedRange(); !active {
		t.Fatal("shift+up must activate selection")
	}
	u, _ = m.queryKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}, "x")
	m = u.(Model)
	if _, _, active := m.editor.SelectedRange(); active {
		t.Fatal("plain typing must clear selection")
	}
	m.editor.SetText("SELECT 1\nFROM foo")
	m.editor.CurLine = 0
	m.editor.ClearSelection()
	u, _ = m.queryKeys(tea.KeyMsg{Type: tea.KeyCtrlD}, "ctrl+d")
	m = u.(Model)
	if got := m.editor.Text(); got != "FROM foo" {
		t.Fatalf("ctrl+d on single line, got %q", got)
	}
}

func TestCommentToggleBothKeyStrings(t *testing.T) {
	for _, key := range []string{"ctrl+/", "ctrl+_"} {
		m := queryTabModel()
		m.editor.SetText("SELECT 1")
		m.showComplete = false
		u, _ := m.queryKeys(tea.KeyMsg{Type: tea.KeyRunes}, key)
		m = u.(Model)
		if got := m.editor.Text(); got != "-- SELECT 1" {
			t.Fatalf("%s: want commented, got %q", key, got)
		}
		u, _ = m.queryKeys(tea.KeyMsg{Type: tea.KeyRunes}, key)
		m = u.(Model)
		if got := m.editor.Text(); got != "SELECT 1" {
			t.Fatalf("%s: want uncommented, got %q", key, got)
		}
	}
}
```

`queryTabModel` already exists in `complete_test.go`; reuse it, do not redefine it.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run 'TestShiftUpExtendsAndCtrlDDeletesLine|TestCommentToggleBothKeyStrings' -v`
Expected: FAIL (no `shift+up`/`ctrl+d`/`ctrl+/` branches).

- [ ] **Step 3: Write minimal implementation**

In `queryKeys`, editor-focused branch (`m.queryFocus == 0`), inside the `if m.showComplete` switch add `shift+up`/`shift+down` passthrough first (selection moves, popup dismissed — typing a selection is never a completion):

```go
case "shift+up", "shift+down":
	m.showComplete = false
	m.completeItems = nil
	m.completeIdx = 0
	if key == "shift+up" {
		m.editor.ExtendSelectionTo(m.editor.CurLine - 1)
	} else {
		m.editor.ExtendSelectionTo(m.editor.CurLine + 1)
	}
	m.clampEditorScroll()
	return m, nil
```

In the main editor `switch key` (after the `tab` case, before `ctrl+r`), add:

```go
case "shift+up":
	m.editor.ExtendSelectionTo(m.editor.CurLine - 1)
	m.clampEditorScroll()
	m.refreshCompletion()
	return m, nil
case "shift+down":
	m.editor.ExtendSelectionTo(m.editor.CurLine + 1)
	m.clampEditorScroll()
	m.refreshCompletion()
	return m, nil
case "ctrl+d":
	if _, _, active := m.editor.SelectedRange(); active {
		m.editor.DeleteRange()
	} else {
		// Delete current line without clipboard (spec: not a cut).
		ln := m.editor.CurLine
		m.editor.ClearSelection()
		// Reuse DeleteRange via a one-line selection.
		m.editor.selActive = true
		m.editor.selAnchor = ln
		m.editor.CurLine = ln
		m.editor.DeleteRange()
	}
	m.clampEditorScroll()
	m.refreshCompletion()
	return m, nil
case "ctrl+/", "ctrl+_":
	m.editor.ToggleCommentRange()
	m.clampEditorScroll()
	m.refreshCompletion()
	return m, nil
```

And clear selection on plain edits: in the `up`/`down`/`left`/`right`/`home`/`end` cases and the `msg.Type == tea.KeyRunes` insert path, call `m.editor.ClearSelection()` first (except the shift cases above). Example for the rune path:

```go
if msg.Type == tea.KeyRunes {
	m.editor.ClearSelection()
	for _, r := range msg.Runes {
		m.editor.Insert(r)
	}
	m.clampEditorScroll()
	m.refreshCompletion()
	return m, nil
}
```

Same one-line `m.editor.ClearSelection()` addition to `backspace`, `delete`, `enter`, `up`, `down`, `left`, `right`, `home`, `end` cases. `esc` clears selection first:

```go
case "esc":
	if _, _, active := m.editor.SelectedRange(); active {
		m.editor.ClearSelection()
		return m, nil
	}
	m.queryFocus = 1
	return m, nil
```

Note: unexported field access (`selActive`, `selAnchor`) from `update.go` is fine — same package. Prefer adding a small exported helper `DeleteCurrentLine()` on `Editor` in Task 1 if reviewers dislike the inline selection synthesis; behavior is identical.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run 'TestShiftUpExtendsAndCtrlDDeletesLine|TestCommentToggleBothKeyStrings' -v`
Expected: PASS.

- [ ] **Step 5: Run the package suite**

Run: `go test ./internal/tui 2>&1 | tail -5`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/update.go internal/tui/queryblock_test.go
git commit -m "feat(tui): wire block selection keys in query editor"
```

### Task 3: Mouse shift-extend

**Files:**
- Modify: `internal/tui/mouse.go`
- Test: `internal/tui/queryblock_test.go`

**Interfaces:**
- Consumes: Task 1 `ExtendSelectionTo`/`ClearSelection`; existing `clickQuery` editor math (gutter `-3`, `OffY + rel` line mapping).
- Produces: shift-aware click path used by Task 4 rendering.

- [ ] **Step 1: Write the failing test**

```go
func TestShiftClickExtendsBlock(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("SELECT 1\nFROM foo\nWHERE x")
	m.width, m.height = 120, 40
	m.resizeBrowse()
	// Plain click line 0 (editor-relative): detailTableTop + 0.
	u, _ := m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: m.paneX() + 4, Y: detailTableTop})
	m = u.(Model)
	if m.editor.CurLine != 0 {
		t.Fatalf("plain click line 0, got %d", m.editor.CurLine)
	}
	u, _ = m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: m.paneX() + 4, Y: detailTableTop + 2, Shift: true})
	m = u.(Model)
	lo, hi, active := m.editor.SelectedRange()
	if !active || lo != 0 || hi != 2 {
		t.Fatalf("shift-click must select 0-2, got %d-%d active=%v", lo, hi, active)
	}
}
```

Check `paneX()`/`resizeBrowse` names against `model.go`/`view.go` before running; the existing mouse tests use `m.paneX()` — reuse the same helper.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run TestShiftClickExtendsBlock -v`
Expected: FAIL (shift ignored, no active selection).

- [ ] **Step 3: Write minimal implementation**

Change signatures and thread `shift` (only call sites are in `mouse.go`):

```go
func (m Model) clickTable(x, y int, shift bool) (tea.Model, tea.Cmd) {
	if m.tab == 3 {
		return m.clickQuery(x, y, shift)
	}
	...
```

```go
func (m Model) clickQuery(x, y int, shift bool) (tea.Model, tea.Cmd) {
	...
	if rel := y - detailTableTop; rel >= 0 && rel < queryEditorH {
		...
		m.queryFocus = 0
		line := m.editor.OffY + rel
		...
		if shift {
			m.editor.ExtendSelectionTo(line)
		} else {
			m.editor.ClearSelection()
			m.editor.CurLine = line
			...
		}
		...
```

Update the two `clickTable` call sites in `handleMouse` to pass `msg.Shift`. Non-query tabs ignore `shift` (existing row-select behavior unchanged). Mouse drag is a sequence of press/motion events; motion does not extend — shift-press is the contract (documented in the hint line).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run TestShiftClickExtendsBlock -v`
Expected: PASS.

- [ ] **Step 5: Run the package suite**

Run: `go test ./internal/tui 2>&1 | tail -5`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/mouse.go internal/tui/queryblock_test.go
git commit -m "feat(tui): extend block selection with shift-click"
```

### Task 4: Highlight, hints, and help text

**Files:**
- Modify: `internal/tui/view_detail.go`, `internal/tui/whichkey.go`
- Test: `internal/tui/queryblock_test.go`

**Interfaces:**
- Consumes: Task 1 `SelectedRange`; existing `dataSelectedStyle`, `queryEditorH`, `fitText`/`paneW`.
- Produces: visual block + discoverable keys (status/hint surface used by Task 5 copy feedback).

- [ ] **Step 1: Write the failing test**

```go
func TestSelectedLinesRenderHighlighted(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("SELECT 1\nFROM foo")
	m.editor.CurLine = 0
	m.editor.ExtendSelectionTo(1)
	out := m.queryEditorView()
	lines := strings.Split(out, "\n")
	if len(lines) != queryEditorH {
		t.Fatalf("editor must stay %d lines, got %d", queryEditorH, len(lines))
	}
	// Both selected rows must differ from an unselected render.
	m2 := queryTabModel()
	m2.editor.SetText("SELECT 1\nFROM foo")
	plain := m2.queryEditorView()
	if out == plain {
		t.Fatal("selected render must differ from plain render")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run TestSelectedLinesRenderHighlighted -v`
Expected: FAIL (no selection styling yet).

- [ ] **Step 3: Write minimal implementation**

In `editorLineView` (or its caller `queryEditorView`), after computing the line string, wrap selected rows:

```go
if lo, hi, active := m.editor.SelectedRange(); active && lineIdx >= lo && lineIdx <= hi {
	// Reuse the grid selection role so block matches app chrome.
	return dataSelectedStyle.Render(lineStr)
}
```

Apply the wrap at the point where the full gutter+code string is assembled (so line numbers highlight too). Keep width truncation before styling (existing `maxCode` logic untouched). Update the query hint line:

```go
"tab complete • ctrl+s copy • ctrl+r run • e edit"
// becomes:
"shift+↑↓ select • ctrl+d del • ctrl+s copy • ctrl+/ comment • ctrl+r run"
```

via `fitText(..., m.paneW())` as today. Add one `whichkey.go` row: `{"shift+up/down", "select block", "Query editor"}`, `{"ctrl+d", "delete line/block", "Query editor"}`, `{"ctrl+/", "toggle -- comment", "Query editor"}`. Keep rows within existing table shape.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run TestSelectedLinesRenderHighlighted -v`
Expected: PASS.

- [ ] **Step 5: Run the package suite**

Run: `go test ./internal/tui 2>&1 | tail -5`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/view_detail.go internal/tui/whichkey.go internal/tui/queryblock_test.go
git commit -m "feat(tui): highlight selected query lines and document keys"
```

### Task 5: Selection-aware copy

**Files:**
- Modify: `internal/tui/copyquery.go`, `internal/tui/update.go`
- Test: `internal/tui/copyquery_test.go`, `internal/tui/queryblock_test.go`

**Interfaces:**
- Consumes: Task 1 `SelectedRange`/`SelectionText`; existing `queryDumpText`, `clipboardWriteAll` seam, `writeQueryDumpFile`, `m.status`, `m.err`.
- Produces: final user-visible behavior; nothing downstream.

- [ ] **Step 1: Write the failing test**

```go
func TestCopySelectionOnly(t *testing.T) {
	var got string
	old := clipboardWriteAll
	clipboardWriteAll = func(s string) error { got = s; return nil }
	defer func() { clipboardWriteAll = old }()
	m := queryTabModel()
	m.editor.SetText("SELECT 1\nFROM foo\nWHERE x")
	m.editor.CurLine = 0
	m.editor.ExtendSelectionTo(1)
	m.err = "boom"
	u, _ := m.queryKeys(tea.KeyMsg{Type: tea.KeyCtrlS}, "ctrl+s")
	m = u.(Model)
	if got != "SELECT 1\nFROM foo" {
		t.Fatalf("selection copy must exclude dump header and error, got %q", got)
	}
	if m.status == "" {
		t.Fatal("status must report the copy")
	}
}
```

Existing `copyquery_test.go` already stubs `clipboardWriteAll` — follow its pattern (save/restore).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run TestCopySelectionOnly -v`
Expected: FAIL (ctrl+s sends full dump with header).

- [ ] **Step 3: Write minimal implementation**

Add to `copyquery.go`:

```go
// copySelectionText copies the raw selected lines (no dump header, no
// error trailer). It shares the never-panic clipboard path.
func (m *Model) copySelectionText(text string) (status string) {
	text = strings.ReplaceAll(text, "\x00", "")
	n := len(strings.Split(text, "\n"))
	defer func() {
		if recover() != nil {
			status = writeQueryDumpFile(text)
		}
	}()
	if err := clipboardWriteAll(text); err == nil {
		if n == 1 {
			return "1 line copied to clipboard"
		}
		return fmt.Sprintf("%d lines copied to clipboard", n)
	}
	return writeQueryDumpFile(text)
}
```

Change both `ctrl+s` sites in `update.go` (editor-focused and results-focused) to branch first:

```go
case "ctrl+s":
	if txt := m.editor.SelectionText(); txt != "" {
		m.status = m.copySelectionText(txt)
		return m, nil
	}
	m.status = m.copyQueryDump()
	return m, nil
```

`SelectionText` returns `""` exactly when no selection is active (Task 1), so the full-dump path is preserved bit-for-bit including the `m.err` trailer.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run TestCopySelectionOnly -v`
Expected: PASS.

- [ ] **Step 5: Run the full suite**

Run: `go test ./... 2>&1 | tail -10`
Expected: all packages PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/copyquery.go internal/tui/update.go internal/tui/copyquery_test.go internal/tui/queryblock_test.go
git commit -m "feat(tui): copy selected query lines to clipboard"
```
