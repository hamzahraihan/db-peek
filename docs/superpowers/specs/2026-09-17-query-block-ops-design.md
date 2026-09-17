# Query Block Ops Design — db-peek

Date: 2026-09-17
Status: approved
Scope: line-wise block select + delete line/block, copy selection to
system clipboard, and `--` comment toggle in the TUI query editor
(`tab == 3`, `queryFocus == 0`). Approach A (line-wise visual selection
with standard keys), approved 2026-09-17.

Non-goals: char-precise (intra-line) selection; in-editor yank/paste
buffer; `/* */` block wrap; vim modal editing; changes to the results
grid, other tabs, or CLI `--query`; configurable keybindings.

## 1. Problem

The query editor (`internal/tui/queryedit.go`) is single-cursor only:
char insert, char backspace/delete, newline, arrows, home/end. There is
no way to mark a block of lines, so deleting several lines means
repeating backspace, copying a fragment means `Ctrl+S` full-dump
(`internal/tui/copyquery.go`) plus manual trimming elsewhere, and
commenting a query out means typing `-- ` per line by hand.

## 2. Architecture

Stays inside the query tab. No new screen or pane. `Editor` gains
selection state; `queryKeys` in `internal/tui/update.go` maps new keys
to small pure helpers; rendering reuses the 8-row `queryEditorView` in
`internal/tui/view_detail.go` with the existing selection style; copy
reuses the `clipboardWriteAll` seam and file fallback.

Components (files touched):

- `internal/tui/queryedit.go` — `Editor` gains `selAnchor int` +
  `selActive bool`; helpers `selectedRange() (lo, hi int)`,
  `clearSelection()`, `extendSelectionTo(line)`, `deleteRange()`,
  `selectionText()`, `toggleCommentRange()` / `toggleCommentLine()`.
- `internal/tui/update.go` — `queryKeys` editor branch: `Shift+Up/Down`,
  `Shift+Click`/drag extend; `Ctrl+D` delete; `Ctrl+S` copies selection
  when active else full dump; `Ctrl+/` toggles comment; `Esc` clears
  selection first. Every op ends with `clampEditorScroll()` +
  `refreshCompletion()`.
- `internal/tui/mouse.go` — `clickQuery` editor rows: shift-press/drag
  extends the block instead of only moving the cursor.
- `internal/tui/view_detail.go` — selected lines render with the
  existing selection role; editor stays 8 rows so `queryResultsTop`
  hit math is unchanged; hint line mentions the new keys and the
  status line shows `SELECT n lines` while active.
- `internal/tui/copyquery.go` — no format change; selection copy sends
  the raw block text (no dump header), full dump keeps
  `queryDumpText` + `m.err`.

## 3. Selection model

Line-wise only. Anchor set on first `Shift` extend; cursor line is the
other end; `selectedRange` normalizes regardless of direction. Plain
move/type (no `Shift`) clears the block. `Esc` clears selection first;
second `Esc` follows today's editor → results path. Active only when
`focusDetail && tab == 3 && queryFocus == 0`.

## 4. Ops and keys

- `Shift+Up / Shift+Down`, `Shift+Click` / drag: extend block.
- `Ctrl+D`: delete selection when active, else current line. Never
  leaves `Lines` empty: last-line delete resets to `[""]`; cursor and
  `OffY` re-clamped. (`Backspace`/`Delete` keep char behavior.)
- `Ctrl+S`: selection text to clipboard when active; otherwise today's
  full dump. (`Ctrl+C` stays global quit and is not reused.)
- `Ctrl+/`: toggle `-- ` per line in selection, else current line.
  Idempotent: fully-commented range uncomments (strips one `-- ` or
  `--`); otherwise comments. Blank lines get `-- ` so re-toggle is
  symmetric. Cursor line/col preserved by rune count.

## 5. Data flow

Key/mouse → normalize range via `selectedRange()` → pure
delete/comment/text helper on `Editor.Lines` → cursor fixup →
`clampEditorScroll()` + `refreshCompletion()` → optional clipboard
write reporting through `m.status`. `runQuery`/`querySeq` path
untouched; selection never affects executed SQL except through the
edited text itself.

## 6. Error handling

Empty/single-line edge cases are safe noops or reset-to-`[""]`; no
panics; `Text()`/`SetText` invariants hold. Clipboard uses today's
never-panic path (strip NUL, recover → `db-peek-debug.txt` fallback).
Comment prefix is always column 0 (no indent sniffing), so toggle is
predictable across SQLite/Postgres/MySQL.

## 7. Testing

- `Editor` unit: delete block, delete current line, delete last line
  resets, toggle on/off idempotent, mixed block comments, anchor
  above/below cursor, `selectionText` joins with `\n`.
- TUI keys: `Shift+Up` extends, plain move clears, `Ctrl+D` deletes
  line, `Ctrl+S` copies selection-only via stubbed `clipboardWriteAll`,
  `Ctrl+/` toggles, `Esc` clears first.
- Regression: `go test ./...` green; 8-row editor budget and
  `queryResultsTop` mouse tests unaffected.
