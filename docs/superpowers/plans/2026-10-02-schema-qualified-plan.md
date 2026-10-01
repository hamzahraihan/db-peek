# Schema-Qualified Tables Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Address any table as `schema.table` end-to-end (TUI + CLI) with an ambiguity error for bare duplicates on Postgres.

**Architecture:** New `QualTable` value type in `internal/db/qual.go` owns parsing/quoting; eight `DB` methods take it instead of bare strings; pg predicates gain `table_schema`/namespace scoping; TUI threads explorer schema through; CLI parses and resolves; mysql/sqlite ignore `Schema` except quoting.

**Tech Stack:** Go 1.26+, `database/sql`, existing `pgx`/`mysql`/`modernc.org/sqlite` drivers, bubbletea MVU (no new deps).

**Spec:** `docs/superpowers/specs/2026-10-02-schema-qualified-design.md`

## Global Constraints

- Go floor 1.26 (`go.mod:3` says `go 1.26.0`).
- 3-part `db.schema.table` names are rejected, never parsed.
- Bare single-match names keep working; bare 2+ matches error, never first-match silently.
- mysql scope stays `DATABASE()`; sqlite scope stays `main`; no semantics change there.
- Ambiguity error text verbatim: `table "users" is ambiguous: analytics.users, public.users — qualify as schema.table` (public first, cap 5 + `(+N more)`).
- No live pg in CI: pg coverage is SQL-text/unit only.

## Review Focus

- Ident with an embedded double-quote (`we"ird`) must render as `"we""ird"` (escaped), not break quoting — pinned in Task 1 (`TestQuoteQualEscapes`).
- `a.` / `.b` / `a.b.c` / `""` must error, never silently trim to a bare name — pinned in Task 1 (`TestParseQualTableRejects`).
- `QualTable{Schema:"x", Name:"t"}` on sqlite/mysql must ignore schema and work (bare compat) — pinned in Task 2 (`TestQualSchemaIgnoredSQLite`).
- Bare pg name matching only system schemas must report not-found, not ambiguous — pinned in Task 3 (`TestAmbiguousErrExcludesSystem` documents filter; resolver SQL carries the same `NOT IN` list).
- TUI detail load on an ambiguous pg table must show the ambiguity error, not merged cross-schema columns — pinned in Task 4 (`TestAmbiguousDetailErr` via `candidates` helper).

---

### Task 1: QualTable type + parse/quote

**Files:**
- Create: `internal/db/qual.go`
- Create: `internal/db/qual_test.go`

**Interfaces:**
- Consumes: `Driver.QuoteIdent` (`internal/db/db.go:28-35`)
- Produces: `QualTable{Schema, Name string}`, `ParseQualTable(string) (QualTable, error)`, `(QualTable) String() string`, `(Driver) QuoteQual(QualTable) string`, `AmbiguousErr(string, []QualTable) error`

- [ ] **Step 1: Write the failing test**

```go
package db

import "testing"

func TestParseQualTable(t *testing.T) {
	q, err := ParseQualTable("analytics.users")
	if err != nil {
		t.Fatal(err)
	}
	if q.Schema != "analytics" || q.Name != "users" {
		t.Fatalf("got %+v", q)
	}
	if q.String() != "analytics.users" {
		t.Fatalf("string = %q", q.String())
	}
	bare, err := ParseQualTable("users")
	if err != nil || bare.Schema != "" || bare.Name != "users" || bare.String() != "users" {
		t.Fatalf("bare = %+v, err = %v", bare, err)
	}
	if got := Postgres.QuoteQual(q); got != `"analytics"."users"` {
		t.Fatalf("quote = %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestParseQualTable -v`
Expected: FAIL with "undefined: ParseQualTable"

- [ ] **Step 3: Write minimal implementation**

