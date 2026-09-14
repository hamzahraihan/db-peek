# Sidebar Scroll + Fragment Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the sidebar explorer a scroll viewport with an overlay scrollbar and eliminate broken-ANSI fragments on long rows.

**Architecture:** `Explorer` gains an `Offset` (first visible tree row) with an `ensureVisible` clamp called after every cursor mutation; `Render` shows a pure window slice and builds lines style-last; a one-cell overlay scrollbar marks position; mouse hit-testing adds the offset.

**Tech Stack:** Go 1.25, `github.com/charmbracelet/bubbletea` v1.3.10 (struct `tea.MouseMsg`), `bubbletea`/`lipgloss`/`x/ansi`, `muesli/termenv` in tests.

## Global Constraints

- `View` is pure rendering only — no state changes (`internal/tui/view.go:3`).
- Mouse rows are row-exact: first visible tree row stays at terminal row `explorerFirstRow = 5` (`internal/tui/view.go:45`).
- `Offset` defaults to 0; no existing test may need modification.
- Sidebar widths and box borders do not change.
- Commit style: `fix(tui): …` / `feat(tui): …`, one task per commit.

---

### Task 1: Offset state, viewport window, cursor-follow clamp

**Files:**
- Modify: `internal/tui/explorer.go:50-55` (struct), `:125-172` (Move/Toggle/SetFilter), `:212-287` (Render)
- Modify: `internal/tui/tables.go:41-49` (add `sidebarTreeH` next to `contentH`)
- Modify: `internal/tui/view.go:64` (Render call uses the helper)
- Test: `internal/tui/scroll_test.go` (new file)

**Interfaces:**
- Consumes: `VisibleRows() []Row`, `contentH() int` (existing).
- Produces: `Offset int` field, `func (e *Explorer) ensureVisible(viewH int)`, `func (m Model) sidebarTreeH() int`. Later tasks rely on these exact names.

- [ ] **Step 1: Write the failing test**

```go
package tui

import (
	"fmt"
	"strings"
	"testing
)

func bigExplorer() Explorer {
	e := NewExplorer("test", []string{"s"})
	e.Schemas[0].Expanded = true
	for i := 0; i < 30; i++ {
		e.Schemas[0].Tables = append(e.Schemas[0].Tables,
			TableNode{Schema: "s", Name: fmt.Sprintf("t%02d", i), CountOK: true, Count: int64(i)})
	}
	return e
}

func TestExplorerEnsureVisibleFollowsCursor(t *testing.T) {
	e := bigExplorer() // 31 rows: 1 schema + 30 tables
	e.Cursor = 29
	e.ensureVisible(10)
	if e.Offset != 20 {
		t.Fatalf("want offset 20, got %d", e.Offset)
	}
	e.Cursor = 0
	e.ensureVisible(10)
	if e.Offset != 0 {
		t.Fatalf("want offset 0, got %d", e.Offset)
	}
}

func TestRenderShowsWindowAndClampsPurely(t *testing.T) {
	e := bigExplorer()
	e.Cursor = 29
	e.ensureVisible(10)
	out := e.Render(32, 10)
	lines := strings.Split(out, "\n")
	if !strings.Contains(lines[2], "t19") {
		t.Fatalf("first visible tree row must be t19, got %q", lines[2])
	}
	if strings.Contains(out, "t00") {
		t.Fatalf("t00 must be scrolled out:\n%s", out)
	}
	e.Offset = 999
	_ = e.Render(32, 10) // stale offset: clamp locally, do NOT mutate
	if e.Offset != 999 {
		t.Fatalf("Render must stay pure, offset mutated to %d", e.Offset)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run 'TestExplorerEnsureVisible|TestRenderShowsWindow' -v`
Expected: FAIL with "undefined: ensureVisible" / "undefined: Offset".

- [ ] **Step 3: Write minimal implementation**

In `internal/tui/explorer.go`, extend the struct:

```go
type Explorer struct {
	ConnName string
	Schemas  []SchemaNode
	Cursor   int
	Filter   string
	Offset   int // first visible tree-row index into VisibleRows
}
```

Add the clamp (handles empty lists and short lists):

```go
// ensureVisible keeps Cursor inside [Offset, Offset+viewH) and Offset
// inside its valid range. Call after every cursor/row mutation.
func (e *Explorer) ensureVisible(viewH int) {
	if viewH < 1 {
		viewH = 1
	}
	n := len(e.VisibleRows())
	if n == 0 {
		e.Cursor, e.Offset = 0, 0
		return
	}
	if e.Cursor < 0 {
		e.Cursor = 0
	}
	if e.Cursor >= n {
		e.Cursor = n - 1
	}
	maxOff := n - viewH
	if maxOff < 0 {
		maxOff = 0
	}
	if e.Offset < 0 {
		e.Offset = 0
	}
	if e.Offset > maxOff {
		e.Offset = maxOff
	}
	if e.Cursor < e.Offset {
		e.Offset = e.Cursor
	}
	if e.Cursor >= e.Offset+viewH {
		e.Offset = e.Cursor - viewH + 1
	}
}
```

