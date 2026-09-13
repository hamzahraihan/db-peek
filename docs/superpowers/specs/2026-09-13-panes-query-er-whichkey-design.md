# Panes, Query Lab, ER Diagram, Which-Key Design — db-peek

Date: 2026-09-13
Status: approved
Scope: one phased plan, four phases (P1 borders+zebra, P2 query+editor, P3 ER+refs, P4 which-key).

## 1. Pane borders + mouse focus

Both sidebar and detail panes get rounded lipgloss border boxes with titles
(sidebar: connection display name, detail: current table name).
Focused pane border is gold `#EAB308` (same as explorer header); unfocused
is dim `#3A3A3A`. The existing dimmed-pane palette is kept, so focus reads
two ways (border + content). App header and footer rows stay outside boxes.

Layout math: a border costs 2 columns + 2 rows per pane. `sidebarW` grows
to absorb the horizontal cost; `contentH` shrinks by 2 for the vertical
cost. All mouse hit-testing becomes border-relative: clicks landing on a
border cell are noop. `explorerFirstRow` and the detail table top shift by
the border offset (+1 row for the top border, +1 col for the left border).

Focus: `tab` still toggles. Additionally, a mouse click anywhere inside a
pane moves focus there AND performs the pane's normal click action
(sidebar: focus + select/toggle row; detail: focus + grid/tab action).
Hover never changes focus.

## 2. Query tab (4th detail tab, key `4`)

Tabs become `1 schema, 2 indexes, 3 rows, 4 query, 5 er`. The query tab
replaces the detail grid area with two stacked sub-panes:

- Top: custom multiline SQL editor owning its rune buffer, line/col
  cursor, scroll offset, and edit mode. Every keystroke re-lexes with the
  chroma SQL lexer and renders the token stream with a dark style
  (keywords gold, strings green, numbers cyan, comments gray). Cursor is a
  reverse-video block. Mouse click positions the cursor
  (border-relative → line/col). v1 has insert/delete/newline only, no
  text selection, no undo.
- Bottom: results in the existing `dataTable` (with zebra striping) plus
  a status line `N rows • M ms`.

`ctrl+enter` or `F5` runs the query through the existing `db.Query`
(200-row cap, 15-second timeout, `detailSeq`-style seq guard against stale
replies). Errors render in an error line and never clear the editor text.
`esc`/arrow keys move focus between editor and results sub-panes.

New dependency: `alecthomas/chroma` (lexer + dark style). No other new deps.

## 3. ER tab (5th detail tab, key `5`)

New `internal/db/refs.go` exposing
`ForeignKey{FromTable, FromColumn, ToTable, ToColumn}` per engine:

- Postgres: `pg_constraint` (`contype='f'`) joined to `pg_attribute` /
  `pg_class` for both column sides.
- MySQL: `information_schema.KEY_COLUMN_USAGE` with non-null
  `REFERENCED_TABLE_NAME`.
- SQLite: `PRAGMA foreign_key_list(table)` per table.

The tab lazily loads the 1-hop graph around the selected table and caches
it per session (same pattern as table counts). Layout: center box is the
selected table with its columns (`🔑` PK / `➤` FK markers reused from the
explorer); left column holds tables it references (outgoing, `──▶` arrows
into center); right column holds tables referencing it (incoming, `◀──`
from center). Boxes size to content and scroll with up/down when taller
than the pane. Clicking a neighbor box jumps the detail view to that
table. Tables with no FKs show a dim `(no foreign keys)` line.

## 4. Which-key overlay + zebra + key changes

`?` opens a centered rounded overlay listing bindings grouped by context
(Sidebar / Detail+tabs / Query editor / ER / Global). Content is generated
from a single `keyRegistry` (key → description + contexts) that every key
handler registers with, so help text can never drift from handlers. `?` or
`esc` closes; mouse clicks outside the overlay dismiss it. `?` works
everywhere except while typing in the conns filter input.

New/changed keys:

| Key | Where | Action |
|-----|-------|--------|
| `4` / `5` | detail | query / er tab |
| `ctrl+enter`, `F5` | query editor | run SQL |
| `?` | anywhere | keybinding overlay |
| click | any pane | focus + select |

Zebra striping: `dataTable` alternates base vs slightly-black even rows
(`#0D1117`) applied UNDER selection/hover styles (selection still wins),
in both normal and dimmed palettes. Applies to every grid (schema,
indexes, rows, query results).

## Phasing

- P1: borders + mouse focus + zebra (UI polish, touches shared layout math).
- P2: query tab + chroma editor (biggest item, ~half the work).
- P3: ER tab + refs (new db + tui units).
- P4: which-key overlay (needs final key inventory incl. P2/P3 keys).

## Non-goals

Editor text selection and undo; multi-hop ER graphs; DDL execution
(`db.Query` is SELECT-oriented, unchanged); filter-input UI for explorer;
changing `toggle-on-click-collapse` sidebar behavior.

## Open detail (resolved during planning)

`Render(height)` keeps data-lines-only semantics; the view layer owns
chrome lines (separator, footer). `loadColumns`/`ApproxCount` resolve table
names within the connection's default schema/search_path (documented, no
db behavior change).