```go
package db

import (
	"fmt"
	"sort"
	"strings"
)

// QualTable is a possibly-schema-qualified table reference. Schema == ""
// means bare (sqlite/mysql always; pg before resolution).
type QualTable struct {
	Schema string
	Name   string
}

func (q QualTable) String() string {
	if q.Schema == "" {
		return q.Name
	}
	return q.Schema + "." + q.Name
}

func unquotePart(p string) string {
	p = strings.TrimSpace(p)
	for {
		if len(p) >= 2 && p[0] == '"' && p[len(p)-1] == '"' {
			p = p[1 : len(p)-1]
			continue
		}
		if len(p) >= 2 && p[0] == '`' && p[len(p)-1] == '`' {
			p = p[1 : len(p)-1]
			continue
		}
		if len(p) >= 2 && p[0] == '[' && p[len(p)-1] == ']' {
			p = p[1 : len(p)-1]
			continue
		}
		return p
	}
}

// ParseQualTable splits "schema.table" or bare "table". More than one dot,
// or an empty side, is an error: db.schema.table is out of scope.
func ParseQualTable(s string) (QualTable, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return QualTable{}, fmt.Errorf("bad table name %q: want [schema.]table", s)
	}
	if strings.Count(t, ".") > 1 {
		return QualTable{}, fmt.Errorf("bad table name %q: want [schema.]table", s)
	}
	if strings.Count(t, ".") == 0 {
		name := unquotePart(t)
		if name == "" || strings.ContainsAny(name, " \t\n\r\"'`()[].,;") {
			return QualTable{}, fmt.Errorf("bad table name %q: want [schema.]table", s)
		}
		return QualTable{Name: name}, nil
	}
	i := strings.LastIndex(t, ".")
	schema := unquotePart(t[:i])
	name := unquotePart(t[i+1:])
	if schema == "" || name == "" {
		return QualTable{}, fmt.Errorf("bad table name %q: want [schema.]table", s)
	}
	if strings.ContainsAny(schema, " \t\n\r\"'`()[].,;") || strings.ContainsAny(name, " \t\n\r\"'`()[].,;") {
		return QualTable{}, fmt.Errorf("bad table name %q: want [schema.]table", s)
	}
	return QualTable{Schema: schema, Name: name}, nil
}

// QuoteQual renders a qualified ident: each part goes through QuoteIdent,
// so embedded quotes escape (`we"ird` → `"we""ird"`). Empty schema keeps
// today's single-ident rendering (sqlite/mysql compatible).
func (d Driver) QuoteQual(q QualTable) string {
	if q.Schema == "" {
		return d.QuoteIdent(q.Name)
	}
	return d.QuoteIdent(q.Schema) + "." + d.QuoteIdent(q.Name)
}