Reset on filter (in `SetFilter`, next to `e.Cursor = 0`):

```go
func (e *Explorer) SetFilter(f string) {
	e.Filter = f
	e.Cursor = 0
	e.Offset = 0
}
```

Window the rows in `Render` (pure clamp via locals only):

```go
func (e *Explorer) Render(sidebarW, height int) string {
	rows := e.VisibleRows()
	if height < 1 {
		height = 1
	}
	start := e.Offset
	maxStart := len(rows) - height
	if maxStart < 0 {
		maxStart = 0
	}
	if start < 0 {
		start = 0
	}
	if start > maxStart {
		start = maxStart
	}
	end := start + height
	if end > len(rows) {
		end = len(rows)
	}
	vis := rows[start:end]
	// ... build title + conn lines as today, then range over vis,
	// highlighting with: if i+start == e.Cursor { ... }
```

In `internal/tui/tables.go`, add the single viewport-height truth:

```go
// sidebarTreeH is the number of explorer tree rows visible in the
// sidebar: inner box height minus title, conn, separator, footer.
func (m Model) sidebarTreeH() int {
	h := m.contentH() - 2 - 4
	if h < 1 {
		h = 1
	}
	return h
}
```

In `internal/tui/view.go:64`, replace `m.explorer.Render(innerW, innerH-4)` with `m.explorer.Render(innerW, m.sidebarTreeH())` (identical value, one source of truth).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run 'TestExplorerEnsureVisible|TestRenderShowsWindow' -v`
Expected: PASS.

Run: `go test ./internal/tui/ -count=1`
Expected: PASS (offset defaults to 0; existing tests unaffected).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/explorer.go internal/tui/tables.go internal/tui/view.go internal/tui/scroll_test.go
git commit -m "feat(tui): sidebar scroll offset with cursor-follow viewport"
```

### Task 2: Style-last line building (fragment fix)

**Files:**
- Modify: `internal/tui/explorer.go:224-281` (row line construction)
- Test: `internal/tui/scroll_test.go` (append)

**Interfaces:**
- Consumes: `Offset`/window from Task 1, `fitText` (`internal/tui/styles.go:43`).
- Produces: `func explorerLine(left, right string, styleRight func(string) string, w int) string`. Task 3 draws the scrollbar over its output.

- [ ] **Step 1: Write the failing test**

```go
func TestSidebarNarrowRenderHasNoBrokenEscapes(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	e := NewExplorer("postgres", []string{"auth"})
	e.Schemas[0].Expanded = true
	e.Schemas[0].Tables = []TableNode{
		{Schema: "auth", Name: "mfa_factors", Expanded: true, CountErr: true, Columns: []ColumnNode{
			{Name: "id", DataType: "uuid", IsPK: true},
			{Name: "status", DataType: "USER-DEFINED"},
			{Name: "last_webauthn_challenge_xyz", DataType: "text"},
		}},
	}
	re := regexp.MustCompile("\x1b\\[[0-9;]*[A-Za-z]")
	for _, w := range []int{18, 20, 24, 32} {
		out := e.Render(w, 40)
		for i, ln := range strings.Split(out, "\n") {
			if lipgloss.Width(ln) > w {
				t.Fatalf("width %d line %d exceeds: %q", w, i, ln)
			}
		}
		if rest := re.ReplaceAllString(out, ""); strings.Contains(rest, "\x1b") {
			t.Fatalf("width %d has broken escape sequence:\n%q", w, out)
		}
	}
}
```

Imports to add in `scroll_test.go`: `regexp`, `github.com/charmbracelet/lipgloss`, `github.com/muesli/termenv`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestSidebarNarrowRenderHasNoBrokenEscapes -v`
Expected: FAIL with "broken escape sequence" (today `fitText` slices through the styled type segment).

- [ ] **Step 3: Write minimal implementation**

Replace the per-row assembly in `Render` so truncation happens on plain text before any styling:

```go
// explorerLine joins a plain left and plain right with gap spaces to
// exactly fit w cells, then applies styleRight. Truncation runs on
// plain text only, so styled output is never sliced mid-escape.
func explorerLine(left, right string, styleRight func(string) string, w int) string {
	if w < 4 {
		w = 4
	}
	if lipgloss.Width(right) > w-2 {
		right = fitText(right, w-2)
	}
	maxLeft := w - lipgloss.Width(right) - 1
	if maxLeft < 1 {
		maxLeft = 1
	}
	left = fitText(left, maxLeft)
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + styleRight(right)
}
```

Per-row call sites become (keeping today's icons, indents, and styles):

```go
case RowSchema:
	// ... disc, n as today ...
	left = disc + " 🗄 " + r.Schema
	right = fmt.Sprintf("%d", n)
	line := explorerLine(left, right, explorerCount.Render, sidebarW)
