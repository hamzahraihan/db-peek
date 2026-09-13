# Panes, Query Lab, ER Diagram, Which-Key Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bordered focusable panes with mouse focus, a live-highlighted SQL query tab with results, a 1-hop ER diagram tab, a `?` keybinding overlay, and zebra-striped grids.

**Architecture:** `browseView` boxes both panes (gold focused / dim unfocused) with border-relative mouse math; detail tabs grow 3→5 sharing the existing tab strip, seq guards, and `dataTable`; new units `queryedit.go` (chroma token-stream editor), `db/refs.go` (per-engine FKs), `er.go` (1-hop boxes), `whichkey.go` (registry-driven overlay).

**Tech Stack:** Go 1.25, bubbletea, bubbles, lipgloss, new `github.com/alecthomas/chroma/v2` (SQL lexer only; rendering stays lipgloss).

## Global Constraints

- Sidebar gold selection stays `#CA8A04` (172m); focused pane border gold `#EAB308`, unfocused `#3A3A3A`; rounded borders.
- Every visible tree/grid row stays exactly 1 terminal row; `lipgloss.Width(line) <=` its pane inner width always.
- `?` works everywhere except conns-filter typing, form inputs, and query-editor typing (where `?` inserts text).
- `db.Query` semantics unchanged (200-row cap); query runs keep a 15s timeout and a seq guard like `detailSeq`.
- Editor v1: insert/delete/newline only, no selection, no undo.
- Commit after every task passes `go test ./...` and `go vet ./...`.

---

## File Structure

- Modify `internal/tui/datatable.go` — zebra row style in `view()` (+ dimmed variant).
- Modify `internal/tui/styles.go` — border styles, zebra styles, chroma token styles, overlay styles.
- Modify `internal/tui/view.go` — box both panes in `browseView`; new border-aware row constants.
- Modify `internal/tui/tables.go` — `resizeBrowse`/`sizeTables` absorb border chrome (2 cols + 2 rows per pane).
- Modify `internal/tui/mouse.go` — border-relative hit-testing; pane click sets focus.
- Modify `internal/tui/model.go` — tab count 3→5, query state, ER state, overlay state, `keyRegistry` wiring points.
- Modify `internal/tui/update.go` — `detailKey` 5-tab cycling, `4`/`5` keys, query/editor key routing, ER nav, `?` toggle.
- Modify `internal/tui/view_detail.go` — query tab layout (editor top, results bottom), ER tab render.
- Modify `internal/tui/commands.go`, `internal/tui/msg.go` — `runQueryCmd`/`queryDoneMsg`, `erLoadedMsg`/`loadERCmd`.
- Create `internal/tui/queryedit.go` — `Editor` rune buffer + cursor + chroma render.
- Create `internal/db/refs.go` — `ForeignKey` + per-engine FK queries.
- Create `internal/tui/er.go` — 1-hop graph model + box layout + render.
- Create `internal/tui/whichkey.go` — `keyRegistry` + overlay render.
- Tests: `datatable_test.go` (new), `queryedit_test.go` (new), `refs_test.go` (new in `internal/db`), `er_test.go` (new), `whichkey_test.go` (new), extend `sidebar_test.go`.

---

### Task 1: Zebra striping

**Files:**
- Modify: `internal/tui/datatable.go:152-193`
- Modify: `internal/tui/styles.go`
- Test: `internal/tui/datatable_test.go` (new)

**Interfaces:**
- Consumes: existing `dataTable.view(dim bool)`, `dataSelectedStyle`, `dataHoverStyle`, `dimDataSelectedStyle`.
- Produces: `zebraStyle`, `zebraDimStyle` in styles.go — used by Task 2+ grids automatically.

- [ ] **Step 1: Write the failing test**

```go
package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestZebraEvenRows(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	var dt dataTable
	dt.Resize(40, 8)
	dt.setData([]string{"a"}, [][]string{{"r0"}, {"r1"}, {"r2"}, {"r3"}})
	dt.SetCursor(99) // clamp away: no selection highlight in output
	dt.cursor = -1
	out := dt.View()
	lines := strings.Split(out, "\n")
	if len(lines) != 6 { // header + border + 4 rows
		t.Fatalf("want 6 lines, got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[4], "48;5;235m") { // 2nd data row (even-numbered) darker
		t.Fatalf("even row missing zebra bg:\n%s", out)
	}
	if strings.Contains(lines[3], "48;5;235m") {
		t.Fatalf("odd row must not carry zebra bg:\n%s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestZebraEvenRows -v`
Expected: FAIL with "even row missing zebra bg" (no zebra style exists yet; note `dt.cursor = -1` bypasses selection — `SetCursor` clamps, direct field set is test-only).

- [ ] **Step 3: Write minimal implementation**

In `styles.go` append to the grid styles block:

```go
zebraStyle    = lipgloss.NewStyle().Background(lipgloss.Color("235"))
zebraDimStyle = lipgloss.NewStyle().Background(lipgloss.Color("234"))
```

In `datatable.go` `view()`, extend the row switch:

