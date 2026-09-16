# SQL Autocomplete Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Auto-popup SQL completion (tables + columns + keywords) with Tab accept in the query editor.

**Architecture:** Pure completion engine in new `internal/tui/complete.go` (word extraction, candidate build, ranking, apply); popup state on `Model` (`showComplete`, `completeIdx`); popup overlays within the fixed 8-row editor budget so `queryResultsTop()` mouse math never shifts; `queryKeys`/`clickQuery` route keys and clicks.

**Tech Stack:** Go, bubbletea, lipgloss (existing palette: field white `15`, type cyan `#67E8F9`, keyword gold `#EAB308`, selection purple `62`).

## Global Constraints

- `queryEditorView` always renders exactly `queryEditorH` (8) lines with popup open or closed.
- `queryResultsTop()` stays `detailTableTop + queryEditorH + 1`; no layout shift.
- Truncation runs on plain text before styling (never slice mid-escape).
- `?` still inserts as text while editing (Postgres JSON rule).
- Existing `esc` editor→results→sidebar chain preserved (popup Esc closes popup first).

---

### Task 1: Completion engine (pure, no UI)

**Files:**
- Create: `internal/tui/complete.go`
- Test: `internal/tui/complete_test.go`

**Interfaces:**
- Consumes: `Explorer.Schemas[].Tables[].Columns[]ColumnNode{Name}`, current table `string`, current columns `[]dbpkg.Column{Name}`, editor full text `string`.
- Produces: `type completeItem struct { Text, Detail, Kind string }` (`Kind`: `"table"|"column"|"keyword"`); `func completeWord(line string, col int) (prefix string, start int)`; `func completeCandidates(prefix, curTable, editorText string, tables []string, colsByTable map[string][]string, curCols []string) []completeItem`; `func applyCompletion(line string, col int, item completeItem) (string, int)`.

- [ ] **Step 1: Write the failing test**

```go
package tui

import "testing"

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

func TestApplyCompletion(t *testing.T) {
    line, col := applyCompletion("SELECT ord", 10, completeItem{Text: "orders"})
    if line != "SELECT orders" || col != 14 {
        t.Fatalf("got %q,%d", line, col)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run TestComplete -v`
Expected: FAIL with "undefined: completeWord"

- [ ] **Step 3: Write minimal implementation**

```go
package tui

import "strings"

type completeItem struct {
	Text   string
	Detail string
	Kind   string
}

var sqlKeywords = []string{"SELECT", "FROM", "WHERE", "JOIN", "LEFT JOIN", "ON", "GROUP BY", "ORDER BY", "LIMIT", "INSERT", "UPDATE", "DELETE", "CREATE", "TABLE", "INDEX", "AND", "OR", "NOT", "NULL", "AS", "DISTINCT", "COUNT", "SUM", "AVG", "IN", "BETWEEN", "LIKE", "OFFSET", "HAVING", "UNION"}

func isCompleteRune(r rune) bool {
	return r == '_' || r == '$' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
}

func completeWord(line string, col int) (string, int) {
	rs := []rune(line)
	if col > len(rs) {
		col = len(rs)
	}
	if col < 0 {
		col = 0
	}
	start := col
	for start > 0 && isCompleteRune(rs[start-1]) {
		start--
	}
	return string(rs[start:col]), start
}

func dotTableForPrefix(fullText, prefix string) string {
	lower := strings.ToLower(fullText)
	needle := strings.ToLower(prefix)
	idx := strings.LastIndex(lower, needle)
	if idx <= 0 {
		return ""
	}
	if lower[idx-1] != '.' {
		return ""
	}
	end := idx - 1
	start := end
	for start > 0 && isCompleteRune(rune(lower[start-1])) {
		start--
	}
	return fullText[start:end]
}

func completeCandidates(prefix, curTable, editorText string, tables []string, colsByTable map[string][]string, curCols []string) []completeItem {
	lower := strings.ToLower(prefix)
	if t := dotTableForPrefix(editorText, prefix); t != "" {
		var out []completeItem
		for _, c := range colsByTable[strings.ToLower(t)] {
			if strings.HasPrefix(strings.ToLower(c), lower) {
				out = append(out, completeItem{Text: c, Detail: t, Kind: "column"})
			}
		}
		return out
	}
	var pref, substr []completeItem
	add := func(it completeItem) {
		if strings.HasPrefix(strings.ToLower(it.Text), lower) {
			pref = append(pref, it)
		} else if lower != "" && strings.Contains(strings.ToLower(it.Text), lower) {
			substr = append(substr, it)
		}
	}
	seen := map[string]bool{}
	for _, tb := range tables {
		if seen[strings.ToLower(tb)] {
			continue
		}
		seen[strings.ToLower(tb)] = true
		add(completeItem{Text: tb, Detail: "table", Kind: "table"})
	}
	colSet := map[string]bool{}
	for _, c := range curCols {
		colSet[strings.ToLower(c)] = true
	}
	for _, tb := range referencedTables(editorText) {
		for _, c := range colsByTable[strings.ToLower(tb)] {
			colSet[strings.ToLower(c)] = true
		}
		_ = curTable
	}
	for c := range colSet {
		add(completeItem{Text: c, Detail: "column", Kind: "column"})
	}
	for _, k := range sqlKeywords {
		add(completeItem{Text: k, Detail: "keyword", Kind: "keyword"})
	}
	out := append(pref, substr...)
	if len(out) > 50 {
		out = out[:50]
	}
	return out
}

func applyCompletion(line string, col int, item completeItem) (string, int) {
	rs := []rune(line)
	if col > len(rs) {
		col = len(rs)
	}
	_, start := completeWord(line, col)
	out := string(rs[:start]) + item.Text + string(rs[col:])
	return out, start + len([]rune(item.Text))
}
```