case RowTable:
	// ... disc, icon as today ...
	left = "  " + disc + " " + icon + " " + r.Table
	if tb.CountErr {
		right = "?"
	} else if tb.CountOK {
		right = humanizeCount(tb.Count)
	} else {
		right = "…"
	}
	line := explorerLine(left, right, explorerCount.Render, sidebarW)
case RowColumn:
	// ... icon as today ...
	left = "    " + icon + " " + r.Column
	line := explorerLine(left, c.DataType, explorerType.Render, sidebarW)
```

then the existing selection wrap (`if i+start == e.Cursor`) and height cap stay as-is. Keep the final `ansi.Truncate` safety net in `browseView` untouched. Add the `x/ansi` import only if the compiler demands it (it does not — no new ansi use here).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestSidebarNarrowRenderHasNoBrokenEscapes -v`
Expected: PASS.

Run: `go test ./internal/tui/ -count=1`
Expected: PASS (content assertions like `"4.0k"`/`"integer"` in `explorer_test.go` still match — truncation only kicks in on overflow).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/explorer.go internal/tui/scroll_test.go
git commit -m "fix(tui): build sidebar lines style-last to prevent ANSI fragments"
```

### Task 3: Overlay scrollbar

**Files:**
- Modify: `internal/tui/explorer.go` (scrollbar pass at end of `Render`)
- Modify: `internal/tui/styles.go:63-71` (add two styles)
- Test: `internal/tui/scroll_test.go` (append)

**Interfaces:**
- Consumes: windowed `lines` + `start` from Tasks 1–2.
- Produces: scrollbar embedded in `Render` output; no new exported API.

- [ ] **Step 1: Write the failing test**

```go
func TestSidebarScrollbarOverlay(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	e := bigExplorer() // 31 rows
	out := e.Render(32, 10)
	lines := strings.Split(out, "\n")
	if !strings.Contains(lines[2], "█") {
		t.Fatalf("top of overflow must show thumb on first tree line:\n%s", out)
	}
	n := 0
	for _, ln := range lines {
		if strings.Contains(ln, "█") {
			n++
		}
	}
	if n != 3 { // 10*10/31 = 3
		t.Fatalf("want thumb height 3, got %d:\n%s", n, out)
	}
	small := NewExplorer("t", []string{"s"})
	small.Schemas[0].Expanded = true
	small.Schemas[0].Tables = []TableNode{{Schema: "s", Name: "only", CountOK: true}}
	if out := small.Render(32, 10); strings.Contains(out, "█") || strings.Contains(out, "│") {
		t.Fatalf("no scrollbar without overflow:\n%s", out)
	}
}
```

Note: the box border `│` is drawn by `browseView`, not `Render`, so asserting its absence here is safe.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestSidebarScrollbarOverlay -v`
Expected: FAIL with "must show thumb".

- [ ] **Step 3: Write minimal implementation**

In `internal/tui/styles.go`, add:

```go
scrollTrackStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
scrollThumbStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
```

In `explorer.go`, add the cell helper (requires importing `github.com/charmbracelet/x/ansi`):

```go
// setLastCell overlays ch on the final cell of an ANSI-styled line of
// width w, padding short lines with spaces first.
func setLastCell(line, ch string, w int) string {
	if w < 1 {
		return line
	}
	if vw := lipgloss.Width(line); vw < w-1 {
		line += strings.Repeat(" ", w-1-vw)
	} else if vw >= w {
		line = ansi.Truncate(line, w-1, "")
	}
	return line + ch
}
```

At the end of `Render`, after the height cap and before joining (operate on the tree-row lines only — skip title index 0 and conn index 1):

```go
if len(rows) > height {
	thumbH := height * height / len(rows)
	if thumbH < 1 {
		thumbH = 1
	}
	thumbStart := 0
	if span := len(rows) - height; span > 0 {
		thumbStart = start * (height - thumbH) / span
	}
	for i := range lines {
		if i < 2 { // title + conn lines carry no scrollbar
			continue
		}
		ti := i - 2
		if ti >= height {
			break
		}
		ch, st := "│", scrollTrackStyle
		if ti >= thumbStart && ti < thumbStart+thumbH {
			ch, st = "█", scrollThumbStyle
		}
		lines[i] = setLastCell(lines[i], st.Render(ch), sidebarW)
	}
}
```

