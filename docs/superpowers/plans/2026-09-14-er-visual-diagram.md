# Visual ER Diagram Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace tab 5 text ER with a full-schema DBeaver-style box canvas (boxes + dashed FK connectors, pan + select).

**Architecture:** New `db.AllForeignKeys` aggregates all FKs; new `erSchema` state + `loadERSchema` async command feed a new `er_canvas.go` (grid layout, box render, orthogonal connectors, viewport slicing); existing `erView`/`erHit`/keys/mouse delegate to canvas.

**Tech Stack:** Go 1.25, Bubbletea, Lipgloss, x/ansi (existing deps only — no new modules).

## Global Constraints

- Go 1.25.0 per `go.mod`; no new dependencies.
- View stays pure: never mutate state in render; width math via `lipgloss.Width`.
- Rendered lines never wrap: cap with `fitText` / `ansi.Truncate` (mouse-row contract).
- Follow existing MVU patterns: state in `model.go`, messages in `msg.go`, side effects in `commands.go`, transitions in `update.go`/`mouse.go`.
- Box borders: blue `#005FD7` normal, gold `#CA8A04` selected (matches explorer selection).
- Connectors drawn under boxes in dim blue; boxes always win hit-tests.
- `go test ./...` green after every task.

---

### Task 1: `db.AllForeignKeys` aggregation + dedupe

**Files:**
- Modify: `internal/db/refs.go`
- Test: `internal/db/refs_test.go`

**Interfaces:**
- Consumes: existing `func (d *DB) ForeignKeys(ctx context.Context, table string) ([]ForeignKey, error)`, existing `func (d *DB) ListTables(ctx context.Context) ([]string, error)`
- Produces: `func (d *DB) AllForeignKeys(ctx context.Context, tables []string) ([]ForeignKey, error)` — deduped by `FromTable|FromColumn|ToTable|ToColumn`; empty input returns `nil, nil`

- [ ] **Step 1: Write the failing test**

```go
func TestAllForeignKeysDedupe(t *testing.T) {
	d := openFKMem(t)
	defer d.SQL.Close()
	out, err := d.AllForeignKeys(context.Background(), []string{"customers", "orders"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("want 1 deduped fk, got %v", out)
	}
	fk := out[0]
	if fk.FromTable != "orders" || fk.ToTable != "customers" {
		t.Fatalf("wrong fk %v", fk)
	}
	if out2, _ := d.AllForeignKeys(context.Background(), nil); len(out2) != 0 {
		t.Fatalf("want empty for nil tables, got %v", out2)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestAllForeignKeysDedupe -v`
Expected: FAIL with "undefined: AllForeignKeys"

- [ ] **Step 3: Write minimal implementation**

In `internal/db/refs.go` append:

```go
// AllForeignKeys aggregates ForeignKeys across tables and dedupes pairs.
// Empty input returns nil. Order: first-seen table order, then FK order.
func (d *DB) AllForeignKeys(ctx context.Context, tables []string) ([]ForeignKey, error) {
	if len(tables) == 0 {
		return nil, nil
	}
	seen := map[string]bool{}
	var out []ForeignKey
	for _, t := range tables {
		fks, err := d.ForeignKeys(ctx, t)
		if err != nil {
			return nil, err
		}
		for _, fk := range fks {
			k := fk.FromTable + "|" + fk.FromColumn + "|" + fk.ToTable + "|" + fk.ToColumn
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, fk)
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/db/ -v`
Expected: PASS (all 3+ tests)

- [ ] **Step 5: Commit**

```bash
git add internal/db/refs.go internal/db/refs_test.go
git commit -m "feat(db): add AllForeignKeys aggregation with dedupe"
```

### Task 2: `erSchema` state + schema-loaded message

**Files:**
- Modify: `internal/tui/model.go:73-76`
- Modify: `internal/tui/msg.go:52-57`

**Interfaces:**
- Consumes: `dbpkg.Column`, `dbpkg.ForeignKey` types
- Produces: `type erTable struct { name string; cols []dbpkg.Column; pk, fk map[string]bool }`, `type erSchemaState struct { tables []erTable; links []dbpkg.ForeignKey; loaded bool; err string }` on `Model.erSchema`; `type erSchemaLoadedMsg struct { tables []erTable; links []dbpkg.ForeignKey; seq int; err error }`; `Model.erPanX, erPanY int`, `Model.erSel, Model.hoverER string`

