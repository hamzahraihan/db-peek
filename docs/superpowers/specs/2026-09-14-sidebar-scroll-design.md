# Sidebar Scroll + Fragment Fix Design — db-peek

Date: 2026-09-14
Status: approved
Scope: fix stray `|` fragments on expanded sidebar column rows; add a
scroll viewport with scrollbar to the sidebar explorer so long schema
lists (e.g. 23-table `auth`) stay navigable. Approach A (viewport +
overlay scrollbar + style-last rendering), approved 2026-09-14.

Non-goals: schema-qualified queries for non-public schemas (the
`relation "mfa_factors" does not exist` detail error and `?` count
badges share that root cause and are tracked separately); detail-pane
scrolling (already works); filter-input behavior (unchanged).

## 1. Problem

Two defects, one area:

1. Fragments: sidebar lines are assembled as plain text + already-styled
   type/count segments, then capped by `fitText`, which walks runes —
   including ANSI escape bytes. On long rows the cut lands inside an
   escape sequence, emitting broken-ANSI fragments (stray `|`s) and
   ragged border cells.
2. No scroll: `Explorer.Render` builds all rows then hard-truncates
   (`lines[:height]`). `Cursor` roams the full `VisibleRows` list, so it
   slides below the fold and disappears; wheel moves the cursor but the
   view never follows. No positional feedback.

## 2. State

- `Explorer` gains `Offset int`: index of the first visible tree row.
  Default 0 — all existing behavior and tests are unchanged when there
  is no overflow.
- Viewport height is what `Render` already receives (`innerH - 4`:
  title, conn, separator, footer). A single Model helper,
  `sidebarTreeH()`, computes it so render, clamp, and hit-testing share
  one truth.
- `SetFilter` resets `Offset` to 0 alongside `Cursor`.
- New `ensureVisible(viewH)` runs after every cursor mutation
  (sidebar keys, filter keys, wheel, toggle) to keep the cursor in
  `[Offset, Offset+viewH)`.
- `Render` stays pure (package rule: View never mutates state). It
  clamps via a local variable as a safety net for stale offsets.

## 3. Rendering

Sidebar path only; no width, border, or row-position changes.

- Style-last lines: build each row from plain strings, truncate the
  name (with `…`) so `left + gap + right` fits, then apply lipgloss
  styles. Styled output is never sliced again, which removes the
  fragment class. `ansi.Truncate` remains as an ANSI-aware safety net.
- Overlay scrollbar: drawn only on overflow (`len(rows) > viewH`),
  only over tree-row lines, only on the last content cell. Dim `│`
  track, bright-white (lipgloss color 15) `█` thumb — readable on both
  plain rows and the gold selection background — standard proportional
  geometry:
  `thumbH = max(1, viewH*viewH/len)`,
  `thumbStart = Offset*(viewH-thumbH)/max(1, len-viewH)`.
  That cell is box padding on virtually every row; on exactly-full
  rows it covers the final `…` — accepted and documented here.
- First visible tree row stays at terminal row `explorerFirstRow = 5`;
  footer/chrome layout is untouched.

## 4. Input

- Cursor-follow scrolling: wheel and up/down (keys) move the cursor
  exactly as today, then `ensureVisible` pulls the viewport along. The
  cursor can never hide below the fold.
- Mouse mapping becomes `idx = (y - explorerFirstRow) + Offset` in
  `clickExplorer` and sidebar hover; misses still noop via `RowAt`.
- Border/chrome/focus semantics from the earlier pane-focus fixes are
  untouched (border clicks switch focus without selecting).

## 5. Tests

- Viewport follows cursor: 30-table fixture, small height, `MoveDown`
  to the end — `Render` shows the tail rows and `Offset > 0`.
- Scrollbar appears on overflow, absent otherwise.
- Click at row `y` with nonzero offset selects
  `rows[offset + y - explorerFirstRow]`.
- Narrow-width render: every line `lipgloss.Width <= innerW`, no broken
  escapes (strip ANSI and compare cell counts).
- Full existing suite stays green; no existing test needs modification
  (offset defaults to 0).

## 6. Risks

- Thumb-over-`…` on exactly-full rows: cosmetic, accepted (see §3).
- Multi-line footers (several empty-schema hints) can steal one tree
  slot under the `innerH` cap, as today; unchanged by this design.
- Emoji width variance across terminals (🗄/🔑/➤): gap math already
  uses `lipgloss.Width`; style-last does not change that contract.