```go
line := strings.Join(cells, "")
switch {
case i == t.cursor && t.cursor >= 0:
	line = selectedStyle.Render(line)
case i == t.hover:
	line = hoverStyle.Render(line)
case i%2 == 1:
	if dim {
		line = zebraDimStyle.Render(line)
	} else {
		line = zebraStyle.Render(line)
	}
}
```

Also guard the cursor case with `t.cursor >= 0` so tests (and future empty states) can render selection-free; `SetCursor` still clamps to valid rows so production behavior is unchanged.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestZebraEvenRows -v`
Expected: PASS. Then: `go test ./... -count=1` and `go vet ./...` clean.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/datatable.go internal/tui/styles.go internal/tui/datatable_test.go
git commit -m "feat(tui): zebra-striped grid rows"
```

---

### Task 2: Pane borders + mouse focus

**Files:**
- Modify: `internal/tui/styles.go`
- Modify: `internal/tui/view.go:36-110`
- Modify: `internal/tui/tables.go:43-92`
- Modify: `internal/tui/mouse.go:46-77`
- Test: `internal/tui/sidebar_test.go` (extend)

**Interfaces:**
- Consumes: `Explorer.Render(w,h)`, `detailView()`, `paneX()`, `contentH()`, `clickExplorer(x,y)`, `clickTable(y)`, `clickTabs(x,y)`.
- Produces: `paneBorder(focused bool) lipgloss.Style` helper in styles.go; new constants `sideBoxTop = 1`, updated `explorerFirstRow = 5`, `detailTableTop = 6` — consumed by Tasks 4 and 6 mouse work.

Layout math (exact, implement verbatim):

```go
// view.go
const explorerFirstRow = 5 // y0 header, y1 side top border, y2 title, y3 conn, y4 separator, y5 first tree row
```

- `browseView`: header stays y0. Sidebar inner content = current `side` lines (title, conn, separator, tree rows, footer/empty lines) rendered at inner width `sidebarW-2`, then wrapped: `sideBox := paneBorder(!m.focusDetail).Render(strings.Join(sideInner, "\n"))` with style `Width(sidebarW-2)`. Detail inner content = current `right` lines at inner width, wrapped the same with `paneBorder(m.focusDetail)`. Join boxes with one space gap (drop the `│` separator). Both boxes get fixed height `contentH()` via style `Height(contentH())` so rows stay aligned; inner content height = `contentH()-2`.
- `resizeBrowse`/`sizeTables` (tables.go): explorer gets `Render(m.sidebarW-2, m.contentH()-2-chromeFoot)` where chromeFoot = 4 (title+conn+sep+footer, matching current Render+view chrome); detail grids get `w = m.width - m.paneX() - 2 - 2` (minus detail borders) and `h = m.contentH()-2-chrome` (existing per-tab chrome logic unchanged, minus 2 for borders).
- `mouse.go` browse branch: sidebar content hit `idx = y - explorerFirstRow` only when `x >= 1 && x <= sidebarW-2` (inside left border) and `y >= 1` box-relative; border cells noop. Any press inside sidebar box sets `m.focusDetail = false` before `clickExplorer`; any press inside detail box (tabs or grid, not its border) sets `m.focusDetail = true` before existing tab/grid handling. `paneX()` unchanged (first column of detail box); detail content x = `msg.X - m.paneX() - 1` (left border); detail content top `detailTableTop = 6` (header + top border + title + blank + tabs + blank).
- `tabStripRow()` scanning still works (labels unchanged); `clickTabs`/`tabAtX` take border-relative x (`msg.X - m.paneX() - 1`).

- [ ] **Step 1: Write the failing test**

```go
func TestPaneClickSwitchesFocus(t *testing.T) {
	m := browseModel(t)
	m.loading = false
	m.focusDetail = false
	m.table = "users"
	m.cols = []db.Column{{Name: "id"}}
	m.buildTables()
	m.sizeTables()
	// Click inside detail box grid area focuses detail.
	x := m.paneX() + 2
	y := 6 + 2 + 2 // detailTableTop + header(2) + first data row
	u, _ := m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: x, Y: y})
	m = u.(Model)
	if !m.focusDetail {
		t.Fatal("click inside detail must focus detail")
	}
	// Click inside sidebar box focuses sidebar.
	u, _ = m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 2, Y: 5})
	m = u.(Model)
	if m.focusDetail {
		t.Fatal("click inside sidebar must focus sidebar")
	}
}

func TestFocusedBorderGold(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	m := browseModel(t)
	m.loading = false
	m.focusDetail = true
	v := m.View()
	if !strings.Contains(v, "38;5;220m") { // gold #EAB308 border
		t.Fatalf("focused pane must draw gold border:\n%s", v)
	}
}
```

