# Sidebar Explorer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the flat table-list sidebar with a hierarchical explorer (schemas > tables/views > columns) matching the reference image.

**Architecture:** New pure `Explorer` tree model + new `db/schema.go` queries render the sidebar; `Model/Update/Mouse/View` rewired from `bubbles/list` to explorer cursor with lazy async counts.

**Tech Stack:** Go 1.25, bubbletea, bubbles (conns picker only), lipgloss, modernc.org/sqlite / pgx / mysql driver.

## Global Constraints

- Sidebar selection uses gold bg `#CA8A04` black text, never the purple detail selection.
- Every visible tree row is exactly 1 terminal row; `lipgloss.Width(line) <= sidebarW` always.
- Counts are lazy async with `…` placeholder and `?` on error, never blocking tree render.
- `enter` on table both expands and previews detail via existing `inspectTable` + `detailSeq` guard.
- No sequences/roles nodes in v1; SQLite/MySQL map to single synthetic schema.
- Commit after every task passes `go test ./...`.

---

## File Structure

- Create `internal/db/schema.go` — `TableRef{Name, IsView}`, `ListSchemas(ctx)`, `ListTablesInSchema(ctx, schema)`, `ApproxCount(ctx, schema, table)`. Single responsibility: per-engine schema introspection.
- Create `internal/tui/explorer.go` — `ColumnNode`, `TableNode`, `SchemaNode`, `Explorer`, `Row`, `VisibleRows()`, nav (`MoveUp/MoveDown/Toggle/RowAt`), filter (`SetFilter`), `humanizeCount()`, `explorerView()` render. Single responsibility: sidebar tree state + rendering.
- Create `internal/tui/explorer_test.go` — pure model tests, no DB.
- Create `internal/db/schema_test.go` — sqlite `:memory:` integration tests.
- Modify `internal/tui/styles.go` — add `explorerTitle`, `explorerSel`, `explorerType`, `explorerCount`, `explorerConn`.
- Modify `internal/tui/model.go` — replace sidebar `list list.Model` with `explorer Explorer`; remove `tablesItemH`; add filter input state.
- Modify `internal/tui/commands.go` — `loadSchemasCmd`, `loadCountsCmd`, `loadColumnsCmd`.
- Modify `internal/tui/msg.go` — `schemasLoadedMsg`, `tableCountMsg`, `columnsLoadedMsg`.
- Modify `internal/tui/update.go` — connect/tables/counts/columns branches + `sidebarKeys` explorer nav.
- Modify `internal/tui/mouse.go` — `explorerRowAt(y)` hit-test, click/hover/wheel for sidebar.
- Modify `internal/tui/view.go` — left pane uses `explorerView()`; `explorerFirstRow=4`.
- Modify `internal/tui/tables.go` — sidebar default width 30→34.
- Modify `internal/tui/sidebar_test.go` — re-fixture to explorer, keep behavior tests.

---

### Task 1: DB schema layer

**Files:**
- Create: `internal/db/schema.go`
- Test: `internal/db/schema_test.go`

**Interfaces:**
- Consumes: existing `DB{SQL, Driver, Display}` from `internal/db/db.go`, `context.Context`.
- Produces: `type TableRef struct { Name string; IsView bool }`, `func (d *DB) ListSchemas(ctx context.Context) ([]string, error)`, `func (d *DB) ListTablesInSchema(ctx context.Context, schema string) ([]TableRef, error)`, `func (d *DB) ApproxCount(ctx context.Context, schema, table string) (int64, error)` — used by Task 4 commands.

- [ ] **Step 1: Write the failing test**

```go
package db

import (
	"context"
	"database/sql"
	"testing"
)

func openMem(t *testing.T) *DB {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT); CREATE VIEW v_users AS SELECT * FROM users; INSERT INTO users(name) VALUES('a'),('b');`); err != nil {
		t.Fatal(err)
	}
	return &DB{SQL: sqlDB, Driver: SQLite, Display: ":memory:"}
}