This sits after the existing `if height > 0 && len(lines) > height` cap; note `lines` at that point is `[title, conn, tree...]` capped to `height` tree rows plus 2 chrome = `height+2` entries. Guard accordingly: cap tree lines to `height` before this pass (existing code already slices `lines[:height]` — adjust it to keep title+conn plus `height` tree rows, i.e. `lines[:height+2]` when longer, since title/conn are always present).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestSidebarScrollbarOverlay -v`
Expected: PASS.

Run: `go test ./internal/tui/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/explorer.go internal/tui/styles.go internal/tui/scroll_test.go
git commit -m "feat(tui): overlay scrollbar for sidebar explorer"
```

### Task 4: Model wiring — keys, wheel, resize, filter

**Files:**
- Modify: `internal/tui/update.go:340-346` (sidebar up/down/toggle), `:413-418` (filter up/down), `:18-22` (WindowSizeMsg → resize path)
- Modify: `internal/tui/mouse.go:131-138` (sidebar wheel)
- Modify: `internal/tui/tables.go:53-66` (`resizeBrowse` clamps offset)
- Test: `internal/tui/scroll_test.go` (append; uses existing helper `browseModel` in `internal/tui/sidebar_test.go:19`)

**Interfaces:**
- Consumes: `ensureVisible`, `sidebarTreeH` from Task 1.
- Produces: cursor-always-visible behavior through every Model path.

- [ ] **Step 1: Write the failing test**

```go
func TestSidebarKeyboardScrollsViewport(t *testing.T) {
	m := browseModel(t)
	m.height = 12 // tree viewport = 12-3-2-4 = 3 rows; fixture has 5 rows
	m.loading = false
	down := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}
	for i := 0; i < 4; i++ {
		u, _ := m.Update(down)
		m = u.(Model)
	}
	if m.explorer.Cursor != 4 {
		t.Fatalf("want cursor 4, got %d", m.explorer.Cursor)
	}
	if m.explorer.Offset != 2 { // 4-3+1
		t.Fatalf("want offset 2, got %d", m.explorer.Offset)
	}
}

func TestSidebarWheelScrollsViewport(t *testing.T) {
	m := browseModel(t)
	m.height = 12
	m.loading = false
	m.focusDetail = false
	u, _ := m.Update(tea.MouseMsg{Type: tea.MouseWheelDown})
	m = u.(Model)
	if m.explorer.Offset == 0 && m.explorer.Cursor >= m.sidebarTreeH() {
		t.Fatalf("wheel must pull viewport along: cursor=%d offset=%d treeH=%d",
			m.explorer.Cursor, m.explorer.Offset, m.sidebarTreeH())
	}
}
```

(`browseModel` builds width 100 / height 30 with the 5-row `fixtureExplorer`: rows 0=schema public, 1=orders, 2=col id, 3=col status, 4=customers. Imports needed in `scroll_test.go`: `tea "github.com/charmbracelet/bubbletea"`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run 'TestSidebarKeyboardScrolls|TestSidebarWheelScrolls' -v`
Expected: FAIL with "want offset 2, got 0".

- [ ] **Step 3: Write minimal implementation**

In `sidebarKeys`, after each cursor mutation:

```go
case "up", "k":
	m.explorer.MoveUp()
	m.explorer.ensureVisible(m.sidebarTreeH())
	return m, nil
case "down", "j":
	m.explorer.MoveDown()
	m.explorer.ensureVisible(m.sidebarTreeH())
	return m, nil
case "left", "right":
	m.explorer.Toggle()
	m.explorer.ensureVisible(m.sidebarTreeH())
	return m, nil
```

In `filterKeys`:

```go
case "up":
	m.explorer.MoveUp()
	m.explorer.ensureVisible(m.sidebarTreeH())
	return m, nil
case "down":
	m.explorer.MoveDown()
	m.explorer.ensureVisible(m.sidebarTreeH())
	return m, nil
```

(`SetFilter` on the typing path already resets offset inside Task 1.)

In `mouse.go` `wheel`, sidebar branch:

```go
if !m.focusDetail {
	for range steps {
		if up {
			m.explorer.MoveUp()
		} else {
			m.explorer.MoveDown()
		}
	}
	m.explorer.ensureVisible(m.sidebarTreeH())
}
```

In `tables.go` `resizeBrowse`, clamp after sizing (terminal shrinks must not strand the offset):