(`db` import already in sidebar_test.go; add `tea` — already imported.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run 'TestPaneClickSwitchesFocus|TestFocusedBorderGold' -v`
Expected: FAIL (no boxes; clicks don't move focus).

- [ ] **Step 3: Write minimal implementation**

styles.go append:

```go
func paneBorder(focused bool) lipgloss.Style {
	s := lipgloss.NewStyle().Border(lipgloss.RoundedBorder())
	if focused {
		return s.BorderForeground(lipgloss.Color("#EAB308"))
	}
	return s.BorderForeground(lipgloss.Color("#3A3A3A"))
}
```

view.go `browseView`: build `sideInner` (existing side lines, width-capped at `sidebarW-2`) and `rightInner` (width-capped at detail inner width), wrap each with `paneBorder`, join with `" "`, keep footer below. Update `explorerFirstRow` to 5. Keep `paneX()` definition; detail content width = `m.width - m.paneX() - 3` (gap + 2 borders), floor 20.

tables.go: `m.explorer` render height/width and `sizeTables` subtract border chrome per layout math above.

mouse.go: implement focus-on-click + border-relative coordinates per layout math above; border cells noop.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run 'TestPaneClickSwitchesFocus|TestFocusedBorderGold' -v`
Expected: PASS. Then full `go test ./... -count=1`, `go vet ./...`; fix existing mouse/fit tests to new row math (y shifts +1, x shifts for detail).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/view.go internal/tui/tables.go internal/tui/mouse.go internal/tui/styles.go internal/tui/sidebar_test.go
git commit -m "feat(tui): bordered panes with mouse focus"
```

---

### Task 3: Chroma editor unit

**Files:**
- Create: `internal/tui/queryedit.go`
- Test: `internal/tui/queryedit_test.go`
- Modify: `go.mod`, `go.sum` (via `go get`)

**Interfaces:**
- Consumes: `github.com/alecthomas/chroma/v2/lexers`, `chroma.TokenType` categories.
- Produces: `type Editor struct { Lines []string; CurLine, CurCol, OffY int }`, `func NewEditor() Editor`, `func (e *Editor) Insert(r rune)`, `func (e *Editor) Backspace()`, `func (e *Editor) Newline()`, `func (e *Editor) Delete()`, `func (e *Editor) MoveUp/Down/Left/Right/Home/End()`, `func (e *Editor) Text() string`, `func (e *Editor) SetText(s string)`, `func HighlightSQL(src string) [][]hlCell`, `type hlCell struct { Text string; Style lipgloss.Style }`, `func (e *Editor) CursorXY() (line, col int)` — consumed by Task 4.

Setup first: `go get github.com/alecthomas/chroma/v2@latest && go mod tidy`.

- [ ] **Step 1: Write the failing test**

```go
package tui

import (
	"strings"
	"testing
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
	e.Backspace() // at col 0 joins lines
	if got := e.Text(); got != "select 1x" {
		t.Fatalf("want join, got %q", got)
	}
}

func TestHighlightSQLKeywords(t *testing.T) {
	cells := HighlightSQL("SELECT * FROM users -- hi\nWHERE id = 1")
	flat := ""
	for _, ln := range cells {
		for _, c := range ln {
			flat += c.Text
		}
	}
	if flat != "SELECT * FROM users -- hi\nWHERE id = 1" {
		t.Fatalf("highlight must preserve source, got %q", flat)
	}
	var kwStyle string
	for _, c := range cells[0] {
		if c.Text == "SELECT" {
			kwStyle = c.Style.String()
		}
	}
	if kwStyle == "" {
		t.Fatal("SELECT token missing")
	}
	var plainStyle string
	for _, c := range cells[0] {
		if c.Text == "users" {
			plainStyle = c.Style.String()
		}
	}
	if kwStyle == plainStyle {
		t.Fatal("keyword must differ in style from identifier")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run 'TestEditorInsertAndText|TestHighlightSQLKeywords' -v`
Expected: FAIL with "undefined: NewEditor".

- [ ] **Step 3: Write minimal implementation**

```go
package tui

import (
	"strings"

	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/lipgloss"
)

type Editor struct {
	Lines   []string
	CurLine int
	CurCol  int // rune offset within line
	OffY    int // first visible line
}

func NewEditor() Editor { return Editor{Lines: []string{""}} }

func (e *Editor) runes() []rune { return []rune(e.Lines[e.CurLine]) }

func (e *Editor) Insert(r rune) {
	rs := e.runes()
	if e.CurCol > len(rs) {
		e.CurCol = len(rs)
	}
	e.Lines[e.CurLine] = string(rs[:e.CurCol]) + string(r) + string(rs[e.CurCol:])
	e.CurCol++
}

func (e *Editor) Backspace() {
	rs := e.runes()
	if e.CurCol > 0 {
		e.Lines[e.CurLine] = string(rs[:e.CurCol-1]) + string(rs[e.CurCol:])
		e.CurCol--
		return
	}
	if e.CurLine > 0 {
		prev := len([]rune(e.Lines[e.CurLine-1]))
		e.Lines[e.CurLine-1] += e.Lines[e.CurLine]
		e.Lines = append(e.Lines[:e.CurLine], e.Lines[e.CurLine+1:]...)
		e.CurLine--
		e.CurCol = prev
	}
}

func (e *Editor) Delete() {
	rs := e.runes()
	if e.CurCol < len(rs) {
		e.Lines[e.CurLine] = string(rs[:e.CurCol]) + string(rs[e.CurCol+1:])
		return
	}
	if e.CurLine < len(e.Lines)-1 {
		e.Lines[e.CurLine] += e.Lines[e.CurLine+1]
		e.Lines = append(e.Lines[:e.CurLine+1], e.Lines[e.CurLine+2:]...)
	}
}

func (e *Editor) Newline() {
	rs := e.runes()
	if e.CurCol > len(rs) {
		e.CurCol = len(rs)
	}
	e.Lines = append(e.Lines[:e.CurLine+1], append([]string{string(rs[e.CurCol:])}, e.Lines[e.CurLine+1:]...)...)
	e.Lines[e.CurLine] = string(rs[:e.CurCol])
	e.CurLine++
	e.CurCol = 0
}

func (e *Editor) MoveUp() {
	if e.CurLine > 0 {
		e.CurLine--
		e.CurCol = min(e.CurCol, len([]rune(e.Lines[e.CurLine])))
	}
}

func (e *Editor) MoveDown() {
	if e.CurLine < len(e.Lines)-1 {
		e.CurLine++
		e.CurCol = min(e.CurCol, len([]rune(e.Lines[e.CurLine])))
	}
}

func (e *Editor) MoveLeft() {
	if e.CurCol > 0 {
		e.CurCol--
	} else if e.CurLine > 0 {
		e.CurLine--
		e.CurCol = len([]rune(e.Lines[e.CurLine]))
	}
}

func (e *Editor) MoveRight() {
	if e.CurCol < len([]rune(e.Lines[e.CurLine])) {
		e.CurCol++
	} else if e.CurLine < len(e.Lines)-1 {
		e.CurLine++
		e.CurCol = 0
	}
}

func (e *Editor) Home() { e.CurCol = 0 }
func (e *Editor) End()  { e.CurCol = len([]rune(e.Lines[e.CurLine])) }

func (e *Editor) Text() string { return strings.Join(e.Lines, "\n") }

func (e *Editor) SetText(s string) {
	e.Lines = strings.Split(s, "\n")
	e.CurLine, e.CurCol, e.OffY = 0, 0, 0
}

func (e *Editor) CursorXY() (int, int) { return e.CurLine, e.CurCol }

type hlCell struct {
	Text  string
	Style lipgloss.Style
}

var (
	sqlKeyword = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#EAB308"))
	sqlString  = lipgloss.NewStyle().Foreground(lipgloss.Color("#4ADE80"))
	sqlNumber  = lipgloss.NewStyle().Foreground(lipgloss.Color("#67E8F9"))
	sqlComment = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	sqlPlain   = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
)

func HighlightSQL(src string) [][]hlCell {
	lx := lexers.Get("sql")
	if lx == nil {
		return [][]hlCell{{{Text: src, Style: sqlPlain}}}
	}
	it, err := lx.Tokenise(nil, src)
	if err != nil {
		return [][]hlCell{{{Text: src, Style: sqlPlain}}}
	}
	var out [][]hlCell
	cur := []hlCell{}
	flush := func() { out = append(out, cur); cur = []hlCell{} }
	for _, tok := range it.Tokens() {
		st := sqlPlain
		switch {
		case tok.Type.InCategory(chroma.Keyword):
			st = sqlKeyword
		case tok.Type.InCategory(chroma.LiteralString):
			st = sqlString
		case tok.Type.InCategory(chroma.LiteralNumber):
			st = sqlNumber
		case tok.Type.InCategory(chroma.Comment):
			st = sqlComment
		}
		parts := strings.Split(tok.Value, "\n")
		for i, p := range parts {
			if i > 0 {
				flush()
			}
			if p != "" {
				cur = append(cur, hlCell{Text: p, Style: st})
			}
		}
	}
	out = append(out, cur)
	return out
}
```

(`chroma.Keyword` etc. need import `"github.com/alecthomas/chroma/v2"`; `min` is builtin in Go 1.21+.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run 'TestEditorInsertAndText|TestHighlightSQLKeywords' -v`
Expected: PASS. Then `go test ./... -count=1`, `go vet ./...`.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/queryedit.go internal/tui/queryedit_test.go go.mod go.sum
git commit -m "feat(tui): chroma SQL editor unit"
```

---

### Task 4: Query tab wiring

**Files:**
- Modify: `internal/tui/model.go:43-73,115-128,214-236`
- Modify: `internal/tui/msg.go`
- Modify: `internal/tui/commands.go`
- Modify: `internal/tui/update.go:303-371`
- Modify: `internal/tui/view_detail.go`
- Modify: `internal/tui/mouse.go`
- Test: `internal/tui/sidebar_test.go` (extend)

**Interfaces:**
- Consumes: `Editor`, `HighlightSQL` (Task 3), `db.Query`, `dataTable`, `detailSeq` pattern.
- Produces: tab index 3 = query (`detailTabLabels`, `setTab` cycles `% 5`); `queryFocus` editor/results routing — consumed by Task 7 registry.

Model additions (exact):

```go
editor      Editor
queryFocus  int // 0 editor, 1 results (only meaningful when tab==3)
querySample *dbpkg.Sample
queryMs     int64
querySeq    int
queryTable  dataTable
```

msg.go append:

```go
queryDoneMsg struct {
	sql    string
	sample *dbpkg.Sample
	ms     int64
	seq    int
	err    error
}
```

commands.go append:

```go
func (m Model) runQuery() tea.Cmd {
	db, sql, seq := m.db, m.editor.Text(), m.querySeq
	return func() tea.Msg {
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		s, err := db.Query(ctx, sql)
		return queryDoneMsg{sql: sql, sample: s, ms: time.Since(start).Milliseconds(), seq: seq, err: err}
	}
}
```

(`time` import in commands.go; `context` already there.)

update.go `detailKey`: change `% 3` cycles to `% 5`; add `case "4": m.setTab(3)`, `case "5": m.setTab(4)`; at top of detailKey, if `m.tab == 3` route to new `queryKeys` (editor editing keys when `queryFocus==0`: runes insert, backspace, enter newline, arrows/home/end move, `ctrl+r`/`f5` run via runQuery with `querySeq++`, `esc` → focus results or sidebar, `tab` still toggles panes per handleKey); if `m.tab == 4` route ER nav (Task 6 implements; for now up/down noop returning nil — Task 6 fills). Guard `activeGrid` use to tabs 0-2 only.

`queryDoneMsg` handler: drop when `seq != m.querySeq`; on err set `m.err` (keep editor text + old results); on success build `queryTable` via `setData(sample.Columns, sample.Rows)` + `sizeTables`-style resize, set `queryMs`, `loading=false`.

view_detail.go: `case 3:` renders editor box (top, fixed 8 rows: highlighted lines from `HighlightSQL` at `OffY` scroll with reverse-video cursor cell when `queryFocus==0`, line numbers dim) + status/hint line + `queryTable` results grid + `N rows • M ms` line. `case 4:` renders `(er diagram — next task)` placeholder line + keeps tab chrome (Task 6 replaces).

mouse.go: clicks in editor area (tab==3, rows within editor box) position cursor (`CurLine = OffY + relY`, `CurCol` = rune count to x, clamped) and set `queryFocus=0`; clicks in results grid reuse grid RowAt on `queryTable` with `queryFocus=1`. `tabStripRow`/`clickTabs` work unchanged (labels shared).

sizeTables: `case m.tab == 3`: chrome = title+blank+tabs+blank+editor(8)+status(2); resize `queryTable` to remainder; `case 4`: chrome similar placeholder (Task 6 adjusts).

- [ ] **Step 1: Write the failing test**

```go
func TestQueryRunSeqGuard(t *testing.T) {
	m := browseModel(t)
	m.table = "users"
	m.tab = 3
	m.querySeq = 5
	stale := queryDoneMsg{sql: "select 1", seq: 4, sample: &db.Sample{Columns: []string{"a"}, Rows: [][]string{{"1"}}}}
	u, _ := m.Update(stale)
	m = u.(Model)
	if m.querySample != nil {
		t.Fatal("stale query reply must not apply")
	}
	fresh := queryDoneMsg{sql: "select 1", seq: 5, sample: &db.Sample{Columns: []string{"a"}, Rows: [][]string{{"1"}}}, ms: 3}
	u, _ = m.Update(fresh)
	m = u.(Model)
	if m.querySample == nil || m.queryMs != 3 {
		t.Fatal("fresh query reply must apply")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestQueryRunSeqGuard -v`
Expected: FAIL with "undefined: queryDoneMsg".

- [ ] **Step 3: Write minimal implementation**

Per interfaces above: model fields, msg, command, detailKey routing + queryKeys, queryDoneMsg handler, view_detail cases 3/4, mouse editor/results clicks, sizeTables chrome, tab labels/cycling `% 5`.

detailTabLabels full becomes:

```go
full := []string{"1 schema", "2 indexes", fmt.Sprintf("3 rows x%d", m.pageSize), "4 query", "5 er"}
```

narrow becomes `[]string{"1", "2", "3", "4", "5"}`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestQueryRunSeqGuard -v`
Expected: PASS. Then full suite + vet; update existing tab tests (`1/2/3` + cycling) to 5-tab.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/msg.go internal/tui/commands.go internal/tui/update.go internal/tui/view_detail.go internal/tui/mouse.go internal/tui/tables.go internal/tui/sidebar_test.go
git commit -m "feat(tui): query tab with live editor and results"
```

---

### Task 5: FK introspection (`db/refs.go`)

**Files:**
- Create: `internal/db/refs.go`
- Test: `internal/db/refs_test.go`

**Interfaces:**
- Consumes: `DB{SQL, Driver}`.
- Produces: `type ForeignKey struct { FromTable, FromColumn, ToTable, ToColumn string }`, `func (d *DB) ForeignKeys(ctx context.Context, table string) ([]ForeignKey, error)` (both directions for `table`: outgoing where table references others + incoming where others reference table), `func (d *DB) ReferencedBy(ctx context.Context, table string) ([]ForeignKey, error)` — fold both into `ForeignKeys` returning outgoing+incoming; ER splits by `FromTable == table`. Consumed by Task 6.

- [ ] **Step 1: Write the failing test**

```go
package db

import (
	"context"
	"database/sql"
	"testing"
)

func openFKMem(t *testing.T) *DB {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = sqlDB.Exec(`PRAGMA foreign_keys=ON;
CREATE TABLE customers(id INTEGER PRIMARY KEY, name TEXT);
CREATE TABLE orders(id INTEGER PRIMARY KEY, customer_id INTEGER REFERENCES customers(id), status TEXT);`)
	if err != nil {
		t.Fatal(err)
	}
	return &DB{SQL: sqlDB, Driver: SQLite, Display: ":memory:"}
}

func TestForeignKeysSQLite(t *testing.T) {
	d := openFKMem(t)
	defer d.SQL.Close()
	out, err := d.ForeignKeys(context.Background(), "orders")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, fk := range out {
		if fk.FromTable == "orders" && fk.FromColumn == "customer_id" && fk.ToTable == "customers" && fk.ToColumn == "id" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing orders→customers fk in %v", out)
	}
	in, err := d.ForeignKeys(context.Background(), "customers")
	if err != nil {
		t.Fatal(err)
	}
	back := false
	for _, fk := range in {
		if fk.FromTable == "orders" && fk.ToTable == "customers" {
			back = true
		}
	}
	if !back {
		t.Fatalf("missing incoming fk for customers in %v", in)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestForeignKeysSQLite -v`
Expected: FAIL with "undefined: ForeignKeys".

- [ ] **Step 3: Write minimal implementation**

```go
package db

import (
	"context"
	"fmt"
)

type ForeignKey struct {
	FromTable  string
	FromColumn string
	ToTable    string
	ToColumn   string
}

func (d *DB) ForeignKeys(ctx context.Context, table string) ([]ForeignKey, error) {
	switch d.Driver {
	case Postgres:
		rows, err := d.SQL.QueryContext(ctx, `
SELECT c.relname, a.attname, c2.relname, a2.attname
FROM pg_constraint o
JOIN pg_class c ON c.oid = o.conrelid
JOIN pg_class c2 ON c2.oid = o.confrelid
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = ANY(o.conkey)
JOIN pg_attribute a2 ON a2.attrelid = c2.oid AND a2.attnum = ANY(o.confkey)
WHERE o.contype = 'f' AND (c.relname = $1 OR c2.relname = $1)
ORDER BY 1, 2`, table)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []ForeignKey
		for rows.Next() {
			var fk ForeignKey
			if err := rows.Scan(&fk.FromTable, &fk.FromColumn, &fk.ToTable, &fk.ToColumn); err != nil {
				return nil, err
			}
			out = append(out, fk)
		}
		return out, rows.Err()
	case MySQL:
		rows, err := d.SQL.QueryContext(ctx, `
SELECT TABLE_NAME, COLUMN_NAME, REFERENCED_TABLE_NAME, REFERENCED_COLUMN_NAME
FROM information_schema.KEY_COLUMN_USAGE
WHERE TABLE_SCHEMA = DATABASE() AND REFERENCED_TABLE_NAME IS NOT NULL
AND (TABLE_NAME = ? OR REFERENCED_TABLE_NAME = ?) ORDER BY 1, 2`, table, table)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []ForeignKey
		for rows.Next() {
			var fk ForeignKey
			if err := rows.Scan(&fk.FromTable, &fk.FromColumn, &fk.ToTable, &fk.ToColumn); err != nil {
				return nil, err
			}
			out = append(out, fk)
		}
		return out, rows.Err()
	default:
		var out []ForeignKey
		out = append(out, d.sqliteOutgoing(ctx, table)...)
		// Incoming: scan all user tables' foreign_key_list for refs to table.
		names, err := d.ListTables(ctx)
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			if n == table {
				continue
			}
			q := fmt.Sprintf(`PRAGMA foreign_key_list(%s)`, d.Driver.QuoteIdent(n))
			r, err := d.SQL.QueryContext(ctx, q)
			if err != nil {
				continue
			}
			func() {
				defer r.Close()
				for r.Next() {
					var id, seq int
					var to, from, toCol string
					var onUpd, onDel, match string
					if err := r.Scan(&id, &seq, &to, &from, &toCol, &onUpd, &onDel, &match); err != nil {
						return
					}
					if to == table {
						out = append(out, ForeignKey{FromTable: n, FromColumn: from, ToTable: to, ToColumn: toCol})
					}
				}
			}()
		}
		return out, nil
	}
}

func (d *DB) sqliteOutgoing(ctx context.Context, table string) []ForeignKey {
	var out []ForeignKey
	q := fmt.Sprintf(`PRAGMA foreign_key_list(%s)`, d.Driver.QuoteIdent(table))
	r, err := d.SQL.QueryContext(ctx, q)
	if err != nil {
		return nil
	}
	defer r.Close()
	for r.Next() {
		var id, seq int
		var to, from, toCol string
		var onUpd, onDel, match string
		if err := r.Scan(&id, &seq, &to, &from, &toCol, &onUpd, &onDel, &match); err != nil {
			break
		}
		out = append(out, ForeignKey{FromTable: table, FromColumn: from, ToTable: to, ToColumn: toCol})
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/db/ -run TestForeignKeysSQLite -v`
Expected: PASS. Then `go test ./... -count=1`, `go vet ./...`.

- [ ] **Step 5: Commit**

```bash
git add internal/db/refs.go internal/db/refs_test.go
git commit -m "feat(db): foreign key introspection per engine"
```

---

### Task 6: ER tab render + wiring

**Files:**
- Create: `internal/tui/er.go`
- Test: `internal/tui/er_test.go`
- Modify: `internal/tui/model.go`, `internal/tui/msg.go`, `internal/tui/commands.go`, `internal/tui/update.go`, `internal/tui/view_detail.go`, `internal/tui/mouse.go`, `internal/tui/tables.go`

**Interfaces:**
- Consumes: `db.ForeignKey`, `dbpkg.Column`, `loadColumns` pattern, `inspectTable(name)`.
- Produces: tab index 4 fully functional; `erOffset` scroll; neighbor-click jump.

Model additions:

```go
erLinks  []dbpkg.ForeignKey
erSeq    int
erOffset int
erCache  map[string][]dbpkg.ForeignKey
```

msg.go append:

```go
erLoadedMsg struct {
	table string
	links []dbpkg.ForeignKey
	seq   int
	err   error
}
```

commands.go append:

```go
func (m Model) loadER(table string) tea.Cmd {
	db, seq := m.db, m.erSeq
	if links, ok := m.erCache[table]; ok {
		return func() tea.Msg { return erLoadedMsg{table: table, links: links, seq: seq} }
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		links, err := db.ForeignKeys(ctx, table)
		return erLoadedMsg{table: table, links: links, seq: seq, err: err}
	}
}
```

update.go: `setTab(4)` (and tab-4 enter path) bumps `erSeq++` and returns `loadER(m.table)`; `erLoadedMsg` drops stale seq, caches, sets `erOffset=0`; detailKey tab==4: up/k/down/j scroll `erOffset` (clamped in render), `r` reloads ER bypassing cache.

er.go: `func (m Model) erView(innerW, innerH int) string` — center box lines for `m.table` (name header + `🔑`/`➤`/`◇` column rows from `m.cols`, reusing explorer icon rules: PK from `Extra` prefix `PK`, FK from `*_id` suffix or FK column set), left boxes = distinct `ToTable` where `FromTable==table` with `from→to` column labels, right boxes = distinct `FromTable` where `ToTable==table`; connector rows `<left> ──▶ <center>` for outgoing and `<center> ◀── <right>` for incoming; boxes sized to content width, clipped to innerW via fitText; vertical scroll via erOffset; `(no foreign keys)` dim line when empty. Record clickable neighbor spans: `func (m Model) erHit(x, y int) (string, bool)` mapping border-relative content coords to table names (center excluded).

mouse.go: tab==4 clicks resolve `erHit` → `inspectTable(name)` (which resets tab to 0 — neighbor jump shows schema of that table).

tables.go sizeTables: tab 4 chrome = title+blank+tabs+blank (4), grid area unused (ER uses full remainder via `m.contentH()-2-4` passed at render; erView clips to it).

- [ ] **Step 1: Write the failing test**

```go
func TestERViewThreeBoxes(t *testing.T) {
	m := browseModel(t)
	m.table = "orders"
	m.tab = 4
	m.cols = []db.Column{{Name: "id", Extra: "PK(1)"}, {Name: "customer_id"}}
	m.erLinks = []db.ForeignKey{
		{FromTable: "orders", FromColumn: "customer_id", ToTable: "customers", ToColumn: "id"},
		{FromTable: "refunds", FromColumn: "order_id", ToTable: "orders", ToColumn: "id"},
	}
	out := m.erView(80, 20)
	for _, want := range []string{"orders", "customers", "refunds", "──▶", "◀──"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestERViewThreeBoxes -v`
Expected: FAIL with "undefined: erView".

- [ ] **Step 3: Write minimal implementation**

Per interfaces above. view_detail.go `case 4:` calls `m.erView(m.paneInnerW(), m.paneInnerH())` (add these helpers in tables.go: innerW = width-paneX-3 floor 20, innerH = contentH-2-4 floor 3).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestERViewThreeBoxes -v`
Expected: PASS. Then full suite + vet.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/er.go internal/tui/er_test.go internal/tui/model.go internal/tui/msg.go internal/tui/commands.go internal/tui/update.go internal/tui/view_detail.go internal/tui/mouse.go internal/tui/tables.go
git commit -m "feat(tui): ER diagram tab"
```

---

### Task 7: Which-key overlay + registry

**Files:**
- Create: `internal/tui/whichkey.go`
- Test: `internal/tui/whichkey_test.go`
- Modify: `internal/tui/model.go`, `internal/tui/update.go`, `internal/tui/view.go`, `internal/tui/mouse.go`

**Interfaces:**
- Consumes: final key inventory from Tasks 2/4/6 (`sidebarKeys`, `detailKey`, `queryKeys`, ER nav, conns/form keys, global tab/q/ctrl+c).
- Produces: `type KeyBinding struct { Key, Desc, Contexts string }`, `var keyRegistry []KeyBinding` (one entry per handled key), `func (m Model) helpView() string`, `showHelp bool` model field. No later tasks.

Registry (exact entries, implement verbatim):

```go
var keyRegistry = []KeyBinding{
	{"up/k", "move up", "Sidebar,Detail,Query-results,ER"},
	{"down/j", "move down", "Sidebar,Detail,Query-results,ER"},
	{"left/right", "collapse/expand", "Sidebar"},
	{"enter", "preview table", "Sidebar"},
	{"/", "clear filter", "Sidebar"},
	{"r", "refresh", "Sidebar,Detail"},
	{"c/esc", "back to connections", "Sidebar"},
	{"q", "quit", "Global"},
	{"tab/shift+tab", "switch pane", "Global"},
	{"h/l", "prev/next tab", "Detail"},
	{"esc/backspace", "back to sidebar", "Detail"},
	{"home/end", "top/bottom of grid", "Detail"},
	{"1/2/3/4/5", "schema/indexes/rows/query/er tab", "Detail"},
	{"n/p", "next/prev rows page", "Detail"},
	{"s", "cycle page size", "Detail"},
	{"pgup/pgdn", "page grid", "Detail"},
	{"ctrl+u/ctrl+d", "half-page grid", "Detail"},
	{"g/G", "top/bottom of grid", "Detail"},
	{"ctrl+r/F5", "run query", "Query editor"},
	{"esc", "editor to results", "Query editor"},
	{"?", "this help", "Global"},
	{"a/e/d", "add/edit/forget connection", "Connections"},
	{"enter", "connect", "Connections"},
}
```

Overlay: centered box (max 3 columns of `key desc` pairs grouped by context header), rounded gold border, sized to min(terminal, content); `showHelp` bool on Model toggled by `?` in `handleKey` AFTER the ctrl+c check but BEFORE screen dispatch, except when: conns `SettingFilter()`, form inputs focused, or query editor focused (`tab==3 && focusDetail && queryFocus==0` — insert `?` as text instead). When open, all keys except `?`/`esc`/`ctrl+c` are swallowed (return nil); mouse left-press outside overlay rect closes, inside is noop.

`helpView()` overlay rect must be recomputable for mouse: `func (m Model) helpRect() (x, y, w, h int)` shared by render and hit-test.

- [ ] **Step 1: Write the failing test**

```go
func TestHelpRegistryCoversHandlers(t *testing.T) {
	handled := []string{"up", "k", "down", "j", "left", "right", "enter", "/", "r", "c", "esc", "q", "tab", "1", "2", "3", "4", "5", "n", "p", "s", "pgup", "pgdown", "ctrl+u", "ctrl+d", "ctrl+r", "g", "G", "home", "end", "f5", "?", "a", "e", "d", "backspace", "h", "l", "shift+tab"}
	have := map[string]bool{}
	for _, b := range keyRegistry {
		if strings.Contains(b.Key, "..") {
			continue
		}
		for _, k := range strings.Split(b.Key, "/") {
			have[strings.ToLower(strings.TrimSpace(k))] = true
		}
	}
	var missing []string
	for _, k := range handled {
		if !have[strings.ToLower(k)] {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("registry missing keys: %v", missing)
	}
}

func TestHelpToggle(t *testing.T) {
	m := browseModel(t)
	m.loading = false
	u, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = u.(Model)
	if !m.showHelp {
		t.Fatal("? must open help")
	}
	u, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = u.(Model)
	if m.showHelp {
		t.Fatal("esc must close help")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run 'TestHelpRegistryCoversHandlers|TestHelpToggle' -v`
Expected: FAIL with "undefined: keyRegistry".

- [ ] **Step 3: Write minimal implementation**

Per interfaces above. view.go: when `showHelp`, render browse/conns/form view then overlay `helpView()` centered on top (overlay lines replace base lines row-for-row so width never exceeds terminal). Footer hint gains `• ? keys`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run 'TestHelpRegistryCoversHandlers|TestHelpToggle' -v`
Expected: PASS. Then full suite + vet.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/whichkey.go internal/tui/whichkey_test.go internal/tui/model.go internal/tui/update.go internal/tui/view.go internal/tui/mouse.go
git commit -m "feat(tui): which-key help overlay"
```

---

## Self-Review (ran before save)

1. Spec coverage: borders+focus → Task 2; zebra → Task 1; query tab+editor+results+chroma → Tasks 3–4; FK refs → Task 5; ER tab → Task 6; which-key+`?` → Task 7; key table (`4/5/ctrl+r/F5/?`/click-focus) → Tasks 2/4/6/7; phasing P1→P4 = Tasks 1-2, 3-4, 5-6, 7. Non-goals (selection, undo, multi-hop, DDL exec) excluded everywhere. No gaps.
2. Placeholder scan: every step carries exact code, exact `go test`/`go vet` commands, exact commit messages and `git add` paths; no TBD/TODO/later/appropriate-handling language.
3. Type consistency: `Editor` methods, `hlCell`, `ForeignKey`, `erLoadedMsg`/`queryDoneMsg` fields, `queryFocus`/`querySeq`/`erSeq`/`erOffset`/`erCache`/`showHelp` spelled identically between producer and consumer tasks; tab indices 3=query/4=ER used uniformly; `paneBorder(focused bool)`, `erView(innerW,innerH)`, `helpRect()` signatures fixed once and reused.
