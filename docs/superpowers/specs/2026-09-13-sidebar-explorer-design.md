# Sidebar Explorer Design — db-peek

Date: 2026-09-13
Status: approved
Scope: replace flat `bubbles/list` sidebar with hierarchical explorer matching reference image.

## 1. Visual / Layout

Reference image structure:

```
explorer
● shop                                          ×
▾ 🗄 public                                      4
  ▸ ▦ customers                               500
  ▸ ▦ order_items                            12.0k
  ▸ 👁 order_totals
  ▾ ▦ orders                                  4.0k
    🔑 id                                  integer
    ➤ customer_id                          integer
    ◇ status                                  text
    ◇ total                                numeric
    ◇ placed_at                        timestamptz
  ▸ ☰ sequences                                 3
▸ 👤 roles                                      1
1 schema
```

Spec:

- Container: rounded border, gold-dim border (`#8A6D1B`), dark background `#0A0F14`.
- Header row 0: `explorer` lowercase, gold `#EAB308`, bold.
- Row 1: connection: green dot `●` (`#22C55E`) + conn display name (`shop` = `DB.Display`) + right-aligned `×` (disconnect affordance, acts as `c`/`esc`).
- Separator line after conn row.
- Schema rows: `▾/▸` disclosure + `🗄` icon + name + right-aligned table count.
  - Postgres: real schemas from `pg_tables` (`public`, …). v1 shows tables+views only (no sequences/roles groups).
  - MySQL/SQLite: single synthetic schema node (`DB.Display` or `main`), expanded by default.
- Table rows (indent 2): `▸/▾` + `▦` table / `👁` view icon + name + right-aligned humanized count (`500`, `12.0k`, `4.0k`, `…` while loading, `?` on error).
- Column rows (indent 4, visible only when parent table expanded): guide prefix `│ ` (or spaces for last child), icon by role:
  - PK first column → `🔑` (gold), FK (`*_id`) → `➤`/`↗` (blue), rest `◇`/`⬢` (dim).
  - Name left, native type right-aligned cyan-dim (`#22D3EE` muted, e.g. `integer`, `text`, `numeric`, `timestamptz`).
- Selection: full-width gold background `#CA8A04`, black text (replaces current purple `selTitle` in sidebar only; detail pane keeps purple).
- Footer: `1 schema` / `N schemas` gold, bottom-anchored.
- Counts: `humanizeCount(n)`: `<1000` exact, `>=1000` with `k` one decimal (`12.0k`), `>=1M` with `M`.
- Sidebar width: grow default 30 → 34 cells (accommodate type column); clamp 20–44; narrow terminals (`<72`) keep half-width rule.

## 2. Data Model & DB

New file `internal/tui/explorer.go`:

```go
type ColumnNode struct { Name, DataType string; IsPK, IsFK bool }
type TableNode struct {
  Schema, Name string; IsView bool; Expanded bool
  Count int64 // -1 unknown
  CountState int // loading/done/err
  Columns []ColumnNode
}
type SchemaNode struct { Name string; Expanded bool; Tables []TableNode }
type Explorer struct {
  Schemas []SchemaNode
  Cursor int // index into flattened visible rows
  Filter string; Filtering bool
  ConnName string
}
func (e *Explorer) VisibleRows() []Row // Row{Kind, Schema, Table, Column, Depth}
func (e *Explorer) Toggle(), MoveUp/Down(), ExpandAll/CollapseAll()
func humanizeCount(n int64) string
```

New file `internal/db/schema.go`:

- `ListSchemas(ctx) []string`:
  - Postgres: `SELECT schema_name FROM information_schema.schemata WHERE schema_name NOT IN ('pg_catalog','information_schema') ORDER BY 1`
  - MySQL: return `[DATABASE()]`
  - SQLite: return `["main"]`
- `ListTablesInSchema(ctx, schema) [{Name, IsView}]`:
  - Postgres: `pg_tables WHERE schemaname=$1` UNION views from `pg_views`
  - MySQL: `information_schema.tables WHERE table_schema=DATABASE()`
  - SQLite: `sqlite_master WHERE type IN ('table','view')`
- `ApproxCount(ctx, schema, table)`:
  - Postgres: `SELECT reltuples FROM pg_class WHERE relname=$1` fallback `COUNT(*)`
  - Others: `SELECT COUNT(*)`
- `Columns()` reused verbatim for expanded-table columns (PK from `Extra PK(n)`, FK heuristic `*_id` suffix or index metadata).

Loading flow:

