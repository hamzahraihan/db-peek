# Query Result View After Writes Design — db-peek

Date: 2026-09-16
Status: approved
Scope: after a successful write in the TUI query tab, show a result view
instead of only `N rows affected`. Approach A (TUI-chained preview +
RETURNING fix), approved 2026-09-16.

Non-goals: CLI `--query` output change; preview pagination or configurable
size; auto-switching to the rows tab; full sidebar refresh on every write;
multi-statement preview (only the first statement's table is previewed).

## 1. Problem

The query tab (`tab == 3`) routes row-returning statements through `Query`
(grid) and everything else through `ExecStmt` (affected-count line).
After `INSERT INTO foo ...` the user sees only `1 row affected` — no
confirmation of what landed in `foo`. Two gaps:

1. `INSERT ... RETURNING *` (and `UPDATE/DELETE ... RETURNING`) starts with
   a write keyword, so `isWriteStatement` routes it through `Exec` and the
   returned rows are discarded.
2. Plain writes (`INSERT/UPDATE/DELETE` without `RETURNING`) never fetch
   anything, so there is no grid to look at.

## 2. Architecture

Stays inside the existing query tab. No new screen or pane. `runQuery`
remains the single entry point; it performs up to two DB calls inside one
`tea.Cmd` and returns one enriched `queryDoneMsg`. The existing grid
(`queryTable`) and footer in `view_detail.go` are reused: the post-write
result view is the same grid SELECTs use, with a different footer.

Components (files touched):

- `internal/db/write.go` — classifier fix + new parser: `hasReturningClause`,
  `parseWriteTable`. `hasReturningClause` is a case-insensitive
  word-boundary match for `RETURNING` after stripping leading comments
  (so a column named `returning` without word boundaries still counts as a
  keyword match only when it appears as a standalone word; string-literal
  false positives are accepted as a documented limitation and simply route
  through the Query path, which surfaces the driver error). Handles `INSERT INTO foo`, `UPDATE foo`,
  `DELETE FROM foo`, quoted `"foo"` / `` `foo` `` / `[foo]`, `schema.foo`
  yielding `foo`, leading `--` / `/* */` comments. Returns `""` when the
  target is unclear (SELECT/WITH, DDL without a single table, CTEs,
  multi-statement).
- `internal/tui/msg.go` — `queryDoneMsg` gains `previewTable string`
  (empty means no preview).
- `internal/tui/commands.go` — `runQuery` chaining: Query path when
  RETURNING is present, else Exec plus best-effort `PageRows(table, 20, 0)`.
- `internal/tui/update.go`, `internal/tui/view_detail.go` — store the
  preview in `querySample` / `queryTable`; footer renders
  `N row(s) affected • preview of <table> • M ms`.
- Side-effect refresh: same-table `loadOneCount` after DML preview; full
  `loadSchemas` after DDL (`CREATE` / `DROP` / `ALTER`) so the sidebar never
  goes stale.

## 3. Data flow

1. User presses `ctrl+r` (or `f5`): `querySeq++`, `loading = true`,
   `runQuery()` captures `sql`.
2. Inside the command (15 s timeout):
   - `hasReturningClause(sql)` is true: route via `Query()`; return
     `(sample, -1)` with `previewTable == ""`. The footer renders the
     existing `N rows • M ms`; the RETURNING rows are the result view.
   - Otherwise `ExecStmt()` yields `n` affected. When `n` succeeds and
     `parseWriteTable` yields `tbl != ""`, run `PageRows(tbl, 20, 0)` and
     return `queryDoneMsg{sample: preview, affected: n, previewTable: tbl}`.
     When parsing yields `""`, return the affected-only message (current
     behavior). DDL naming a table chains `loadSchemas` instead of a grid
     preview.
3. `update.go` on `queryDoneMsg`: the existing `connSeq` / `querySeq` guards
   apply unchanged — the preview shares the originating seq, so a re-run or
   connection switch still drops the late message. On success:
   `querySample = preview` (possibly nil), `queryAffected = n`, rebuild
   `queryTable` from the preview when present, then `sizeTables()`. Focus is
   never stolen (editor stays editor, results stay results).
4. Render: write plus preview shows a grid of up to 20 heap-scan rows plus
   the footer `1 row affected • preview of foo • 12 ms`. Write without a
   preview shows the current `1 row affected • 12 ms`. SELECT and RETURNING
   show `N rows • M ms`.

Preview is a fixed `LIMIT 20` heap scan (no `ORDER BY`), matching the cheap
preview semantics of the rows tab.

## 4. Error handling

- Exec failure: unchanged — editor text and the old grid are kept, `m.err`
  shows the driver error, no preview is attempted.
- Preview SELECT failure (table dropped, permission denied): not an error
  splash. Keep the affected-count success, set `querySample = nil` and
  `previewTable = ""`, and fall back to the `N rows affected • M ms` line.
  The failure is swallowed because the affected line is the source of truth.
- RETURNING-statement failure: same path as SELECT failure today.
- Comment-only / empty input: stays on the query path; driver errors surface
  as today.
- Multi-statement paste (`INSERT ...; SELECT ...`): out of scope. Only the
  first statement's table is previewed; drivers that reject multi-statements
  surface their error unchanged.
- Staleness: both DB calls share one command and seq, so no new race is
  introduced.

## 5. Testing

- `internal/db/write_test.go`: `hasReturningClause` (upper/lowercase,
  leading comments, no false positive on a column literally named
  `returning`); `parseWriteTable` (plain, quoted, schema-qualified,
  `IF NOT EXISTS`, leading comments, `""` for SELECT / WITH / unclear DDL);
  `isWriteStatement` returns false for any statement carrying RETURNING.
- `internal/tui`: `queryDoneMsg` carrying `previewTable` builds the grid and
  the `affected • preview of` footer; a stale `querySeq` drops the preview;
  a nil preview falls back to the affected-only line.
- Manual: against `test.db`, run `INSERT INTO <table> ...` and confirm the
  grid plus footer appear; run `INSERT ... RETURNING *` and confirm returned
  rows render; run `CREATE TABLE` and confirm the sidebar refreshes.