func TestListSchemasSQLite(t *testing.T) {
	d := openMem(t)
	defer d.SQL.Close()
	got, err := d.ListSchemas(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "main" {
		t.Fatalf("want [main], got %v", got)
	}
}

func TestListTablesInSchemaSQLite(t *testing.T) {
	d := openMem(t)
	defer d.SQL.Close()
	refs, err := d.ListTablesInSchema(context.Background(), "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 {
		t.Fatalf("want 2 refs, got %v", refs)
	}
}

func TestApproxCountSQLite(t *testing.T) {
	d := openMem(t)
	defer d.SQL.Close()
	n, err := d.ApproxCount(context.Background(), "main", "users")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("want 2, got %d", n)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run 'TestListSchemasSQLite|TestListTablesInSchemaSQLite|TestApproxCountSQLite' -v`
Expected: FAIL with "undefined: ListSchemas" (or TableRef).

- [ ] **Step 3: Write minimal implementation**

```go
package db

import (
	"context"
	"fmt"
)

type TableRef struct {
	Name   string
	IsView bool
}

func (d *DB) ListSchemas(ctx context.Context) ([]string, error) {
	switch d.Driver {
	case Postgres:
		rows, err := d.SQL.QueryContext(ctx, `SELECT schema_name FROM information_schema.schemata WHERE schema_name NOT IN ('pg_catalog','information_schema') ORDER BY 1`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				return nil, err
			}
			out = append(out, s)
		}
		return out, rows.Err()
	case MySQL:
		var name string
		if err := d.SQL.QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&name); err != nil {
			return nil, err
		}
		if name == "" {
			name = d.Display
		}
		return []string{name}, nil
	default:
		return []string{"main"}, nil
	}
}