```go
m.sidebarW = w
m.explorer.ensureVisible(m.sidebarTreeH())
m.sizeTables()
```

Note: `resizeBrowse` has pointer receiver and `sidebarTreeH` has value receiver — callable directly. `enter`-key previews (`inspectTable`) intentionally do not touch focus or offset (prior fix).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run 'TestSidebarKeyboardScrolls|TestSidebarWheelScrolls' -v`
Expected: PASS.

Run: `go test ./... -count=1`
Expected: PASS across `db` and `tui`.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/update.go internal/tui/mouse.go internal/tui/tables.go internal/tui/scroll_test.go
git commit -m "fix(tui): keep sidebar cursor visible on keys, wheel, resize"
```

### Task 5: Mouse hit-testing with scroll offset

**Files:**
- Modify: `internal/tui/mouse.go:215-221` (`clickExplorer`), `:339-348` (`hoverList` sidebar branch)
- Test: `internal/tui/scroll_test.go` (append)

**Interfaces:**
- Consumes: `Offset` from Task 1. No new API.

- [ ] **Step 1: Write the failing test**

```go
func TestSidebarClickUsesOffset(t *testing.T) {
	m := browseModel(t)
	m.height = 12 // tree viewport 3 rows; fixture rows 0..4
	m.loading = false
	m.focusDetail = false
	m.explorer.Offset = 2
	// First visible tree row (y=5) is fixture row 2 (col id → previews orders).
	u, _ := m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 2, Y: 5})
	m = u.(Model)
	if m.explorer.Cursor != 2 {
		t.Fatalf("want cursor 2, got %d", m.explorer.Cursor)
	}
	if m.table != "orders" {
		t.Fatalf("want orders previewed, got %q", m.table)
	}
	if m.focusDetail {
		t.Fatal("sidebar click must not steal focus")
	}
}

func TestSidebarHoverUsesOffset(t *testing.T) {
	m := browseModel(t)
	m.height = 12
	m.loading = false
	m.explorer.Offset = 2
	u, _ := m.Update(tea.MouseMsg{Type: tea.MouseMotion, X: 2, Y: 6})
	m = u.(Model)
	if m.explorer.Cursor != 3 {
		t.Fatalf("want cursor 3, got %d", m.explorer.Cursor)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run 'TestSidebarClickUsesOffset|TestSidebarHoverUsesOffset' -v`
Expected: FAIL (click maps to row 0, cursor 0).

- [ ] **Step 3: Write minimal implementation**

In `clickExplorer`:

```go
func (m Model) clickExplorer(x, y int) (tea.Model, tea.Cmd) {
	idx := y - explorerFirstRow + m.explorer.Offset
```

In `hoverList` sidebar branch:

```go
if which == screenBrowse {
	// Sidebar hover follows the explorer cursor; no preview.
	idx := y - explorerFirstRow + m.explorer.Offset
```

Nothing else changes: `RowAt` miss handling still noops out-of-range clicks, border/chrome/focus logic is untouched, and the first visible row remains at `explorerFirstRow`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run 'TestSidebarClickUsesOffset|TestSidebarHoverUsesOffset' -v`
Expected: PASS.

Run: `go test ./... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/mouse.go internal/tui/scroll_test.go
git commit -m "fix(tui): sidebar mouse hit-testing accounts for scroll offset"
```

## Self-Review

- Spec coverage: §2 state → Task 1 (Offset, ensureVisible, sidebarTreeH, SetFilter reset, pure Render clamp). §3 rendering → Task 2 (style-last fragment fix) + Task 3 (overlay scrollbar, geometry, chrome exclusion). §4 input → Task 4 (cursor-follow on keys/wheel/resize/filter) + Task 5 (offset hit-testing, y=5 anchor preserved). §5 tests → each task's tests; existing suite untouched by default-zero offset.
- Placeholder scan: no TBD/TODO; every step names exact files, symbols, commands, and expected outcomes. Test code is complete and compilable against named imports (`fmt`, `regexp`, `strings`, `testing`, `lipgloss`, `termenv`, `tea`).
- Type consistency: `Offset int`, `ensureVisible(viewH int)`, `sidebarTreeH() int`, `explorerLine(left, right string, styleRight func(string) string, w int) string`, `setLastCell(line, ch string, w int) string`, `scrollTrackStyle`/`scrollThumbStyle`, thumb math identical in Task 3 code and test (`10*10/31 = 3`).
- One correction made inline: Task 3's existing height-cap slice must preserve title+conn plus `height` tree rows (`lines[:height+2]`), since the scrollbar loop skips indices 0–1.
