# Schema-Qualified Tables Design — db-peek

Date: 2026-10-02
Status: approved (Approach A: QualTable end-to-end)
Scope: pg duplicate table names across schemas; qualified `schema.table` CLI+TUI; ambiguous-bare error.
Repo: github.com/hamzahraihan/db-peek
Non-goals: `db.schema.table` 3-part names, cross-schema JOIN UX, search_path editing, MySQL/SQLite schema semantics change.

## 1. Intent

`db-peek` returns wrong metadata when two pg schemas hold the same table name:
`Columns`/`Indexes`/`Count`/`PageRows` match on bare `table_name`/`relname`
(`internal/db/db.go:222,370,524,541`, `internal/db/schema.go:139`,
`internal/db/refs.go:18`) while the explorer already threads `(schema, table)`
(`internal/tui/commands.go:78`, `internal/tui/update.go:856`,
`internal/tui/explorer.go:249`). Success = `analytics.users` vs
`public.users` addressable everywhere; bare `users` with 2+ matches errors
with candidates; single-match bare names keep working; mysql/sqlite behavior
unchanged; `go vet` + `go test ./...` green.

Assumptions: pg only meaningful schema work; mysql scope is `DATABASE()`,
sqlite scope is `main`; connection selects the database; TUI explorer is the
schema source-of-truth in-app.

## 2. qual.go (new: `internal/db/qual.go`)

```go
type QualTable struct{ Schema, Name string }
func ParseQualTable(s string) (QualTable, error)
func (q QualTable) String() string      // "s.t" or "t"
func (d Driver) QuoteQual(q QualTable) string // `"s"."t"` (pg/sqlite), backticks (mysql); empty schema → QuoteIdent(name)
```

- Split on LAST `.`; strip whitespace + surrounding `"`, backtick, `[]` per part.
- Empty side (`a.`, `.b`, ``) → error `bad table name %q: want [schema.]table`.
- 2+ dots → error (3-part `db.schema.table` out of scope).
- `String()` used in status lines and ambiguity candidates.

## 3. DB layer changes

Signatures (table param → `QualTable`):
`Columns, Indexes, Count, PageRows, SampleRows, PrimaryKey, UpdateCell, ForeignKeys`.
`AllForeignKeys`/`sqliteAllForeignKeys` take `[]QualTable`; callers resolve every
bare name via `ResolveTable` before calling (pg path never receives Schema == "").

pg query deltas (mysql/sqlite ignore Schema except quoting):
- `Columns`: `WHERE table_name=$1 AND table_schema=$2` (schema empty → `AND ($2='' OR table_schema=current_schema())`? No — empty schema keeps today's exact predicate to avoid behavior change; resolution happens before).
- `Indexes` (pg_indexes): `WHERE tablename=$1 AND schemaname=$2`.
- `Count`/`PageRows`/`SampleRows`: `FROM "s"."t"` via `QuoteQual`.
- `PrimaryKey`: `i.indrelid = ($1::text::regclass)` stays, but caller passes
  schema-qualified string (`q.String()` cast) so `a.users` resolves; add
  `AND c.relnamespace::regnamespace::text=$2` guard via join to `pg_class c`.
- `ApproxCount`: `SELECT reltuples FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.relname=$1 AND n.nspname=$2`, fallback `COUNT(*)` on qualified ident.
- `ForeignKeys`: join `pg_namespace` on both sides; predicate
  `(c.relname=$1 AND nc.nspname=$2) OR (c2.relname=$1 AND nc2.nspname=$2)`.

New resolver (pg only; mysql/sqlite return input):

```go
func (d *DB) ResolveTable(ctx context.Context, name string) (QualTable, error)
```

- If `ParseQualTable` yields Schema != "" → verify exists (`to_regclass` / information_schema), else `table "s.t" not found`.
- Bare: `SELECT table_schema FROM information_schema.tables WHERE table_name=$1 AND table_schema NOT IN (<system list>)` — reuse `SystemSchemas` filter from `internal/db/schema.go:16`.
- 0 rows → `table "x" not found`; 1 → return it; 2+ → `table "x" is ambiguous: a.x, b.x — qualify as schema.table` (sorted, public first).

`UpdateCell`/`PreviewTarget`: take the already-resolved `QualTable`; no re-resolution inside.

## 4. Caller changes

- TUI `commands.go`: `loadDetail(schema, table)` builds `QualTable{schema, table}`;
  `loadRowsPage`, `loadCount`, `loadERCtx`/`loadERSchemaCtx`, ER column fetch
  (`commands.go:295`), `runQuery` write-preview (`commands.go:236`), cell edit —
  all pass the qual; `update.go:1430` (`loadDetail("", name)`) resolves schema via
  `explorer.schemaOf` first, falling back to `ResolveTable` on empty.
- CLI `main.go`: `--schema/--rows` args parsed with `ParseQualTable`; bare names go
  through `ResolveTable` (pg) before `Columns/Count/PageRows`; errors print to
  stderr with exit 1 (existing `fatal` path).
- Status/echo lines print `q.String()` so `analytics.users` is visible.

## 5. Ambiguity UX

Exact error: `table "users" is ambiguous: analytics.users, public.users — qualify as schema.table`.
Public sorts first; list capped at 5 + `(+N more)`.

## 6. Verification

- `go vet ./...` clean; `go test ./...` all pkgs ok (existing sqlite tests unchanged: `schema_test.go:33` main/users).
- New unit tests: `ParseQualTable` table (a.t, quoted `"a"."t"`, bare, empty-side error, 2-dot error), `QuoteQual` per driver, ambiguity-error text golden.
- Manual (pg, when available): two schemas with same table name; `--list`, `--rows a.x`, `--rows x` (error), TUI detail/rows/query/ER on each.
- No live pg in CI: pg coverage is SQL-text/unit only.

## 7. Files touched

- ADD `internal/db/qual.go`, `internal/db/qual_test.go`
- EDIT `internal/db/db.go` (7 signatures + pg predicates + quoting)
- EDIT `internal/db/schema.go` (`ApproxCount` join)
- EDIT `internal/db/refs.go` (namespace joins + `[]QualTable`)
- EDIT `internal/db/write.go` (`UpdateCell`, `PreviewTarget` qual)
- EDIT `internal/tui/commands.go`, `internal/tui/update.go` (pass qual through)
- EDIT `main.go` (parse + resolve CLI names)
- ADD this spec `docs/superpowers/specs/2026-10-02-schema-qualified-design.md`