1. `connectMsg` → build schema nodes (expanded: first schema + conn), fire `loadSchemasCmd` then batched `loadCountsCmd` (8 concurrent, 10s timeout each).
2. Each count returns `tableCountMsg{schema, table, count, err}` → updates `TableNode`, no full rebuild.
3. Table expand (`enter`/`right`/click) fires `loadColumnsCmd{table}` reusing `db.Columns()`; result `columnsLoadedMsg` fills `ColumnNode`s; `inspectTable()` still fires for detail pane (seq-guarded `detailSeq` unchanged).
4. Cache: counts/columns persist per session; `r` invalidates and refetches.

## 3. Interaction

Replaces `m.list list.Model` for sidebar (conns picker keeps `bubbles/list`).

Keyboard (sidebar focused, `focusDetail==false`):

| Key | Action |
|-----|--------|
| `up/k`, `down/j` | move explorer cursor across visible rows (schemas, tables, columns) |
| `left` / `right` / `enter` | toggle expand on schema/table; on column row `enter` = preview table in detail (same as table `enter`) |
| `/` | focus regex filter input (same `(?i)` regex as today); filter narrows tables/columns, keeps parent schemas visible |
| `enter` on table | expand + `inspectTable()` (detail preview) — matches image's expanded `orders` |
| `r` | reload schemas + tables + counts |
| `c`, `esc` | disconnect to conns screen |
| `tab` | toggle sidebar/detail focus |
| `q` | quit (unless filtering) |

Mouse (cell-motion already enabled):

- Click `▸/▾` or anywhere on schema/table row → toggle + select; single click on table also previews detail immediately (same as today's `clickList` preview).
- Click column row → select parent table detail (no extra expand).
- Click `×` on conn row → disconnect.
- Hover moves explorer cursor without preview (deduplicated like `hoverList`).
- Wheel over sidebar scrolls explorer; over detail scrolls grid (existing `paneX()` split kept).
- Hit-testing: `row = y - explorerFirstRow` (header 1 + conn 1 + sep 1 = first tree row at y=4); each visible row is exactly 1 terminal row (no variable `itemH`), deleting `listIndexAtRaw`/`tablesItemH` complexity.

Filter: `/` input row replaces tree top while `Filtering`; `esc` clears; footer shows `n/m tables` when filtered.

## 4. Errors, Files & Tests

Errors (non-blocking):

- Count query fails → cell shows `?`, tooltip in status; tree stays.
- `Columns()` on expand fails → `m.err` set, row auto-collapses, detail pane untouched.
- Empty schema → `(empty)` dim child row.
- Zero tables overall → sidebar shows `(no tables)` + hint `r refresh • c conns`.

Files:

- NEW `internal/tui/explorer.go` — model, flatten, nav, render (`explorerView()`), filter, humanize.
- NEW `internal/db/schema.go` — `ListSchemas`, `ListTablesInSchema`, `ApproxCount`.
- EDIT `internal/tui/model.go` — replace `list list.Model` sidebar with `explorer Explorer`; drop `tablesItemH`; keep `conns list.Model`.
- EDIT `internal/tui/update.go` — `connectMsg`/`tablesLoadedMsg` build explorer; new `tableCountMsg`/`columnsLoadedMsg`; `sidebarKeys` → explorer nav.
- EDIT `internal/tui/mouse.go` — replace `clickList`/`hoverList`/`listIndexAtRaw` sidebar branch with `explorerRowAt(y)`; keep conns branch.
- EDIT `internal/tui/view.go` — `browseView` left pane = `m.explorerView()` instead of `m.list.View()`; `explorerFirstRow=4`.
- EDIT `internal/tui/styles.go` — add `explorerTitle` (gold), `explorerConn` (green dot), `explorerSel` (gold bg), `explorerType` (cyan-dim), `explorerCount` (dim right).
- EDIT `internal/tui/tables.go` — `resizeBrowse` sidebar default 34.

Tests (update + new):

- `explorer_test.go`: nav up/down across schemas/tables/columns, toggle expand/collapse, `VisibleRows` flatten order, `humanizeCount` table (999→`999`, 12000→`12.0k`), filter keeps parents, count-msg applies to right node, stale seq dropped.
- Update `sidebar_test.go`: replace `list.SetItems` helpers with explorer fixtures; keep click-previews-detail, pane-offset, dim-follows-focus, width-fit tests (assert gold `48;5;178m` selection in sidebar, not purple).
- `internal/db/schema_test.go` (sqlite `:memory:`): schemas, tables-in-schema, counts.

Out of scope: sequences/roles nodes, view-DDL, cross-schema FK icons beyond `*_id` heuristic, connection-picker redesign.
