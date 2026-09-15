# Visual ER Diagram (DBeaver-style) Design — db-peek

Date: 2026-09-14
Status: approved
Scope: replace tab 5 text ER (`internal/tui/er.go:27` `erLayout` — center table +
`orders ──▶ customers` lines) with a full-schema DBeaver-style box canvas:
every table is a bordered box with header + column rows, FKs are dashed-blue
orthogonal connectors. Pan + select interaction. Approach A (native TUI
canvas, auto-grid) approved 2026-09-14.

Non-goals: drag-to-move boxes, curved/diagonal lines, zoom levels,
`--erdot`/`--ermmd` export (tracked as follow-up); schema-qualified queries
for non-public schemas (separate issue); detail-pane scrolling changes.

## 1. Problem

Current ER tab shows only the active table centered with one text line per
neighbor (`to ──▶ table cols`, `table ◀── from cols`). Neighbor tables show
no columns, no PK/FK highlighting, no type hints, no multi-hop structure.
The DBeaver reference shows each table as a box (header + full column list,
PK green/bold, `ABC`/`123` type icons, FK markers) with dashed lines joining
FK columns across boxes. Users want that visual density inside the terminal.

Constraints: Bubbletea + Lipgloss only, no new deps. Terminal cells are a
fixed grid — no free-floating boxes, no diagonals, lines must be orthogonal
(`─ │ ┌ ┐ └ ┘ ┄ ┆`). Detail pane width is `paneInnerW()` (often 60–120
cells), so the canvas needs a viewport + pan.

## 2. State

- `db.AllForeignKeys(ctx)` in `internal/db/refs.go`: all FKs in the visible
  scope (loop `ForeignKeys` per table, dedupe; per-engine single query
  allowed as optimization). Signature:
  `func (d *DB) AllForeignKeys(ctx, tables []string) ([]ForeignKey, error)`.
- `Model.erSchema`: `{tables []erTable, links []ForeignKey, loaded bool,
  err string}` where `erTable = {name string, cols []dbpkg.Column,
  pk, fk map[string]bool}`. Reuses `erCache` extended to a full-schema
  entry (`"__schema__"` key) plus existing per-table entries.
- `Model.erPanX, erPanY int`: top-left canvas cell of the viewport.
  `Model.erSel string`: selected box name (default = current `m.table`).
  `Model.hoverER string`: box name under cursor (`""` when none).
  Existing `erOffset` retired for the new tab (keep field for compat, unused).
- `loadERSchema` command in `internal/tui/commands.go`: fetches table list
  (`ListTablesInSchema` for current schema), then `Columns` per table
  (bounded concurrency 4, 15s timeout), then `AllForeignKeys`. Returns
  `erSchemaLoadedMsg`. `setTab(4)` kicks it when `!erSchema.loaded`;
  cached thereafter. Stale-guard via existing `erSeq`.

## 3. Rendering

New file `internal/tui/er_canvas.go`; `er.go` keeps `erHit`/`erView`
signatures but delegates to canvas.

- Box: width `min(28, max(header, rows))`, height `1 + min(N, cap)` where
  cap = all columns when `len(tables) <= 40` else PK/FK columns (table column
  order) + first 8 non-key columns + `… M more` row. Header `▦ name` in blue
  `#005FD7` rounded border; selected box gold
  `#CA8A04` border (matches explorer selection). Rows: `🔑 pk` bold/green
  tint, `➤ fk`, `◇ plain` + right-aligned dim type hint (`ABC` text,
  `123` int/numeric, `◷` time/date, else raw short type). Reuse
  `fitText`/`ansi.Truncate` so lines never wrap (mouse-row contract).
- Layout: deterministic grid. Sort tables by name; grid columns =
  `max(1, round(sqrt(n) * max(1.0, float64(innerW) / float64(max(1, innerH)) / 2.0)))`
  computed once per schema load with the load-time pane size, place
  left→right/top→bottom with gaps `gx=4, gy=2`. Box origin recorded in
  `map[string]rect`. Stable across runs, no force-directed pass.
- Connectors: per FK, orthogonal L-path from right edge midpoint of
  from-box to left edge midpoint of to-box (same-row: direct `┄`; else
  exit right → vertical `┆` at mid-gap → enter left). Self-ref loops on
  right edge. Drawn first (under boxes) in dim-blue (`lipgloss.Color("12")`
  dimmed); FK rows that overflow the box cap still connect box-to-box.
- Viewport: `erView(innerW, innerH)` slices canvas at `(erPanX, erPanY)`,
  pads to `innerH`, joins with `\n`. Empty DB → `(no tables)`; no FKs →
  boxes only + dim `(no foreign keys — boxes only)` hint line.

## 4. Input

- Pan: arrows / WASD / hjkl move `erPanX/Y` by 2/1 cells; `PgUp/PgDn`
  vertical jump; wheel = vertical pan (shift+wheel = horizontal where
  reported). Clamped to `[0, canvasW-innerW] × [0, canvasH-innerH]`.
- Select: `Tab`/`S-Tab` cycles boxes in layout order; click inside a box
  selects it; `Enter` calls existing `inspectTable(erSel)` (preview +
  detail reload). Hover highlights box border (reuse `hoverTab` pattern
  with `hoverER string`).
- `erHit(x, y)` rewritten: viewport→canvas coords, topmost box containing
  the cell wins; connector cells never hit (boxes drawn over them).
- `r` reloads schema (clears `erSchema.loaded`, re-runs `loadERSchema`).

## 5. Data flow / errors

`setTab(4)` → cached render or `loading...` + `loadERSchema`. Partial
failure (one table's `Columns` errors) keeps other boxes, records
`erSchema.err` shown via `errStyle` footer line; total failure falls back
to legacy single-table `erLayout` with error. Count badges unchanged.
Performance: columns cached per session; reload explicit only.

## 6. Tests

- `TestERCanvasBoxes`: 3-table fixture → output contains all names,
  `🔑`/`➤`, box border chars (`╭`/`─`), type hints.
- `TestERCanvasConnectors`: from-box right edge and to-box left edge
  joined by `┄`/`┆` in canvas (not clipped in full-canvas render).
- `TestERPanClamp`: pan beyond edges clamps; viewport size respected.
- `TestERHitBox`: click inside box returns its name; click on gap returns
  miss; center-box exclusion removed (all boxes hittable).
- `TestAllForeignKeysDedupe`: sqlite `test.db` fixture, no duplicate pairs.
- Full `go test ./...` green; legacy `TestERViewThreeBoxes` updated to new
  box markers.

## 7. Risks

- Dense schemas (100+ tables): canvas large (e.g. 10×10 grid × 30×12
  cells ≈ 300×140). Pan-only navigation is slow — mitigated by `Tab`
  cycling + filter follow-up (not in scope).
- Connector crossings on dense graphs: accepted; Manhattan routing may
  cross boxes. Mitigated by drawing under boxes + gap spacing.
- Emoji width variance (`🔑`/`➤`/`▦`): all width math via
  `lipgloss.Width`, consistent with explorer/style-last contract.
- Load cost (N `Columns` queries): bounded concurrency + cache; worst
  case shows progressive `loading...` rather than blocking.