Helper function `referencedTables` (FROM/JOIN regex over editor text, case-insensitive, returns table names as written):

```go
Full versions (part of the same implementation step):

```go
func dotTableForPrefix(fullText, prefix string) string {
	lower := strings.ToLower(fullText)
	needle := strings.ToLower(prefix)
	idx := strings.LastIndex(lower, needle)
	if idx <= 0 {
		return ""
	}
	if lower[idx-1] != '.' {
		return ""
	}
	end := idx - 1
	start := end
	for start > 0 && isCompleteRune(rune(lower[start-1])) {
		start--
	}
	return fullText[start:end]
}
```

```go
func referencedTables(text string) []string {
	var out []string
	upper := strings.ToUpper(text)
	for _, kw := range []string{"FROM", "JOIN", "UPDATE", "INTO"} {
		for i := 0; i+len(kw) <= len(upper); i++ {
			if upper[i:i+len(kw)] != kw {
				continue
			}
			j := i + len(kw)
			for j < len(text) && (text[j] == ' ' || text[j] == '\t' || text[j] == '\n') {
				j++
			}
			s := j
			for j < len(text) && isCompleteRune(rune(text[j])) {
				j++
			}
			if s < j {
				out = append(out, text[s:j])
			}
		}
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run TestComplete -v`
Expected: PASS

- [ ] **Step 5: Run full package tests**

Run: `go test ./internal/tui`
Expected: PASS (no regressions)

- [ ] **Step 6: Commit**

```bash
git add internal/tui/complete.go internal/tui/complete_test.go
git commit -m "feat: add SQL completion engine (tables, columns, keywords)"
```

### Task 2: Popup state + in-budget rendering

**Files:**
- Modify: `internal/tui/model.go:72-77` (add `showComplete bool`, `completeIdx int`, `completeItems []completeItem`)
- Modify: `internal/tui/view_detail.go:127-144` (`queryEditorView` overlay)
- Modify: `internal/tui/styles.go` (reuse `colFieldStyle`, `colTypeStyle`, `sqlKeyword`, `dataSelectedStyle`; add `completeFooterStyle = dimStyle` alias-free: use existing `dimStyle`)
- Test: `internal/tui/complete_test.go` (append view invariant test)

**Interfaces:**
- Consumes: Task 1 `completeWord`, `completeCandidates`, `dotTable`; `Model.editor`, `Model.explorer`, `Model.table`, `Model.cols`.
- Produces: `func (m *Model) refreshCompletion()` (rebuilds `completeItems` from cursor; sets `showComplete`); `func (m Model) completePopupLines(w int) []string` (styled rows, max 5 + footer).

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run TestQueryEditorLineCountWithPopup -v`
Expected: FAIL with "undefined: refreshCompletion" or "undefined: showComplete"

- [ ] **Step 3: Write minimal implementation**

In `model.go`, extend the query block:

```go
editor      Editor
queryFocus  int
querySample *dbpkg.Sample
queryMs     int64
querySeq    int
queryTable  dataTable
// SQL autocomplete popup (query tab only, editor focused).
showComplete   bool
completeIdx    int
completeItems  []completeItem
completeStart  int // rune offset where current prefix starts
completePrefix string
```

In a new section of `complete.go`:

```go
func (m *Model) completionContext() (tables []string, colsByTable map[string][]string, curCols []string) {
	colsByTable = map[string][]string{}
	for _, s := range m.explorer.Schemas {
		for _, tb := range s.Tables {
			tables = append(tables, tb.Name)
			var cs []string
			for _, c := range tb.Columns {
				cs = append(cs, c.Name)
			}
			colsByTable[strings.ToLower(tb.Name)] = cs
		}
	}
	for _, c := range m.cols {
		curCols = append(curCols, c.Name)
	}
	if _, ok := colsByTable[strings.ToLower(m.table)]; !ok && len(curCols) > 0 {
		colsByTable[strings.ToLower(m.table)] = curCols
	}
	return tables, colsByTable, curCols
}

func (m *Model) refreshCompletion() {
	m.showComplete = false
	m.completeItems = nil
	m.completeIdx = 0
	if !(m.focusDetail && m.tab == 3 && m.queryFocus == 0) {
		return
	}
	if m.editor.CurLine < 0 || m.editor.CurLine >= len(m.editor.Lines) {
		return
	}
	line := m.editor.Lines[m.editor.CurLine]
	prefix, start := completeWord(line, m.editor.CurCol)
	if prefix == "" {
		return
	}
	tables, colsByTable, curCols := m.completionContext()
	items := completeCandidates(prefix, m.table, m.editor.Text(), tables, colsByTable, curCols)
	if len(items) == 0 {
		return
	}
	m.showComplete = true
	m.completeItems = items
	m.completeStart = start
	m.completePrefix = prefix
	if m.completeIdx >= len(items) {
		m.completeIdx = 0
	}
}
```

In `view_detail.go` replace `queryEditorView` tail: build the 8 base lines as today, then if `m.showComplete && focused`, overwrite the rows after the cursor line with popup rows:

```go
func (m Model) queryEditorView() string {
	w := m.paneInnerW()
	hl := HighlightSQL(m.editor.Text())
	lines := make([]string, 0, queryEditorH)
	for i := 0; i < queryEditorH; i++ {
		lineIdx := m.editor.OffY + i
		var src string
		if lineIdx < len(m.editor.Lines) {
			src = m.editor.Lines[lineIdx]
		}
		var cells []hlCell
		if lineIdx < len(hl) {
			cells = hl[lineIdx]
		}
		lines = append(lines, m.editorLineView(lineIdx, src, cells, w))
	}
	if m.showComplete && m.focusDetail && m.tab == 3 && m.queryFocus == 0 && len(m.completeItems) > 0 {
		curRel := m.editor.CurLine - m.editor.OffY
		if curRel >= 0 && curRel < queryEditorH-1 {
			pop := m.completePopupLines(w)
			copy(lines[curRel+1:], pop)
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) completePopupLines(w int) []string {
	max := 5
	items := m.completeItems
	more := 0
	if len(items) > max {
		more = len(items) - max
		items = items[:max]
	}
	out := make([]string, 0, max+1)
	for i, it := range items {
		label := "  " + it.Text
		detail := " " + it.Detail
		line := fitText(label, w-len(detail)) + dimStyle.Render(fitText(detail, w))
		switch it.Kind {
		case "keyword":
			line = "  " + sqlKeyword.Render(fitText(it.Text, w-2)) + dimStyle.Render(fitText(detail, w))
		case "table":
			line = "  " + colFieldStyle.Render(fitText(it.Text, w-2)) + dimStyle.Render(fitText(detail, w))
		default:
			line = "  " + colFieldStyle.Render(fitText(it.Text, w-2)) + colTypeStyle.Render(fitText(detail, w))
		}
		if i == m.completeIdx {
			line = dataSelectedStyle.Render(fitText("  "+it.Text+" "+it.Detail, w))
		}
		out = append(out, line)
	}
	if more > 0 {
		out = append(out, dimStyle.Render(fitText("  … "+strconv.Itoa(more)+" more", w)))
	}
	for len(out) < queryEditorH {
		out = append(out, "")
	}
	return out[:queryEditorH]
}
```

(`strconv.Itoa`; add `"strconv"` to the `view_detail.go` imports. Trim `out` at the call site by `copy`, so the exact length logic is: build up to 6 rows, `copy` clips to remaining editor rows.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run TestQueryEditorLineCountWithPopup -v`
Expected: PASS

- [ ] **Step 5: Run package tests**

Run: `go test ./internal/tui`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/tui/model.go internal/tui/complete.go internal/tui/view_detail.go internal/tui/complete_test.go
git commit -m "feat: render SQL autocomplete popup inside editor budget"
```

### Task 3: Key/mouse wiring + invariants

**Files:**
- Modify: `internal/tui/update.go:625-680` (`queryKeys` editor branch)
- Modify: `internal/tui/mouse.go:464-497` (`clickQuery` popup hit-test)
- Modify: `internal/tui/view_detail.go:74-75` (hint line: `"tab complete • esc dismiss • ctrl+r run"`)
- Test: `internal/tui/complete_test.go` (accept, esc two-stage, nav)

**Interfaces:**
- Consumes: Task 1 `applyCompletion`; Task 2 `refreshCompletion`, `showComplete`, `completeIdx`, `completeItems`, `completeStart`.
- Produces: no new exports; behavior: Tab/Enter accept, Esc dismiss-first, Up/Down navigate popup, typing re-filters, Ctrl+R closes popup and runs.

- [ ] **Step 1: Write the failing test**

```go
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
```

Test helper at top of `complete_test.go` (add imports `"strings"` and `tea "github.com/charmbracelet/bubbletea"`):

```go
func keyMsg(k string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}
```

Note: real `tab` arrives as `tea.KeyTab`, not runes; the implementation must match on the `key == "tab"` string parameter (existing `queryKeys` convention), so the helper above suffices for the test harness which calls `queryKeys(msg, "tab")` directly.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui -run TestCompleteAcceptAndEsc -v`
Expected: FAIL (Tab inserts newline or is unhandled)

- [ ] **Step 3: Write minimal implementation**

In `queryKeys`, at the top of the `m.queryFocus == 0` branch, before the existing switch:

```go
if m.queryFocus == 0 && m.showComplete {
	switch key {
	case "esc":
		m.showComplete = false
		m.completeItems = nil
		return m, nil
	case "tab", "enter":
		if m.completeIdx >= 0 && m.completeIdx < len(m.completeItems) {
			it := m.completeItems[m.completeIdx]
			ln := m.editor.CurLine
			newLine, newCol := applyCompletion(m.editor.Lines[ln], m.editor.CurCol, it)
			m.editor.Lines[ln] = newLine
			m.editor.CurCol = newCol
		}
		m.showComplete = false
		m.completeItems = nil
		m.clampEditorScroll()
		return m, nil
	case "up":
		if m.completeIdx > 0 {
			m.completeIdx--
		}
		return m, nil
	case "down":
		if m.completeIdx < len(m.completeItems)-1 {
			m.completeIdx++
		}
		return m, nil
	case "ctrl+r", "f5":
		m.showComplete = false
		m.completeItems = nil
		m.querySeq++
		m.loading = true
		return m, m.runQuery()
	}
}
```

After every mutation of the editor in that branch (`Newline`, `Backspace`, `Delete`, rune insert, `MoveLeft/Right/Up/Down`, `Home/End`), append `m.refreshCompletion()`. Example for the rune path:

```go
if msg.Type == tea.KeyRunes {
	for _, r := range msg.Runes {
		m.editor.Insert(r)
	}
	m.clampEditorScroll()
	m.refreshCompletion()
	return m, nil
}
```

Do the same single-line append for `enter`, `backspace`, `delete`, `up/down/left/right/home/end` (after `clampEditorScroll` where present). `up`/`down` only reach the editor path when the popup is closed (popup-open arrows return early above), so cursor movement still works.

In `clickQuery`, before the existing editor positioning: if `m.showComplete`, compute popup rows `curRel+1 .. curRel+len(visible)`; a click there accepts that item (same `applyCompletion`), closes the popup, and returns. Otherwise fall through to existing cursor positioning + `m.refreshCompletion()` at the end of the editor branch.

Hint line in `view_detail.go` tab 3: change `"ctrl+r run • esc results • e edit"` to `"tab complete • esc dismiss • ctrl+r run • e edit"`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui -run TestComplete -v`
Expected: PASS

- [ ] **Step 5: Run full verification**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add internal/tui/update.go internal/tui/mouse.go internal/tui/view_detail.go internal/tui/complete_test.go
git commit -m "feat: wire SQL autocomplete keys, clicks, and hints"
```