- [ ] **Step 1: Write the failing test**

In `internal/tui/er_test.go` append (temporarily failing — types missing):

```go
func TestERSchemaStateDefaults(t *testing.T) {
	m := browseModel(t)
	if m.erSchema.loaded {
		t.Fatal("erSchema should start unloaded")
	}
	if m.erPanX != 0 || m.erPanY != 0 {
		t.Fatal("pan should start at 0,0")
	}
}
```

Helper `browseModel(t)` already exists in test suite (used by `TestERViewThreeBoxes`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestERSchemaStateDefaults -v`
Expected: FAIL with "m.erSchema undefined"

- [ ] **Step 3: Write minimal implementation**

In `internal/tui/model.go` after `erCache` field add:

```go
erSchema erSchemaState
erPanX   int
erPanY   int
erSel    string
hoverER  string
```

Above `type Model struct` closing brace, add:

```go
// erTable is one box on the ER canvas.
type erTable struct {
	name string
	cols []dbpkg.Column
	pk   map[string]bool
	fk   map[string]bool
}

// erSchemaState is the full-schema ER cache (one load per session).
type erSchemaState struct {
	tables []erTable
	links  []dbpkg.ForeignKey
	loaded bool
	err    string
}
```

In `internal/tui/msg.go` append:

```go
erSchemaLoadedMsg struct {
	tables []erTable
	links  []dbpkg.ForeignKey
	seq    int
	err    error
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestERSchemaStateDefaults -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/msg.go internal/tui/er_test.go
git commit -m "feat(tui): add erSchema state and schema-loaded message"
```

### Task 3: `loadERSchema` command + update + tab wiring

**Files:**
- Modify: `internal/tui/commands.go:130-141`
- Modify: `internal/tui/update.go:203-220`
- Modify: `internal/tui/model.go:261-270` (`setTab`)
- Test: `internal/tui/er_test.go`

**Interfaces:**
- Consumes: `Model.erSchema`, `erSchemaLoadedMsg`, `db.AllForeignKeys`, `db.Columns`, `Model.loadERSchema(schema string, tables []string) tea.Cmd`
- Produces: schema load fills `erSchema` + sets `erSel = m.table`; `setTab(4)` triggers it when unloaded; stale `seq` replies dropped via `erSeq`

- [ ] **Step 1: Write the failing test**

```go
func TestERSchemaLoadedMsgApplies(t *testing.T) {
	m := browseModel(t)
	m.table = "orders"
	m.erSeq = 7
	msg := erSchemaLoadedMsg{
		tables: []erTable{{name: "orders"}, {name: "customers"}},
		links:  []dbpkg.ForeignKey{{FromTable: "orders", FromColumn: "customer_id", ToTable: "customers", ToColumn: "id"}},
		seq:    7,
	}
	nm, _ := m.Update(msg)
	got := nm.(Model)
	if !got.erSchema.loaded || len(got.erSchema.tables) != 2 || len(got.erSchema.links) != 1 {
		t.Fatalf("schema not applied: %+v", got.erSchema)
	}
	if got.erSel != "orders" {
		t.Fatalf("erSel = %q, want orders", got.erSel)
	}
	// stale seq dropped
	stale := erSchemaLoadedMsg{tables: []erTable{{name: "x"}}, seq: 6}
	nm2, _ := got.Update(stale)
	if len(nm2.(Model).erSchema.tables) != 2 {
		t.Fatal("stale msg should be dropped")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestERSchemaLoadedMsgApplies -v`
Expected: FAIL with "undefined: erSchemaLoadedMsg"

- [ ] **Step 3: Write minimal implementation**

In `internal/tui/commands.go` after `loadER` add:

```go
// loadERSchema fetches columns for every table plus all FKs (bounded
// concurrency 4, 15s timeout). Tables come from the caller (current schema).
func (m Model) loadERSchema(schema string, tables []string) tea.Cmd {
	db, seq := m.db, m.erSeq
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		type res struct {
			t   string
			cols []dbpkg.Column
			err  error
		}
		ch := make(chan res, len(tables))
		sem := make(chan struct{}, 4)
		for _, t := range tables {
			t := t
			go func() {
				sem <- struct{}{}
				defer func() { <-sem }()
				cols, err := db.Columns(ctx, t)
				ch <- res{t: t, cols: cols, err: err}
			}()
		}
		byName := map[string][]dbpkg.Column{}
		var firstErr error
		for range tables {
			r := <-ch
			if r.err != nil && firstErr == nil {
				firstErr = r.err
				continue
			}
			byName[r.t] = r.cols
		}
		links, err := db.AllForeignKeys(ctx, tables)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		_ = schema // scoping is explorer-side; db calls take plain table names
		var out []erTable
		for _, t := range tables {
			cols := byName[t]
			pk := map[string]bool{}
			fk := map[string]bool{}
			for _, c := range cols {
				if len(c.Extra) >= 2 && c.Extra[:2] == "PK" {
					pk[c.Name] = true
				}
			}
			for _, l := range links {
				if l.FromTable == t {
					fk[l.FromColumn] = true
				}
			}
			out = append(out, erTable{name: t, cols: cols, pk: pk, fk: fk})
		}
		return erSchemaLoadedMsg{tables: out, links: links, seq: seq, err: firstErr}
	}
}
```

In `internal/tui/update.go` after the `erLoadedMsg` case add:

```go
case erSchemaLoadedMsg:
	if msg.seq != m.erSeq {
		return m, nil // superseded
	}
	if msg.err != nil {
		m.err = msg.err.Error()
		m.erSchema.err = msg.err.Error()
		return m, nil
	}
	m.erSchema = erSchemaState{tables: msg.tables, links: msg.links, loaded: true}
	m.erSel = m.table
	m.erPanX, m.erPanY = 0, 0
	m.err = ""
	return m, nil
```

In `internal/tui/model.go` `setTab`, replace the `if i == 4` branch:

```go
if i == 4 {
	m.erSeq++
	m.sizeTables()
	if m.erSchema.loaded {
		return nil
	}
	var names []string
	for _, s := range m.explorer.Schemas {
		for _, tb := range s.Tables {
			names = append(names, tb.Name)
		}
	}
	if len(names) == 0 && m.table != "" {
		names = []string{m.table}
	}
	return m.loadERSchema("", names)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run 'TestERSchema|TestERView' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/commands.go internal/tui/update.go internal/tui/model.go internal/tui/er_test.go
git commit -m "feat(tui): load full ER schema with columns and FKs"
```

### Task 4: Canvas box model, grid layout, box render

**Files:**
- Create: `internal/tui/er_canvas.go`
- Test: `internal/tui/er_test.go`

**Interfaces:**
- Consumes: `erTable`, `erSchemaState`
- Produces: `type erRect struct { x, y, w, h int }`, `func erBoxWidth(t erTable) int`, `func erBoxHeight(t erTable, total int) int`, `func erGridLayout(tables []erTable, innerW, innerH int) map[string]erRect`, `func erTypeHint(sqlType string) string // ABC|123|◷|raw`, `func erBoxLines(t erTable, total int, selected bool) []string`

Box rules (from spec): width `min(28, max(header, rows))`; height `1 + min(N, cap)` where cap = all cols if `total <= 40` else PK/FK in table order + first 8 non-key + `… M more`. Header `▦ name`. Rows `🔑 pk` bold/green, `➤ fk`, `◇ plain` + dim type hint right-aligned.

- [ ] **Step 1: Write the failing test**

```go
func TestERCanvasBoxes(t *testing.T) {
	tables := []erTable{
		{name: "customers", cols: []dbpkg.Column{{Name: "id", Type: "integer", Extra: "PK(1)"}, {Name: "name", Type: "text"}}, pk: map[string]bool{"id": true}},
		{name: "orders", cols: []dbpkg.Column{{Name: "id", Type: "integer", Extra: "PK(1)"}, {Name: "customer_id", Type: "integer"}}, pk: map[string]bool{"id": true}, fk: map[string]bool{"customer_id": true}},
	}
	pos := erGridLayout(tables, 80, 20)
	if len(pos) != 2 {
		t.Fatalf("want 2 boxes, got %v", pos)
	}
	lines := erBoxLines(tables[1], 2, false)
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"orders", "🔑", "➤", "123"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in:\n%s", want, joined)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestERCanvasBoxes -v`
Expected: FAIL with "undefined: erGridLayout"

- [ ] **Step 3: Write minimal implementation**

Create `internal/tui/er_canvas.go`:

```go
package tui

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	dbpkg "db-peek/internal/db"
)

// erRect is a box origin + size in canvas cells.
type erRect struct{ x, y, w, h int }

func erTypeHint(sqlType string) string {
	s := strings.ToLower(sqlType)
	switch {
	case strings.Contains(s, "int") || strings.Contains(s, "serial") || strings.Contains(s, "numeric") || strings.Contains(s, "decimal") || strings.Contains(s, "float") || strings.Contains(s, "double") || strings.Contains(s, "real"):
		return "123"
	case strings.Contains(s, "char") || strings.Contains(s, "text") || strings.Contains(s, "varchar") || strings.Contains(s, "name"):
		return "ABC"
	case strings.Contains(s, "time") || strings.Contains(s, "date") || strings.Contains(s, "stamp"):
		return "◷"
	case strings.Contains(s, "bool"):
		return "◯"
	default:
		if sqlType == "" {
			return ""
		}
		return fitText(sqlType, 8)
	}
}

func erVisibleCols(t erTable, total int) ([]dbpkg.Column, int) {
	if total <= 40 || len(t.cols) <= 12 {
		return t.cols, 0
	}
	var keys, rest []dbpkg.Column
	for _, c := range t.cols {
		if t.pk[c.Name] || t.fk[c.Name] {
			keys = append(keys, c)
		} else {
			rest = append(rest, c)
		}
	}
	out := append([]dbpkg.Column{}, keys...)
	more := 0
	for _, c := range rest {
		if len(out) >= len(keys)+8 {
			more++
			continue
		}
		out = append(out, c)
	}
	return out, more
}

func erBoxWidth(t erTable) int {
	w := lipgloss.Width("▦ " + t.name)
	for _, c := range t.cols {
		if rw := lipgloss.Width(c.Name) + 6; rw > w {
			w = rw
		}
	}
	if w > 28 {
		w = 28
	}
	if w < 12 {
		w = 12
	}
	return w
}

func erBoxHeight(t erTable, total int) int {
	cols, more := erVisibleCols(t, total)
	h := 1 + len(cols)
	if more > 0 {
		h++
	}
	return h
}

func erBoxLines(t erTable, total int, selected bool) []string {
	cols, more := erVisibleCols(t, total)
	w := erBoxWidth(t)
	head := fitText("▦ "+t.name, w)
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#005FD7")).Width(w)
	if selected {
		style = style.BorderForeground(lipgloss.Color("#CA8A04"))
	}
	var rows []string
	for _, c := range cols {
		icon := "◇"
		switch {
		case t.pk[c.Name]:
			icon = "🔑"
		case t.fk[c.Name]:
			icon = "➤"
		}
		hint := erTypeHint(c.Type)
		left := fitText(icon+" "+c.Name, w-len(hint)-1)
		gap := w - lipgloss.Width(left) - lipgloss.Width(hint)
		if gap < 1 {
			gap = 1
		}
		row := left + strings.Repeat(" ", gap) + dimStyle.Render(hint)
		if t.pk[c.Name] {
			row = dataSelectedStyle.Render(left) + strings.Repeat(" ", gap) + dimStyle.Render(hint)
		}
		rows = append(rows, row)
	}
	if more > 0 {
		rows = append(rows, dimStyle.Render(fitText("… "+strconv.Itoa(more)+" more", w)))
	}
	box := style.Render(strings.Join(append([]string{head}, rows...), "\n"))
	return strings.Split(box, "\n")
}

// erGridLayout places boxes deterministically: sort by name, grid columns
// from pane aspect, gaps gx=4 gy=2. Uses max box size for cell stride.
func erGridLayout(tables []erTable, innerW, innerH int) map[string]erRect {
	cp := append([]erTable(nil), tables...)
	sort.Slice(cp, func(i, j int) bool { return cp[i].name < cp[j].name })
	n := len(cp)
	if n == 0 {
		return map[string]erRect{}
	}
	if innerH < 1 {
		innerH = 1
	}
	ar := float64(innerW) / float64(innerH) / 2.0
	if ar < 1.0 {
		ar = 1.0
	}
	cols := int(0.5 + math.Sqrt(float64(n))*ar)
	if cols < 1 {
		cols = 1
	}
	if cols > n {
		cols = n
	}
	maxW, maxH := 0, 0
	ws := make([]int, n)
	hs := make([]int, n)
	for i, t := range cp {
		ws[i] = erBoxWidth(t)
		hs[i] = erBoxHeight(t, n)
		if ws[i] > maxW {
			maxW = ws[i]
		}
		if hs[i] > maxH {
			maxH = hs[i]
		}
	}
	pos := map[string]erRect{}
	for i, t := range cp {
		r, c := i/cols, i%cols
		pos[t.name] = erRect{x: c * (maxW + 4), y: r * (maxH + 2), w: ws[i], h: hs[i]}
	}
	return pos
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestERCanvasBoxes -v`
Expected: PASS (fix imports/stub until green)

- [ ] **Step 5: Commit**

```bash
git add internal/tui/er_canvas.go internal/tui/er_test.go
git commit -m "feat(tui): ER canvas boxes with grid layout and type hints"
```

### Task 5: Connectors + full canvas render + viewport

**Files:**
- Modify: `internal/tui/er_canvas.go`
- Test: `internal/tui/er_test.go`

**Interfaces:**
- Consumes: `erGridLayout`, `erBoxLines`, `Model.erSchema`, `Model.erPanX/Y`
- Produces: `func erRenderCanvas(tables []erTable, links []dbpkg.ForeignKey, innerW, innerH int, sel string) []string` (full canvas rows), `func erSliceViewport(canvas []string, panX, panY, w, h int) []string`, `func (m Model) erCanvasView(innerW, innerH int) string`

Connector rule: L-path from right-edge midpoint of from-box to left-edge midpoint of to-box; same-row → direct `┄`; else exit right → vertical `┆` at mid-gap → enter left; self-ref loops on right edge. Drawn first (under boxes) in dim blue.

- [ ] **Step 1: Write the failing test**

```go
func TestERCanvasConnectors(t *testing.T) {
	tables := []erTable{
		{name: "customers", cols: []dbpkg.Column{{Name: "id", Extra: "PK(1)"}}, pk: map[string]bool{"id": true}},
		{name: "orders", cols: []dbpkg.Column{{Name: "id", Extra: "PK(1)"}, {Name: "customer_id"}}, pk: map[string]bool{"id": true}, fk: map[string]bool{"customer_id": true}},
	}
	links := []dbpkg.ForeignKey{{FromTable: "orders", FromColumn: "customer_id", ToTable: "customers", ToColumn: "id"}}
	canvas := erRenderCanvas(tables, links, 80, 20, "orders")
	joined := strings.Join(canvas, "\n")
	if !strings.Contains(joined, "┄") && !strings.Contains(joined, "┆") {
		t.Fatalf("missing connector chars in:\n%s", joined)
	}
	for _, want := range []string{"customers", "orders"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing box %q", want)
		}
	}
}

func TestERPanClamp(t *testing.T) {
	m := browseModel(t)
	m.erPanX, m.erPanY = 9999, 9999
	out := m.erCanvasView(40, 10)
	if out == "" {
		t.Fatal("viewport should render even when pan is out of range")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run 'TestERCanvasConnectors|TestERPanClamp' -v`
Expected: FAIL with "undefined: erRenderCanvas"

- [ ] **Step 3: Write minimal implementation**

Add to `internal/tui/er_canvas.go`:

```go
// erRenderCanvas draws connectors first then boxes over them; returns all
// canvas rows (unclipped). Caller slices via erSliceViewport.
func erRenderCanvas(tables []erTable, links []dbpkg.ForeignKey, innerW, innerH int, sel string) []string {
	if len(tables) == 0 {
		return []string{"(no tables)"}
	}
	pos := erGridLayout(tables, innerW, innerH)
	cw, chh := 0, 0
	for _, r := range pos {
		if r.x+r.w > cw {
			cw = r.x + r.w
		}
		if r.y+r.h > chh {
			chh = r.y + r.h
		}
	}
	grid := make([][]rune, chh+2)
	for i := range grid {
		grid[i] = []rune(strings.Repeat(" ", cw+8))
	}
	set := func(x, y int, ch rune) {
		if y < 0 || y >= len(grid) || x < 0 || x >= len(grid[y]) {
			return
		}
		grid[y][x] = ch
	}
	linkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	_ = linkStyle
	for _, l := range links {
		a, okA := pos[l.FromTable]
		b, okB := pos[l.ToTable]
		if !okA || !okB {
			continue
		}
		y1 := a.y + a.h/2
		y2 := b.y + b.h/2
		x1 := a.x + a.w
		x2 := b.x
		if l.FromTable == l.ToTable {
			for x := x1; x < x1+3; x++ {
				set(x, y1, '┄')
			}
			continue
		}
		if y1 == y2 {
			step := 1
			if x2 < x1 {
				step = -1
			}
			for x := x1; x != x2; x += step {
				set(x, y1, '┄')
			}
			continue
		}
		mid := (x1 + x2) / 2
		if mid <= x1 && x2 > x1 {
			mid = x1 + 2
		}
		stepX := 1
		if mid < x1 {
			stepX = -1
		}
		for x := x1; x != mid; x += stepX {
			set(x, y1, '┄')
		}
		top, bot := y1, y2
		if bot < top {
			top, bot = bot, top
		}
		for y := top; y <= bot; y++ {
			if y == y1 {
				continue
			}
			set(mid, y, '┆')
		}
		step := 1
		if x2 < mid {
			step = -1
		}
		for x := mid; x != x2; x += step {
			set(x, y2, '┄')
		}
	}
	rows := make([]string, len(grid))
	for i, r := range grid {
		rows[i] = strings.TrimRight(string(r), " ")
	}
	// Overlay boxes (box wins over connectors).
	type item struct {
		name string
		r    erRect
	}
	var items []item
	for _, t := range tables {
		items = append(items, item{t.name, pos[t.name]})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].r.y < items[j].r.y })
	byName := map[string]erTable{}
	for _, t := range tables {
		byName[t.name] = t
	}
	for _, it := range items {
		bl := erBoxPlainLines(byName[it.name], len(tables))
		for dy, ln := range bl {
			y := it.r.y + dy
			if y < 0 || y >= len(rows) {
				continue
			}
			nr := []rune(rows[y])
			need := it.r.x + lipgloss.Width(ln) + 1
			for len(nr) < need {
				nr = append(nr, ' ')
			}
			copy(nr[it.r.x:], []rune(ln))
			rows[y] = string(nr)
		}
	}
	return rows
}

// erBoxPlainLines is the geometry twin of erBoxLines without ANSI styles,
// used for canvas overlay so box cells overwrite connectors exactly.
func erBoxPlainLines(t erTable, total int) []string {
	cols, more := erVisibleCols(t, total)
	w := erBoxWidth(t)
	out := []string{fitText("▦ "+t.name, w)}
	for _, c := range cols {
		icon := "◇"
		switch {
		case t.pk[c.Name]:
			icon = "🔑"
		case t.fk[c.Name]:
			icon = "➤"
		}
		out = append(out, fitText(icon+" "+c.Name+" "+erTypeHint(c.Type), w))
	}
	if more > 0 {
		out = append(out, fitText("… "+strconv.Itoa(more)+" more", w))
	}
	return out
}

func erSliceViewport(canvas []string, panX, panY, w, h int) []string {
	if h < 1 {
		h = 1
	}
	if panY < 0 {
		panY = 0
	}
	if panX < 0 {
		panX = 0
	}
	if panY > len(canvas)-1 {
		panY = len(canvas) - 1
	}
	if panY < 0 {
		panY = 0
	}
	out := []string{}
	for i := panY; i < panY+h && i < len(canvas); i++ {
		start := panX
		if start > len([]rune(canvas[i])) {
			start = len([]rune(canvas[i]))
		}
		out = append(out, fitText(string([]rune(canvas[i]))[start:], w))
	}
	for len(out) < h {
		out = append(out, "")
	}
	return out
}
```

Also add:

```go
func (m Model) erCanvasView(innerW, innerH int) string {
	if !m.erSchema.loaded {
		if m.loading {
			return "loading..."
		}
		return "(no ER data — press 5 to load)"
	}
	canvas := erRenderCanvas(m.erSchema.tables, m.erSchema.links, innerW, innerH, m.erSel)
	lines := erSliceViewport(canvas, m.erPanX, m.erPanY, innerW, innerH)
	if len(m.erSchema.links) == 0 && len(m.erSchema.tables) > 0 {
		lines = append(lines, dimStyle.Render("(no foreign keys — boxes only)"))
		lines = lines[len(lines)-innerH:]
	}
	return strings.Join(lines, "\n")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run 'TestERCanvas|TestERPan' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/er_canvas.go internal/tui/er_test.go
git commit -m "feat(tui): ER canvas connectors and viewport rendering"
```

### Task 6: Wire `erView`/`erHit`/keys/mouse to canvas

**Files:**
- Modify: `internal/tui/er.go:143-193`
- Modify: `internal/tui/update.go:639-681` (`erKeys`)
- Modify: `internal/tui/mouse.go:168-175,381-383,413-419` (wheel, hover, click)
- Test: `internal/tui/er_test.go`

**Interfaces:**
- Consumes: `erCanvasView`, `erGridLayout`, `Model.erPanX/Y`, `Model.erSel`, `Model.hoverER`, `inspectTable`
- Produces: `erView` delegates to canvas when `erSchema.loaded` (fallback legacy otherwise); `erHit(x,y)` maps viewport→canvas→box; `erKeys` pans with arrows/WASD/hjkl, `Tab` cycles, `Enter` inspects, `r` reloads schema; wheel pans vertically; click selects + double-Enter inspects

- [ ] **Step 1: Write the failing test**

```go
func TestERHitBox(t *testing.T) {
	m := browseModel(t)
	m.table = "orders"
	m.erSchema = erSchemaState{loaded: true,
		tables: []erTable{{name: "customers"}, {name: "orders"}},
		links:  []dbpkg.ForeignKey{{FromTable: "orders", FromColumn: "customer_id", ToTable: "customers", ToColumn: "id"}},
	}
	m.width = 120
	m.sidebarW = 34
	name, ok := m.erHit(1, 6)
	if !ok || (name != "customers" && name != "orders") {
		t.Fatalf("want a box hit, got %q ok=%v", name, ok)
	}
	if _, ok := m.erHit(119, 40); ok {
		t.Fatal("gap click should miss")
	}
}

func TestERViewUsesCanvas(t *testing.T) {
	m := browseModel(t)
	m.erSchema = erSchemaState{loaded: true, tables: []erTable{{name: "orders", cols: []dbpkg.Column{{Name: "id", Extra: "PK(1)"}}, pk: map[string]bool{"id": true}}}}
	out := m.erView(60, 12)
	if !strings.Contains(out, "orders") || !strings.Contains(out, "🔑") {
		t.Fatalf("canvas view missing box markers:\n%s", out)
	}
}
```

Also update legacy `TestERViewThreeBoxes` expectations from `──▶`/`◀──` to `🔑`/`➤` box markers in the same edit.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run 'TestERHitBox|TestERView' -v`
Expected: FAIL (old `erHit` spans miss boxes)

- [ ] **Step 3: Write minimal implementation**

In `internal/tui/er.go` replace `erView` and `erHit`:

```go
func (m Model) erView(innerW, innerH int) string {
	if m.erSchema.loaded {
		return m.erCanvasView(innerW, innerH)
	}
	return m.erLegacyView(innerW, innerH)
}
```

Rename existing body to `erLegacyView` (keep byte-for-byte, only rename). Replace `erHit`:

```go
func (m Model) erHit(x, y int) (string, bool) {
	if !m.erSchema.loaded {
		return m.erLegacyHit(x, y)
	}
	pos := erGridLayout(m.erSchema.tables, m.paneInnerW(), m.paneInnerH())
	cx := m.erPanX + x
	cy := (y - detailTableTop) + m.erPanY
	for name, r := range pos {
		if cx >= r.x && cx < r.x+r.w && cy >= r.y && cy < r.y+r.h {
			return name, true
		}
	}
	return "", false
}
```

Rename old `erHit` body to `erLegacyHit`. Keep `erClampOff` for legacy scroll.

In `internal/tui/update.go` replace `erKeys`:

```go
func (m Model) erKeys(_ tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "backspace":
		m.focusDetail = false
		m.err = ""
		m.hoverTab = -1
		return m, nil
	case "tab", "right", "l":
		cmd := m.setTab((m.tab + 1) % 5)
		return m, cmd
	case "shift+tab", "left", "h":
		cmd := m.setTab((m.tab + 4) % 5)
		return m, cmd
	case "1":
		cmd := m.setTab(0)
		return m, cmd
	case "2":
		cmd := m.setTab(1)
		return m, cmd
	case "3":
		cmd := m.setTab(2)
		return m, cmd
	case "4":
		cmd := m.setTab(3)
		return m, cmd
	case "5":
		cmd := m.setTab(4)
		return m, cmd
	case "up", "k", "w":
		if m.erPanY > 0 {
			m.erPanY--
		}
		return m, nil
	case "down", "j", "s":
		m.erPanY++
		return m, nil
	case "a":
		if m.erPanX > 0 {
			m.erPanX -= 2
		}
		return m, nil
	case "d":
		m.erPanX += 2
		return m, nil
	case "pgup":
		m.erPanY -= 10
		if m.erPanY < 0 {
			m.erPanY = 0
		}
		return m, nil
	case "pgdown":
		m.erPanY += 10
		return m, nil
	case "n":
		m.erSel = erNextBox(m.erSchema.tables, m.erSel, 1)
		return m, nil
	case "p":
		m.erSel = erNextBox(m.erSchema.tables, m.erSel, -1)
		return m, nil
	case "enter":
		if m.erSel != "" {
			return m.inspectTable(m.erSel)
		}
		return m, nil
	case "r":
		m.erSchema.loaded = false
		m.erSeq++
		var names []string
		for _, t := range m.erSchema.tables {
			names = append(names, t.name)
		}
		if len(names) == 0 && m.table != "" {
			names = []string{m.table}
		}
		return m, m.loadERSchema("", names)
	}
	if key == "tab" {
		return m, nil
	}
	return m, nil
}
```

Add helper in `er_canvas.go` (`n`/`p` cycle boxes because `tab` is reserved for pane tab-switching):

```go
func erNextBox(tables []erTable, cur string, dir int) string {
	if len(tables) == 0 {
		return cur
	}
	names := make([]string, len(tables))
	for i, t := range tables {
		names[i] = t.name
	}
	sort.Strings(names)
	idx := 0
	for i, n := range names {
		if n == cur {
			idx = i
			break
		}
	}
	idx = (idx + dir + len(names)) % len(names)
	return names[idx]
}
```

In `internal/tui/mouse.go`: wheel case 4 → `m.erPanY += n` clamped at 0 (replace `erOffset` lines); `clickTable` tab-4 branch: on hit set `m.erSel = name` and on double-press inspect (reuse `lastClickAt` 500ms pattern from `clickList`); `hoverTable` tab-4: set `m.hoverER` via `erHit` instead of noop.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -v`
Expected: PASS (update legacy test markers first if red)

- [ ] **Step 5: Commit**

```bash
git add internal/tui/er.go internal/tui/er_canvas.go internal/tui/update.go internal/tui/mouse.go internal/tui/er_test.go
git commit -m "feat(tui): wire ER canvas view, hit-test, pan and select"
```

### Task 7: Polish — empty states, resize clamp, styles, full suite

**Files:**
- Modify: `internal/tui/er_canvas.go`, `internal/tui/tables.go:104-135`, `internal/tui/view_detail.go:82-83`
- Test: `internal/tui/er_test.go`

**Interfaces:**
- Consumes: all prior tasks
- Produces: empty-DB `(no tables)`, FK-less `(no foreign keys — boxes only)` dim hint, pan clamp on resize, `r` hint in ER footer path, green suite

- [ ] **Step 1: Write the failing test**

```go
func TestEREmptyStates(t *testing.T) {
	m := browseModel(t)
	m.erSchema = erSchemaState{loaded: true}
	if out := m.erView(60, 10); !strings.Contains(out, "(no tables)") {
		t.Fatalf("want no-tables hint, got:\n%s", out)
	}
	m.erSchema = erSchemaState{loaded: true, tables: []erTable{{name: "t", cols: []dbpkg.Column{{Name: "id"}}}}}
	if out := m.erView(60, 10); !strings.Contains(out, "(no foreign keys") {
		t.Fatalf("want fk-less hint, got:\n%s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestEREmptyStates -v`
Expected: FAIL (hint text missing)

- [ ] **Step 3: Write minimal implementation**

1. `erRenderCanvas`: first line `if len(tables) == 0 { return []string{"(no tables)"} }` (already in Task 5 — verify).
2. `erCanvasView`: after slicing, if `len(links) == 0 && len(tables) > 0`, append `dimStyle.Render(fitText("(no foreign keys — boxes only)", innerW))` keeping total rows `<= innerH` (drop oldest row).
3. `sizeTables`/`resizeBrowse`: after resize, clamp `erPanX/Y >= 0` (upper clamp happens at render via `erSliceViewport`).
4. `view_detail.go` ER branch: footer already generic; no change unless hint overflows — verify `erView` output rows `<= paneInnerH()`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./...`
Expected: PASS (full suite green)

Run: `go vet ./...`
Expected: clean

- [ ] **Step 5: Commit**

```bash
git add internal/tui/er_canvas.go internal/tui/tables.go internal/tui/view_detail.go internal/tui/er_test.go
git commit -m "feat(tui): polish ER empty states and viewport clamping"
```