// AmbiguousErr formats the duplicate-name error: public first, cap 5.
// Exported: the TUI surfaces it without a DB roundtrip (Task 4) and the
// resolver returns it (Task 3).
func AmbiguousErr(name string, quals []QualTable) error {
	cp := append([]QualTable(nil), quals...)
	sort.Slice(cp, func(i, j int) bool {
		if (cp[i].Schema == "public") != (cp[j].Schema == "public") {
			return cp[i].Schema == "public"
		}
		if cp[i].Schema != cp[j].Schema {
			return cp[i].Schema < cp[j].Schema
		}
		return cp[i].Name < cp[j].Name
	})
	strs := make([]string, len(cp))
	for i, q := range cp {
		strs[i] = q.String()
	}
	if len(strs) > 5 {
		strs = append(strs[:5], "(+N more)")
	}
	return fmt.Errorf("table %q is ambiguous: %s — qualify as schema.table", name, strings.Join(strs, ", "))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/db/ -run TestParseQualTable -v`
Expected: PASS

- [ ] **Step 5: Add rejection + escape tests, watch fail then pass**

```go
func TestParseQualTableRejects(t *testing.T) {
	for _, s := range []string{"", "a.", ".b", "a.b.c", `""`, "a b"} {
		if _, err := ParseQualTable(s); err == nil {
			t.Fatalf("ParseQualTable(%q) = nil error", s)
		}
	}
}

func TestQuoteQualEscapes(t *testing.T) {
	q := QualTable{Schema: `we"ird`, Name: "t"}
	if got := Postgres.QuoteQual(q); got != `"we""ird"."t"` {
		t.Fatalf("quote = %q", got)
	}
	if got := MySQL.QuoteQual(QualTable{Schema: "s", Name: "t"}); got != "`s`.`t`" {
		t.Fatalf("mysql quote = %q", got)
	}
	if got := SQLite.QuoteQual(QualTable{Name: "t"}); got != `"t"` {
		t.Fatalf("bare quote = %q", got)
	}
}
```

Run: `go test ./internal/db/ -run 'TestParseQualTableRejects|TestQuoteQualEscapes' -v`
Expected: PASS (implementation from Step 3 already covers; this pins Review Focus lines 1-2)

- [ ] **Step 6: Commit**

```bash
git add internal/db/qual.go internal/db/qual_test.go
git commit -m "db: add QualTable parse and quote"
```

### Task 2: Core db methods take QualTable

**Files:**
- Modify: `internal/db/db.go:222` (`Columns`), `internal/db/db.go:307` (`PrimaryKey`), `internal/db/db.go:370` (`Indexes`), `internal/db/db.go:524` (`Count`), `internal/db/db.go:535` (`SampleRows`), `internal/db/db.go:541` (`PageRows`)
- Test: `internal/db/qual_test.go` (append), existing `internal/db/*_test.go`

**Interfaces:**
- Consumes: `QualTable`, `(Driver) QuoteQual` from Task 1
- Produces: new signatures `Columns(ctx, QualTable)`, `Indexes(ctx, QualTable)`, `Count(ctx, QualTable)`, `PageRows(ctx, QualTable, int, int)`, `SampleRows(ctx, QualTable, int)`, `PrimaryKey(ctx, QualTable)`

- [ ] **Step 1: Write the failing sqlite-compat test**

```go
func TestQualSchemaIgnoredSQLite(t *testing.T) {
	d := openMem(t)
	defer d.SQL.Close()
	ctx := context.Background()
	bare, err := d.Columns(ctx, QualTable{Name: "users"})
	if err != nil {
		t.Fatal(err)
	}
	qual, err := d.Columns(ctx, QualTable{Schema: "main", Name: "users"})
	if err != nil {
		t.Fatal(err)
	}
	if len(bare) != len(qual) {
		t.Fatalf("bare %d cols vs qual %d cols", len(bare), len(qual))
	}
	if _, err := d.Count(ctx, QualTable{Schema: "main", Name: "users"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.PageRows(ctx, QualTable{Schema: "main", Name: "users"}, 5, 0); err != nil {
		t.Fatal(err)
	}
}
```

Note: `openMem` is the existing helper in `schema_test.go:9` (opens `:memory:`, migrates `users` with 2 rows).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestQualSchemaIgnoredSQLite -v`
Expected: FAIL with "too many arguments" / "cannot use QualTable as string" (signatures not yet changed)

- [ ] **Step 3: Change signatures + pg predicates + quoting**

`Columns`: `func (d *DB) Columns(ctx context.Context, q QualTable) ([]Column, error)`; pg query `WHERE table_name = $1 AND table_schema = $2 ORDER BY ordinal_position` with args `q.Name, q.Schema`; mysql keeps `WHERE table_schema = DATABASE() AND table_name = ?` with `q.Name`; sqlite `PRAGMA table_info(d.Driver.QuoteQual(q))`.

`PrimaryKey`: `func (d *DB) PrimaryKey(ctx context.Context, q QualTable) ([]string, error)`; pg query becomes:

```sql
SELECT a.attname
FROM pg_index i
JOIN pg_class c ON c.oid = i.indrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
WHERE c.relname = $1 AND n.nspname = $2 AND i.indisprimary
ORDER BY array_position(i.indkey, a.attnum)
```

with args `q.Name, q.Schema`; mysql uses `q.Name`; sqlite default branch calls `d.Columns(ctx, q)`.

`Indexes`: `func (d *DB) Indexes(ctx context.Context, q QualTable) ([]Index, error)`; pg `SELECT indexname, indexdef FROM pg_indexes WHERE tablename = $1 AND schemaname = $2 ORDER BY indexname`; mysql `SHOW INDEX FROM QuoteQual(q)`; sqlite `sqlite_master ... tbl_name = ?` with `q.Name` and `PRAGMA index_list(QuoteQual(q))`, `PRAGMA index_info` unchanged (index names are unqualified).

`Count`: `func (d *DB) Count(ctx context.Context, q QualTable) (int64, error)` with `SELECT COUNT(*) FROM QuoteQual(q)`.

`PageRows`: `func (d *DB) PageRows(ctx context.Context, q QualTable, limit, offset int) (*Sample, error)` with `SELECT * FROM QuoteQual(q) LIMIT … OFFSET …`; `SampleRows` takes `QualTable` and forwards.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/db/ -v 2>&1 | tail -8`
Expected: all PASS including `TestQualSchemaIgnoredSQLite` and existing `TestListTablesInSchemaSQLite`

- [ ] **Step 5: Commit**

```bash
git add internal/db/db.go internal/db/qual_test.go
git commit -m "db: thread QualTable through core methods"
```

### Task 3: ApproxCount + FK resolver

**Files:**
- Modify: `internal/db/schema.go:139` (`ApproxCount`), `internal/db/refs.go:15` (`ForeignKeys`), `internal/db/refs.go:129` (`AllForeignKeys`), `internal/db/refs.go:159` (`sqliteAllForeignKeys`)
- Create: resolver in `internal/db/schema.go` + tests in `internal/db/qual_test.go`

**Interfaces:**
- Consumes: `QualTable`, `AmbiguousErr`, `SystemSchemas` (`internal/db/schema.go:16`)
- Produces: `ApproxCount(ctx, QualTable)`, `ForeignKeys(ctx, QualTable)`, `AllForeignKeys(ctx, []QualTable)`, `ResolveTable(ctx, string) (QualTable, error)`

- [ ] **Step 1: Write the failing resolver tests**

```go
func TestAmbiguousErrFormat(t *testing.T) {
	err := AmbiguousErr("users", []QualTable{{Schema: "analytics", Name: "users"}, {Schema: "public", Name: "users"}})
	want := `table "users" is ambiguous: public.users, analytics.users — qualify as schema.table`
	if err.Error() != want {
		t.Fatalf("got %q want %q", err.Error(), want)
	}
}

func TestResolveTableSQLitePassthrough(t *testing.T) {
	d := openMem(t)
	defer d.SQL.Close()
	ctx := context.Background()
	q, err := d.ResolveTable(ctx, "users")
	if err != nil || q.Name != "users" {
		t.Fatalf("got %+v, %v", q, err)
	}
	if _, err := d.ResolveTable(ctx, "no_such_table_xyz"); err == nil {
		t.Fatal("expected not-found error")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/db/ -run 'TestAmbiguousErrFormat|TestResolveTableSQLitePassthrough' -v`
Expected: FAIL with "undefined: ResolveTable". (`AmbiguousErr` exists from Task 1, so the format test passes; the resolver is what's missing.)

- [ ] **Step 3: Implement ApproxCount + ForeignKeys + ResolveTable**

`ApproxCount`: `func (d *DB) ApproxCount(ctx context.Context, q QualTable) (int64, error)`; pg branch `SELECT reltuples::bigint FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE c.relname = $1 AND n.nspname = $2` args `q.Name, q.Schema`; fallback `SELECT COUNT(*) FROM QuoteQual(q)`.

`ForeignKeys`: `func (d *DB) ForeignKeys(ctx context.Context, q QualTable) ([]ForeignKey, error)`; pg query adds `JOIN pg_namespace nc ON nc.oid = c.oid JOIN pg_namespace nc2 ON nc2.oid = c2.oid` and predicate `WHERE o.contype = 'f' AND ((c.relname = $1 AND nc.nspname = $2) OR (c2.relname = $1 AND nc2.nspname = $2)) ORDER BY 1, 2` args `q.Name, q.Schema, q.Name, q.Schema`; mysql filters in Go are unchanged (uses `q.Name` twice); sqlite outgoing uses `PRAGMA foreign_key_list(QuoteQual(q))` and compares `to == q.Name`, incoming compares `to == q.Name` with `FromTable: n`.

`AllForeignKeys`/`sqliteAllForeignKeys` take `[]QualTable`; sqlite matches by `Name`; dedupe keys use `FromTable|FromColumn|ToTable|ToColumn` as today.

`ResolveTable` in `schema.go`:

```go
func (d *DB) ResolveTable(ctx context.Context, name string) (QualTable, error) {
	q, err := ParseQualTable(name)
	if err != nil {
		return QualTable{}, err
	}
	if d.Driver != Postgres {
		if q.Schema == "" {
			return QualTable{Name: q.Name}, nil
		}
		return q, nil
	}
	if q.Schema != "" {
		var ok bool
		err := d.SQL.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = $1 AND table_name = $2)`, q.Schema, q.Name).Scan(&ok)
		if err != nil {
			return QualTable{}, err
		}
		if !ok {
			return QualTable{}, fmt.Errorf("table %q not found", q.String())
		}
		return q, nil
	}
	rows, err := d.SQL.QueryContext(ctx, `SELECT table_schema FROM information_schema.tables WHERE table_name = $1 AND table_schema NOT IN ('pg_catalog','information_schema','auth','storage','realtime','extensions','graphql','graphql_public','supabase_functions','supabase_migrations','vault','pgsodium','pgsodium_masks','net','postgis','_postgis','tiger','tiger_data','topology','cron','_realtime','_supabase') AND table_schema NOT LIKE 'pg\_%' ESCAPE '\' AND table_schema NOT LIKE 'pg_temp_%' AND table_schema NOT LIKE 'pg_toast%' ORDER BY CASE WHEN table_schema='public' THEN 0 ELSE 1 END, table_schema`, q.Name)
	if err != nil {
		return QualTable{}, err
	}
	defer rows.Close()
	var quals []QualTable
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return QualTable{}, err
		}
		quals = append(quals, QualTable{Schema: s, Name: q.Name})
	}
	if err := rows.Err(); err != nil {
		return QualTable{}, err
	}
	switch len(quals) {
	case 0:
		return QualTable{}, fmt.Errorf("table %q not found", q.Name)
	case 1:
		return quals[0], nil
	default:
		return QualTable{}, AmbiguousErr(q.Name, quals)
	}
}
```

(The `NOT IN` list mirrors `SystemSchemas` at `internal/db/schema.go:16` plus the `pg_%` guards from `ListTables` at `internal/db/db.go:199`.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/db/ -v 2>&1 | tail -8`
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/db/schema.go internal/db/refs.go internal/db/qual_test.go
git commit -m "db: schema-scoped counts, FKs, and ResolveTable"
```

### Task 4: TUI thread-through + ambiguity surface

**Files:**
- Modify: `internal/tui/commands.go:78-145` (`loadDetailCtx`, `loadCount`, `loadRowsPage`), ER/count call sites (`commands.go:158,187,236,266,295,316`), `internal/tui/update.go:1430`, `internal/tui/explorer.go:249`
- Test: `internal/tui/` new `qual_test.go`

**Interfaces:**
- Consumes: `QualTable`, `AmbiguousErr` (db), `Explorer.schemaOf` + new `Explorer.candidates`
- Produces: detail/rows/count/ER loads scoped to `QualTable{explorer schema, table}`; ambiguous pg selection shows resolver-style error with zero DB calls

- [ ] **Step 1: Write the failing candidates test**

```go
package tui

import "testing"

func TestCandidatesAmbiguous(t *testing.T) {
	e := NewExplorer("", []string{})
	e.Schemas = []SchemaNode{
		{Name: "public", Tables: []TableNode{{Name: "users"}}},
		{Name: "analytics", Tables: []TableNode{{Name: "users"}}},
	}
	got := e.candidates("users")
	if len(got) != 2 {
		t.Fatalf("candidates = %v", got)
	}
	if got[0] != "public" || got[1] != "analytics" {
		t.Fatalf("order = %v", got)
	}
	if len(e.candidates("missing")) != 0 {
		t.Fatal("expected no candidates")
	}
}
```

Note: `SchemaNode{Name, Tables}`, `TableNode{Name}` shapes per `explorer.go`; `NewExplorer("", []string{})` per `model.go` construction. If field names differ, use the real ones from `explorer.go` — the assertion (2 candidates, public first, empty for missing) is the contract.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestCandidatesAmbiguous -v`
Expected: FAIL with "undefined: candidates"

- [ ] **Step 3: Implement candidates + thread qual through**

In `explorer.go` after `schemaOf`:

```go
// candidates returns every schema owning table, public first then alpha.
func (e *Explorer) candidates(table string) []string {
	var out []string
	for _, s := range e.Schemas {
		for _, tb := range s.Tables {
			if tb.Name == table {
				out = append(out, s.Name)
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i] == "public") != (out[j] == "public") {
			return out[i] == "public"
		}
		return out[i] < out[j]
	})
	return out
}
```

In `commands.go loadDetailCtx`: build `q := dbpkg.QualTable{Schema: schema, Name: table}` once; when `schema == "" && db.Driver == dbpkg.Postgres`, resolve against the explorer instead of guessing: the caller passes candidates — implement inside `loadDetailCtx` via a `resolve func(string) []string` field? No new plumbing: handle at the two call sites. `inspectTable` already receives schema; `update.go:1430` (ER-driven selection) changes to:

```go
schema := m.explorer.schemaOf(name)
if schema == "" && m.db != nil && m.db.Driver == dbpkg.Postgres {
	if cands := m.explorer.candidates(name); len(cands) > 1 {
		quals := make([]dbpkg.QualTable, len(cands))
		for i, s := range cands {
			quals[i] = dbpkg.QualTable{Schema: s, Name: name}
		}
		// surface without a DB roundtrip: reuse the seq-guarded error path
		m.loading = false
		m.err = dbpkg.AmbiguousErr(name, quals).Error()
		return m, nil
	}
}
return m, m.loadDetail(schema, name)
```

This uses the exported formatter from Task 1 — no rename needed. Then `loadDetailCtx` uses `db.Columns(ctx, q)`, `db.Indexes(ctx, q)`, `db.PageRows(ctx, q, size, 0)`; `loadCount` keys `m.counts` by `QualTable{…}.String()` and calls `db.Count(ctx, q)`; `loadRowsPage` builds `q` from `m.explorer.schemaOf(m.table)` + `m.table`; ER/count sites pass their existing schema var into the qual.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/ -run TestCandidatesAmbiguous -v && go test ./... 2>&1 | tail -6`
Expected: PASS, full suite green

- [ ] **Step 5: Commit**

```bash
git add internal/tui/ internal/db/qual.go
git commit -m "tui: scope detail rows counts ER to schema"
```

### Task 5: CLI parse + resolve + full verification

**Files:**
- Modify: `main.go:165-203` (`--schema`/`--rows` blocks)
- Test: manual sqlite runs + full suite (no new test file; CLI is `package main` with no harness)

**Interfaces:**
- Consumes: `ParseQualTable`, `(DB) ResolveTable`, `QualTable.String()` for echo lines
- Produces: `db-peek [conn] --schema [schema.]TABLE` and `--rows [schema.]TABLE` with pg ambiguity errors on stderr exit 1

- [ ] **Step 1: Verify current CLI still takes bare names (baseline)**

Run: `go build -o $env:TEMP\db-peek-qual.exe . && & $env:TEMP\db-peek-qual.exe ./test.db --list`
Expected: prints `users` (baseline before edit; proves binary works)

- [ ] **Step 2: Implement CLI parse + resolve**

In `main.go`, at the top of each of the `--schema` and `--rows` cases, insert:

```go
qt, err := dbpkg.ParseQualTable(*schemaF) // (*rowsF in the rows case)
if err != nil {
	fatal(err)
}
if qt.Schema == "" {
	if r, rerr := db.ResolveTable(ctx, qt.Name); rerr != nil {
		fatal(rerr)
	} else {
		qt = r
	}
}
```

then replace every `*schemaF`/`*rowsF` use in that case with `qt.Name` for display of the bare name? No — echo lines and `Columns/Indexes/Count/PageRows` take the qual: `db.Columns(ctx, qt)`, `db.Indexes(ctx, qt)`, `db.Count(ctx, qt)`, `db.PageRows(ctx, qt, *limitF, *offsetF)`; echo headers print `qt.String()` so `analytics.users` is visible. sqlite/mysql `ResolveTable` is a passthrough, so single-code-path works on all drivers.

- [ ] **Step 3: Verify qualified + bare sqlite runs**

Run: `& $env:TEMP\db-peek-qual.exe ./test.db --list` then rebuild and run `go build -o $env:TEMP\db-peek-qual.exe . && & $env:TEMP\db-peek-qual.exe ./test.db --rows users --limit 2 && & $env:TEMP\db-peek-qual.exe ./test.db --rows main.users --limit 2 && & $env:TEMP\db-peek-qual.exe ./test.db --schema users`
Expected: bare and `main.users` print the same rows/columns; no behavior change on sqlite

- [ ] **Step 4: Run full suite + vet**

Run: `go vet ./... && go test ./... 2>&1 | tail -6`
Expected: vet clean, all 4 pkgs ok

- [ ] **Step 5: Commit**

```bash
git add main.go
git commit -m "cli: accept schema.table for rows and schema"
```