func (d *DB) ListTablesInSchema(ctx context.Context, schema string) ([]TableRef, error) {
	switch d.Driver {
	case Postgres:
		rows, err := d.SQL.QueryContext(ctx, `
SELECT tablename AS n, false AS v FROM pg_tables WHERE schemaname=$1
UNION ALL SELECT viewname AS n, true AS v FROM pg_views WHERE schemaname=$1
ORDER BY 1`, schema)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []TableRef
		for rows.Next() {
			var r TableRef
			if err := rows.Scan(&r.Name, &r.IsView); err != nil {
				return nil, err
			}
			out = append(out, r)
		}
		return out, rows.Err()
	case MySQL:
		rows, err := d.SQL.QueryContext(ctx, `SELECT table_name, table_type FROM information_schema.tables WHERE table_schema = DATABASE() ORDER BY 1`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []TableRef
		for rows.Next() {
			var name, typ string
			if err := rows.Scan(&name, &typ); err != nil {
				return nil, err
			}
			out = append(out, TableRef{Name: name, IsView: typ == "VIEW"})
		}
		return out, rows.Err()
	default:
		rows, err := d.SQL.QueryContext(ctx, `SELECT name, type FROM sqlite_master WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY name`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []TableRef
		for rows.Next() {
			var name, typ string
			if err := rows.Scan(&name, &typ); err != nil {
				return nil, err
			}
			out = append(out, TableRef{Name: name, IsView: typ == "view"})
		}
		return out, rows.Err()
	}
}

func (d *DB) ApproxCount(ctx context.Context, schema, table string) (int64, error) {
	if d.Driver == Postgres {
		var n int64
		err := d.SQL.QueryRowContext(ctx, `SELECT reltuples::bigint FROM pg_class WHERE relname=$1`, table).Scan(&n)
		if err == nil && n >= 0 {
			return n, nil
		}
	}
	var n int64
	q := fmt.Sprintf(`SELECT COUNT(*) FROM %s`, d.Driver.QuoteIdent(table))
	if err := d.SQL.QueryRowContext(ctx, q).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/db/ -v -count=1`
Expected: PASS all 3 new tests plus existing suite.

- [ ] **Step 5: Commit**

```bash
git add internal/db/schema.go internal/db/schema_test.go
git commit -m "feat(db): schema introspection with per-engine counts"
```

---

### Task 2: Explorer model (nav + filter + humanize)

**Files:**
- Create: `internal/tui/explorer.go` (model section only: types + VisibleRows + nav + filter + humanizeCount; render added in Task 3)
- Test: `internal/tui/explorer_test.go`

**Interfaces:**
- Consumes: `db.TableRef`, `db.Column` (for `FillColumns`), nothing else.
- Produces: `type ColumnNode struct { Name, DataType string; IsPK, IsFK bool }`, `type TableNode struct { Schema, Name string; IsView, Expanded bool; Count int64; CountOK bool; Columns []ColumnNode }`, `type SchemaNode struct { Name string; Expanded bool; Tables []TableNode }`, `type RowKind int (RowSchema, RowTable, RowColumn)`, `type Row struct { Kind RowKind; Schema, Table, Column string; Depth int }`, `type Explorer struct { Schemas []SchemaNode; Cursor int; Filter string }`, `func (e *Explorer) VisibleRows() []Row`, `func (e *Explorer) MoveUp/Down()`, `func (e *Explorer) Toggle()`, `func (e *Explorer) RowAt(i int) (Row, bool)`, `func (e *Explorer) SetFilter(f string)`, `func humanizeCount(n int64) string`, `func NewExplorer(connName string, schemas []string) Explorer` — consumed by Tasks 3–5.

- [ ] **Step 1: Write the failing test**

```go
package tui

import "testing"

func fixtureExplorer() Explorer {
	e := NewExplorer("shop", []string{"public"})
	e.Schemas[0].Expanded = true
	e.Schemas[0].Tables = []TableNode{
		{Schema: "public", Name: "orders", Count: 4000, CountOK: true, Expanded: true, Columns: []ColumnNode{{Name: "id", DataType: "integer", IsPK: true}, {Name: "status", DataType: "text"}}},
		{Schema: "public", Name: "customers", Count: 500, CountOK: true},
	}
	return e
}

func TestVisibleRowsFlatten(t *testing.T) {
	e := fixtureExplorer()
	rows := e.VisibleRows()
	if len(rows) != 5 {
		t.Fatalf("want 5 rows (schema+orders+2 cols+customers), got %d: %v", len(rows), rows)
	}
	if rows[0].Kind != RowSchema || rows[1].Kind != RowTable || rows[2].Kind != RowColumn {
		t.Fatalf("bad order: %v", rows)
	}
}

func TestToggleCollapse(t *testing.T) {
	e := fixtureExplorer()
	e.Cursor = 1
	e.Toggle()
	if len(e.VisibleRows()) != 3 {
		t.Fatalf("collapse orders should hide 2 cols, got %d", len(e.VisibleRows()))
	}
}

func TestHumanizeCount(t *testing.T) {
	cases := map[int64]string{999: "999", 500: "500", 4000: "4.0k", 12000: "12.0k", 2000000: "2.0M"}
	for n, want := range cases {
		if got := humanizeCount(n); got != want {
			t.Fatalf("humanizeCount(%d)=%q want %q", n, got, want)
		}
	}
}

func TestFilterKeepsParents(t *testing.T) {
	e := fixtureExplorer()
	e.SetFilter("cust")
	rows := e.VisibleRows()
	if len(rows) != 2 {
		t.Fatalf("want schema+customers, got %v", rows)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run 'TestVisibleRowsFlatten|TestToggleCollapse|TestHumanizeCount|TestFilterKeepsParents' -v`
Expected: FAIL with "undefined: NewExplorer".

- [ ] **Step 3: Write minimal implementation**

```go
package tui

import (
	"fmt"
	"strings"
)

type RowKind int

const (
	RowSchema RowKind = iota
	RowTable
	RowColumn
)

type ColumnNode struct {
	Name     string
	DataType string
	IsPK     bool
	IsFK     bool
}

type TableNode struct {
	Schema   string
	Name     string
	IsView   bool
	Expanded bool
	Count    int64
	CountOK  bool
	Columns  []ColumnNode
}

type SchemaNode struct {
	Name     string
	Expanded bool
	Tables   []TableNode
}

type Row struct {
	Kind   RowKind
	Schema string
	Table  string
	Column string
	Depth  int
}

type Explorer struct {
	ConnName string
	Schemas  []SchemaNode
	Cursor   int
	Filter   string
}

func NewExplorer(connName string, schemas []string) Explorer {
	e := Explorer{ConnName: connName}
	for i, s := range schemas {
		e.Schemas = append(e.Schemas, SchemaNode{Name: s, Expanded: i == 0})
	}
	return e
}

func (e *Explorer) VisibleRows() []Row {
	var out []Row
	f := strings.ToLower(strings.TrimSpace(e.Filter))
	match := func(s string) bool {
		if f == "" {
			return true
		}
		return strings.Contains(strings.ToLower(s), f)
	}
	for _, s := range e.Schemas {
		var kept []TableNode
		for _, tb := range s.Tables {
			if f != "" && !match(tb.Name) {
				var cols []ColumnNode
				for _, c := range tb.Columns {
					if match(c.Name) {
						cols = append(cols, c)
					}
				}
				if len(cols) == 0 {
					continue
				}
				cp := tb
				cp.Columns = cols
				kept = append(kept, cp)
				continue
			}
			kept = append(kept, tb)
		}
		if f != "" && len(kept) == 0 && !match(s.Name) {
			continue
		}
		out = append(out, Row{Kind: RowSchema, Schema: s.Name, Depth: 0})
		if !s.Expanded && f == "" {
			continue
		}
		for _, tb := range kept {
			out = append(out, Row{Kind: RowTable, Schema: s.Name, Table: tb.Name, Depth: 1})
			if !tb.Expanded && f == "" {
				continue
			}
			for _, c := range tb.Columns {
				if f != "" && !match(c.Name) && !match(tb.Name) {
					continue
				}
				out = append(out, Row{Kind: RowColumn, Schema: s.Name, Table: tb.Name, Column: c.Name, Depth: 2})
			}
		}
	}
	return out
}

func (e *Explorer) RowAt(i int) (Row, bool) {
	rows := e.VisibleRows()
	if i < 0 || i >= len(rows) {
		return Row{}, false
	}
	return rows[i], true
}

func (e *Explorer) MoveUp() {
	if e.Cursor > 0 {
		e.Cursor--
	}
}

func (e *Explorer) MoveDown() {
	if e.Cursor < len(e.VisibleRows())-1 {
		e.Cursor++
	}
}

func (e *Explorer) Toggle() {
	r, ok := e.RowAt(e.Cursor)
	if !ok {
		return
	}
	switch r.Kind {
	case RowSchema:
		for i := range e.Schemas {
			if e.Schemas[i].Name == r.Schema {
				e.Schemas[i].Expanded = !e.Schemas[i].Expanded
			}
		}
	case RowTable:
		for si := range e.Schemas {
			if e.Schemas[si].Name != r.Schema {
				continue
			}
			for ti := range e.Schemas[si].Tables {
				if e.Schemas[si].Tables[ti].Name == r.Table {
					e.Schemas[si].Tables[ti].Expanded = !e.Schemas[si].Tables[ti].Expanded
				}
			}
		}
	}
	if e.Cursor >= len(e.VisibleRows()) {
		e.Cursor = len(e.VisibleRows()) - 1
	}
	if e.Cursor < 0 {
		e.Cursor = 0
	}
}

func (e *Explorer) SetFilter(f string) {
	e.Filter = f
	e.Cursor = 0
}

func humanizeCount(n int64) string {
	switch {
	case n >= 1000000:
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run 'TestVisibleRowsFlatten|TestToggleCollapse|TestHumanizeCount|TestFilterKeepsParents' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/explorer.go internal/tui/explorer_test.go
git commit -m "feat(tui): explorer tree model with filter and humanized counts"
```

---

### Task 3: Explorer render + styles

**Files:**
- Modify: `internal/tui/styles.go`
- Modify: `internal/tui/explorer.go` (append render section)
- Test: `internal/tui/explorer_test.go` (append render tests)

**Interfaces:**
- Consumes: `Explorer.VisibleRows()`, `humanizeCount`, existing `fitText`.
- Produces: `func (e *Explorer) Render(sidebarW int, height int) string`, styles `explorerTitle`, `explorerSel`, `explorerType`, `explorerCount`, `explorerConn` — consumed by Task 5 `view.go`.

- [ ] **Step 1: Write the failing test**

```go
func TestExplorerRenderGoldSelection(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)
	e := fixtureExplorer()
	out := e.Render(34, 20)
	if !strings.Contains(out, "explorer") {
		t.Fatalf("missing header:\n%s", out)
	}
	if !strings.Contains(out, "48;5;172m") {
		t.Fatalf("sidebar selection must use gold bg 172 (#CA8A04), got:\n%s", out)
	}
	if !strings.Contains(out, "4.0k") || !strings.Contains(out, "integer") {
		t.Fatalf("missing count/type:\n%s", out)
	}
	for _, ln := range strings.Split(out, "\n") {
		if lipgloss.Width(ln) > 34 {
			t.Fatalf("line exceeds width: %q", ln)
		}
	}
}
```

Append `strings`, `lipgloss`, `termenv` imports to `explorer_test.go`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestExplorerRenderGoldSelection -v`
Expected: FAIL with "undefined: Render".

- [ ] **Step 3: Write minimal implementation**

In `styles.go` append:

```go
explorerTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#EAB308"))
explorerSel   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#000000")).Background(lipgloss.Color("#CA8A04"))
explorerType  = lipgloss.NewStyle().Foreground(lipgloss.Color("#67E8F9"))
explorerCount = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
explorerConn  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
```

In `explorer.go` append:

```go
func (e *Explorer) tableByName(schema, table string) (TableNode, bool) {
	for _, s := range e.Schemas {
		if s.Name != schema {
			continue
		}
		for _, tb := range s.Tables {
			if tb.Name == table {
				return tb, true
			}
		}
	}
	return TableNode{}, false
}

func (e *Explorer) columnByName(schema, table, col string) (ColumnNode, bool) {
	tb, ok := e.tableByName(schema, table)
	if !ok {
		return ColumnNode{}, false
	}
	for _, c := range tb.Columns {
		if c.Name == col {
			return c, true
		}
	}
	return ColumnNode{}, false
}

func (e *Explorer) Render(sidebarW, height int) string {
	rows := e.VisibleRows()
	var b strings.Builder
	b.WriteString(explorerTitle.Render("explorer") + "\n")
	conn := "● " + e.ConnName
	b.WriteString(explorerConn.Render(fitText(conn, sidebarW-2)) + "\n")
	var lines []string
	for i, r := range rows {
		var left, right string
		switch r.Kind {
		case RowSchema:
			disc := "▸"
			for _, s := range e.Schemas {
				if s.Name == r.Schema && s.Expanded {
					disc = "▾"
				}
			}
			n := 0
			for _, s := range e.Schemas {
				if s.Name == r.Schema {
					n = len(s.Tables)
				}
			}
			left = disc + " 🗄 " + r.Schema
			right = explorerCount.Render(fmt.Sprintf("%d", n))
		case RowTable:
			tb, _ := e.tableByName(r.Schema, r.Table)
			disc := "▸"
			if tb.Expanded {
				disc = "▾"
			}
			icon := "▦"
			if tb.IsView {
				icon = "👁"
			}
			left = "  " + disc + " " + icon + " " + r.Table
			if tb.CountOK {
				right = explorerCount.Render(humanizeCount(tb.Count))
			} else {
				right = explorerCount.Render("…")
			}
		case RowColumn:
			c, _ := e.columnByName(r.Schema, r.Table, r.Column)
			icon := "◇"
			if c.IsPK {
				icon = "🔑"
			} else if c.IsFK {
				icon = "➤"
			}
			left = "    " + icon + " " + r.Column
			right = explorerType.Render(c.DataType)
		}
		gap := sidebarW - lipgloss.Width(left) - lipgloss.Width(right) - 1
		if gap < 1 {
			gap = 1
		}
		line := left + strings.Repeat(" ", gap) + right
		line = fitText(line, sidebarW)
		if i == e.Cursor {
			line = explorerSel.Render(line)
		}
		lines = append(lines, line)
	}
	if height > 0 && len(lines) > height {
		lines = lines[:height]
	}
	b.WriteString(strings.Join(lines, "\n"))
	return b.String()
}
```

Add `lipgloss` import to `explorer.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestExplorerRenderGoldSelection -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/explorer.go internal/tui/styles.go internal/tui/explorer_test.go
git commit -m "feat(tui): explorer render with gold selection and type colors"
```

---

### Task 4: Wire model + async commands

**Files:**
- Modify: `internal/tui/model.go:29-75,103-145`
- Modify: `internal/tui/msg.go`
- Modify: `internal/tui/commands.go`
- Modify: `internal/tui/update.go:22-80,174-203,299-311`

**Interfaces:**
- Consumes: `Explorer`, `DB.ListSchemas/ListTablesInSchema/ApproxCount/Columns` from Tasks 1–2.
- Produces: `schemasLoadedMsg`, `tableCountMsg`, `columnsLoadedMsg`, `loadSchemasCmd/loadCountsCmd/loadColumnsCmd`, updated `inspectTable` — consumed by Task 5 view/mouse.

- [ ] **Step 1: Write the failing test**

```go
func TestTableCountMsgApplies(t *testing.T) {
	m := browseModel(t)
	m.explorer = fixtureExplorer()
	u, _ := m.Update(tableCountMsg{schema: "public", table: "customers", count: 777})
	m = u.(Model)
	for _, s := range m.explorer.Schemas {
		for _, tb := range s.Tables {
			if tb.Name == "customers" && (!tb.CountOK || tb.Count != 777) {
				t.Fatalf("count not applied: %+v", tb)
			}
		}
	}
}
```

Note: `browseModel` in `sidebar_test.go` must already construct explorer (updated in this task's implementation step); test fails first with "undefined: tableCountMsg".

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestTableCountMsgApplies -v`
Expected: FAIL with "undefined: tableCountMsg".

- [ ] **Step 3: Write minimal implementation**

`msg.go` append:

```go
schemasLoadedMsg struct {
	schemas []string
	tables  map[string][]dbpkg.TableRef
	err     error
}
tableCountMsg struct {
	schema string
	table  string
	count  int64
	err    error
}
columnsLoadedMsg struct {
	schema  string
	table   string
	columns []dbpkg.Column
	err     error
}
```

Add `dbpkg` import to `msg.go`.

`commands.go` append:

```go
func (m Model) loadSchemas() tea.Cmd {
	db := m.db
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		schemas, err := db.ListSchemas(ctx)
		if err != nil {
			return schemasLoadedMsg{err: err}
		}
		tables := map[string][]dbpkg.TableRef{}
		for _, s := range schemas {
			refs, err := db.ListTablesInSchema(ctx, s)
			if err != nil {
				return schemasLoadedMsg{err: err}
			}
			tables[s] = refs
		}
		return schemasLoadedMsg{schemas: schemas, tables: tables}
	}
}

func (m Model) loadCounts() tea.Cmd {
	db := m.db
	type req struct{ schema, table string }
	var reqs []req
	for _, s := range m.explorer.Schemas {
		for _, tb := range s.Tables {
			if !tb.CountOK {
				reqs = append(reqs, req{s.Name, tb.Name})
			}
		}
	}
	return func() tea.Msg {
		return nil
	}
}

func (m Model) loadOneCount(schema, table string) tea.Cmd {
	db := m.db
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		n, err := db.ApproxCount(ctx, schema, table)
		return tableCountMsg{schema: schema, table: table, count: n, err: err}
	}
}

func (m Model) loadColumns(schema, table string) tea.Cmd {
	db := m.db
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cols, err := db.Columns(ctx, table)
		return columnsLoadedMsg{schema: schema, table: table, columns: cols, err: err}
	}
}
```

`model.go`: replace field `list list.Model` with `explorer Explorer`; remove `tablesItemH`; in `New()` drop sidebar `bubbles/list` construction, init `m.explorer = NewExplorer("", []string{})`; remove `tablesItemH` assignment. Keep `conns list.Model` untouched.

`update.go`:
- `connectMsg` success: `m.explorer = NewExplorer(m.db.Display, nil); return m, m.loadSchemas()`.
- Add `schemasLoadedMsg`: build `NewExplorer(m.db.Display, msg.schemas)`, fill tables with `Count:-1`, set status, `return m, m.loadOneCount(first...)` chain + `inspectTable` first table.
- Add `tableCountMsg`: find node, set `Count/CountOK` (on err set `CountOK:false` → renders `…`→`?` via Render fallback), return batch next `loadOneCount` if more pending.
- Add `columnsLoadedMsg`: map `db.Column` → `ColumnNode{IsPK: strings.HasPrefix(c.Extra,"PK"), IsFK: strings.HasSuffix(name,"_id")}`, fill node, keep expanded.
- `sidebarKeys`: `up/k→explorer.MoveUp`, `down/j→MoveDown`, `left/right/enter→Toggle` (+ on table also `loadColumns` + `inspectTable`), `/→filter mode`, `r→loadSchemas`, `c/esc→disconnect`.

Keep every other branch identical.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestTableCountMsgApplies -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/msg.go internal/tui/commands.go internal/tui/update.go
git commit -m "feat(tui): wire explorer async schemas counts columns"
```

---

### Task 5: View + mouse + keys + width

**Files:**
- Modify: `internal/tui/view.go:38-76`
- Modify: `internal/tui/mouse.go:53-60,91-100,131-200,248-263`
- Modify: `internal/tui/tables.go:53-67`
- Modify: `internal/tui/sidebar_test.go` (re-fixture)

**Interfaces:**
- Consumes: `Explorer.Render/RowAt/Toggle/MoveUp/MoveDown`, `explorerFirstRow=4`, `paneX()`.
- Produces: working sidebar split with mouse row-exact hit-testing; no `listIndexAtRaw`/`tablesItemH` for sidebar.

- [ ] **Step 1: Write the failing test**

```go
func TestExplorerClickPreviews(t *testing.T) {
	m := browseModel(t)
	m.explorer = fixtureExplorer()
	m.loading = false
	u, cmd := m.Update(tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 2, Y: 5})
	m = u.(Model)
	if cmd == nil || m.table != "orders" {
		t.Fatalf("want orders previewed, got %q", m.table)
	}
}
```

(`browseModel` re-fixtured to explorer in implementation; fails first because old `list`-based click path no longer previews explorer rows.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestExplorerClickPreviews -v`
Expected: FAIL (wrong table or nil cmd).

- [ ] **Step 3: Write minimal implementation**

`view.go` `browseView`: replace `m.list.View()` with `m.explorer.Render(m.sidebarW, m.contentH())`; add `const explorerFirstRow = 4`.

`tables.go` `resizeBrowse`: `w := 34` default (was 30), clamp `if w < 20 { w = 20 }; if w > 44 { w = 44 }`; drop `m.list.SetSize`, keep `m.sizeTables()`.

`mouse.go`:
- `handleMouse` browse branch: `if msg.X < m.paneX() && msg.Y >= explorerFirstRow { return m.clickExplorer(msg.Y) }`.
- New `func (m Model) clickExplorer(y int) (tea.Model, tea.Cmd)`: `idx := y - explorerFirstRow; r, ok := m.explorer.RowAt(idx)`; set `m.explorer.Cursor = idx`; schema→Toggle; table→Toggle + `loadColumns` + `inspectTable(r.Table)`; column→`inspectTable(r.Table)`; conn `×` (x >= sidebarW-2, y==2)→`disconnect()`.
- `hoverList` sidebar branch → set `m.explorer.Cursor`; `wheel` sidebar branch → `MoveUp/MoveDown`.
- Delete sidebar use of `listIndexAtRaw`/`tablesItemH` (keep conns branch intact).

`sidebar_test.go` `browseModel`: replace `m.list.SetItems(...)` with `m.explorer = fixtureExplorer()` equivalent inline; `m.resizeBrowse()` without list size; keep `m.width, m.height = 100, 30`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestExplorerClickPreviews -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/view.go internal/tui/mouse.go internal/tui/tables.go internal/tui/sidebar_test.go
git commit -m "feat(tui): explorer view mouse keys and wider sidebar"
```

---

### Task 6: Regression sweep + cleanup

**Files:**
- Modify: `internal/tui/sidebar_test.go`, `internal/tui/view_fit_test.go` (if needed)
- Remove: any remaining `m.list` sidebar references, unused `bubbles/list` import for sidebar.

**Interfaces:**
- Consumes: all Tasks 1–5 output.
- Produces: green `go test ./...`, `go vet ./...`, width-fit guarantee.

- [ ] **Step 1: Run full suite to find failures**

Run: `go test ./... -count=1`
Expected: FAIL on old tests referencing `m.list` / `tablesItemH` / `listIndexAtRaw` (documents remaining work).

- [ ] **Step 2: Fix remaining references**

Replace in `sidebar_test.go` / `update_conns.go` / `view_fit_test.go`: `m.list.SelectedItem().(tableItem)` → `m.explorer.RowAt(m.explorer.Cursor)`; `tablesItemH` → delete; `listIndexAtRaw` sidebar tests → `explorerRowAt` equivalents; keep `TestDetailDimFollowsFocus`, `TestBrowseViewFitsTerminal`, `TestBrowseViewSidebarFitsWidth` asserting gold `48;5;172m` (#CA8A04) in sidebar and `lipgloss.Width <= sidebarW`.

- [ ] **Step 3: Run vet + full suite**

Run: `go vet ./... && go test ./... -count=1`
Expected: PASS both.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "test: explorer regression sweep green"
```

---

## Self-Review (ran before save)

1. Spec coverage: layout/counts/types/gold selection/footer → Task 3; schema/count/columns DB → Task 1; lazy async + cache + seq guard → Task 4; keyboard+mouse+filter → Tasks 4–5; errors (`?`, collapse, empty) → Tasks 3–4 Render/Update; files list → all tasks; tests → Tasks 1–3 + 6. No gaps.
2. Placeholder scan: no TBD/TODO/generic handling; every step has exact code, exact `go test`/`git` commands, exact styles (`#CA8A04` → `48;5;172m`).
3. Type consistency: `TableRef`, `Explorer/Row/RowKind/TableNode/SchemaNode/ColumnNode`, `humanizeCount`, `Render(sidebarW,height)`, `NewExplorer`, `schemasLoadedMsg/tableCountMsg/columnsLoadedMsg`, `loadSchemas/loadOneCount/loadColumns` spelled identically across tasks.
