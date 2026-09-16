# Query Result View After Writes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** After a successful write in the TUI query tab, show the affected table's rows (or RETURNING rows) in the existing results grid with an `affected • preview of` footer.

**Architecture:** Fix the `db` classifier so `RETURNING` statements use the Query path; add a single-table parser; chain a best-effort `PageRows(tbl, 20, 0)` preview inside the existing `runQuery` command and carry it in one enriched `queryDoneMsg`.

**Tech Stack:** Go 1.23+, Bubble Tea Elm (Model/Update/View), `modernc.org/sqlite` / `pgx` / `go-sql-driver` via `internal/db`.

## Global Constraints

- Preview is fixed `LIMIT 20`, heap order, no `ORDER BY`.
- No new screen or pane; reuse `queryTable` grid and `view_detail.go` footer.
- `connSeq` / `querySeq` guards stay unchanged; preview shares the originating seq.
- Preview SELECT failure falls back to the affected-only line (swallowed).
- TUI only in v1; CLI `--query` output is unchanged.
- Multi-statement input is out of scope (first statement's table only).

---

### Task 1: RETURNING classifier fix

**Files:**
- Modify: `internal/db/write.go`
- Test: `internal/db/write_test.go`

**Interfaces:**
- Consumes: existing `stripLeadingComments(q string) string`, `isWriteStatement(sql string) bool`.
- Produces: `hasReturningClause(sql string) bool` — case-insensitive standalone-word match for `RETURNING` after stripping leading comments; `isWriteStatement` returns `false` when it is true.

- [ ] **Step 1: Write the failing test**

```go
func TestReturningGoesThroughQueryPath(t *testing.T) {
    query := []string{
        "INSERT INTO t VALUES (1) RETURNING *",
        "insert into t values (1) returning id",
        "-- comment\nUPDATE t SET a=1 RETURNING a",
        "/* block */ DELETE FROM t RETURNING id",
    }
    for _, q := range query {
        if isWriteStatement(q) {
            t.Fatalf("want query path for %q", q)
        }
        if !hasReturningClause(q) {
            t.Fatalf("want returning detected for %q", q)
        }
    }
    if hasReturningClause("SELECT returning FROM t") {
        // `returning` as a bare column name still matches the keyword;
        // accepted limitation: it routes via Query and surfaces driver errors.
    }
    if hasReturningClause("INSERT INTO t VALUES (1)") {
        t.Fatal("plain insert must not count as returning")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestReturningGoesThroughQueryPath -v`
Expected: FAIL with "hasReturningClause undefined"

- [ ] **Step 3: Write minimal implementation**

```go
// hasReturningClause reports whether sql carries a standalone RETURNING
// keyword (INSERT/UPDATE/DELETE ... RETURNING). Matching is
// case-insensitive on word boundaries after stripping leading comments.
// A column literally named `returning` is an accepted false positive: it
// routes via Query and the driver error surfaces normally.
func hasReturningClause(sql string) bool {
    s := stripLeadingComments(sql)
    lower := strings.ToLower(s)
    for i := strings.Index(lower, "returning"); i >= 0; i = strings.Index(lower, "returning") {
        beforeOK := i == 0 || !isIdentChar(lower[i-1])
        afterIdx := i + len("returning")
        afterOK := afterIdx >= len(lower) || !isIdentChar(lower[afterIdx])
        if beforeOK && afterOK {
            return true
        }
        lower = lower[i+1:]
        s = s[i+1:]
        _ = s
    }
    return false
}

func isIdentChar(c byte) bool {
    return c == '_' || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
}
```

And at the top of `isWriteStatement`, after the empty check:

```go
if hasReturningClause(sql) {
    return false
}
```

(`strings` is already imported in `write.go`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/db/ -run 'TestReturningGoesThroughQueryPath|TestClassifyStatement' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/db/write.go internal/db/write_test.go
git commit -m "feat(db): route RETURNING statements through query path"
```

### Task 2: Single-table write parser

**Files:**
- Modify: `internal/db/write.go`
- Test: `internal/db/write_test.go`

**Interfaces:**
- Consumes: `stripLeadingComments`, `hasReturningClause` from Task 1.
- Produces: `parseWriteTable(sql string) (table string, kind string)` — `kind` is one of `"insert"`, `"update"`, `"delete"`, `"ddl"`, `""`. `table` is the bare unquoted table name (`schema.foo` yields `foo`); `""` means no preview.

- [ ] **Step 1: Write the failing test**

```go
func TestParseWriteTable(t *testing.T) {
    cases := map[string][2]string{
        "INSERT INTO foo VALUES (1)":            {"foo", "insert"},
        `INSERT INTO "foo" VALUES (1)`:          {"foo", "insert"},
        "insert into public.foo values (1)":     {"foo", "insert"},
        "INSERT INTO IF NOT EXISTS foo (a) VALUES (1)": {"foo", "insert"},
        "UPDATE foo SET a=1":                    {"foo", "update"},
        "update public.foo set a=1 where id=2":  {"foo", "update"},
        "DELETE FROM foo WHERE id=1":            {"foo", "delete"},
        "delete from `foo` where id=1":          {"foo", "delete"},
        "CREATE TABLE foo (a INT)":              {"foo", "ddl"},
        "DROP TABLE IF EXISTS foo":              {"foo", "ddl"},
        "ALTER TABLE foo ADD COLUMN b TEXT":     {"foo", "ddl"},
        "SELECT * FROM foo":                     {"", ""},
        "WITH x AS (SELECT 1) SELECT * FROM x":  {"", ""},
        "-- c\n/* b */ INSERT INTO foo VALUES (1)": {"foo", "insert"},
    }
    for q, want := range cases {
        tbl, kind := parseWriteTable(q)
        if tbl != want[0] || kind != want[1] {
            t.Fatalf("parseWriteTable(%q) = (%q,%q), want (%q,%q)", q, tbl, kind, want[0], want[1])
        }
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestParseWriteTable -v`
Expected: FAIL with "parseWriteTable undefined"

- [ ] **Step 3: Write minimal implementation**

```go
// parseWriteTable extracts the single target table of a write statement:
// INSERT INTO <t>, UPDATE <t>, DELETE FROM <t>, or DDL
// (CREATE/DROP/ALTER TABLE <t>). Returns ("", "") when the target is
// unclear (SELECT/WITH/CTE/multi-statement). Quoting (`"foo"`, `` `foo` ``,
// `[foo]`) is stripped and `schema.foo` yields `foo`.
func parseWriteTable(sql string) (string, string) {
    s := stripLeadingComments(sql)
    toks := splitSQLWords(s)
    if len(toks) == 0 {
        return "", ""
    }
    head := strings.ToUpper(toks[0])
    var raw, kind string
    switch head {
    case "INSERT":
        // INSERT INTO [IF NOT EXISTS] <table>
        i := 1
        if i < len(toks) && strings.ToUpper(toks[i]) == "INTO" {
            i++
        } else {
            return "", ""
        }
        for i < len(toks) && (strings.ToUpper(toks[i]) == "IF" || strings.ToUpper(toks[i]) == "NOT" || strings.ToUpper(toks[i]) == "EXISTS") {
            i++
        }
        if i >= len(toks) {
            return "", ""
        }
        raw, kind = toks[i], "insert"
    case "UPDATE":
        if len(toks) < 2 {
            return "", ""
        }
        // UPDATE [OR IGNORE/REPLACE] <table> — skip sqlite OR-conflict clause.
        i := 1
        if strings.ToUpper(toks[i]) == "OR" && i+2 < len(toks) {
            i += 3
        }
        if i >= len(toks) {
            return "", ""
        }
        raw, kind = toks[i], "update"
    case "DELETE":
        // DELETE FROM <table>
        if len(toks) < 3 || strings.ToUpper(toks[1]) != "FROM" {
            return "", ""
        }
        raw, kind = toks[2], "delete"
    case "CREATE", "DROP", "ALTER":
        // CREATE TABLE [IF NOT EXISTS] <t> / DROP TABLE [IF EXISTS] <t> /
        // ALTER TABLE [IF EXISTS] <t>
        i := 1
        if i < len(toks) && strings.ToUpper(toks[i]) == "TABLE" {
            i++
        } else {
            return "", ""
        }
        for i < len(toks) && (strings.ToUpper(toks[i]) == "IF" || strings.ToUpper(toks[i]) == "NOT" || strings.ToUpper(toks[i]) == "EXISTS") {
            i++
        }
        if i >= len(toks) {
            return "", ""
        }
        raw, kind = toks[i], "ddl"
    default:
        return "", ""
    }
    return unquoteTable(raw), kind
}

func splitSQLWords(s string) []string {
    // Split on whitespace and '(' — `INSERT INTO foo(a)` yields ["INSERT","INTO","foo"].
    // Semicolons terminate: only the first statement is considered.
    if i := strings.Index(s, ";"); i >= 0 {
        s = s[:i]
    }
    return strings.FieldsFunc(s, func(r rune) bool {
        return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '('
    })
}

func unquoteTable(raw string) string {
    t := strings.TrimSpace(raw)
    // Strip trailing comma/paren remnants, then schema qualifier.
    t = strings.Trim(t, ",()")
    if i := strings.LastIndex(t, "."); i >= 0 {
        t = t[i+1:]
    }
    t = strings.Trim(t, `"``[]`)
    // A bare "?" or keyword means unclear.
    if t == "" || strings.ContainsAny(t, " \t\n\r\"'`()[].,;") {
        return ""
    }
    return t
}

// PreviewTarget returns the table to auto-preview after sql succeeds,
// and whether it is DDL (the caller refreshes schemas instead of
// gridding). Plain DML yields (table, false); DDL yields (table, true);
// unclear targets yield ("", false).
func PreviewTarget(sql string) (string, bool) {
    tbl, kind := parseWriteTable(sql)
    return tbl, kind == "ddl"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/db/ -v`
Expected: PASS (all db tests incl. Tasks 1–2)

- [ ] **Step 5: Commit**

```bash
git add internal/db/write.go internal/db/write_test.go
git commit -m "feat(db): parse single target table of write statements"
```

### Task 3: Chained preview command + message + update

**Files:**
- Modify: `internal/tui/msg.go`, `internal/tui/model.go`, `internal/tui/commands.go`, `internal/tui/update.go`
- Test: `internal/tui/sidebar_test.go` (append new tests; reuse existing `browseModel(t)` helper)

**Interfaces:**
- Consumes: `dbpkg.RunUserQuery(ctx, sql)`, `dbpkg.PageRows(ctx, table, 20, 0)`, and `dbpkg.PreviewTarget(sql) (table string, isDDL bool)` from Task 2.
- Produces: `queryDoneMsg.previewTable string` + `queryDoneMsg.isDDL bool`; `Model.queryPreviewTable string` (cleared in `clearConnState`); `runQuery` returns preview sample in `sample` with `affected >= 0` for DML writes.

- [ ] **Step 1: Write the failing test**

```go
func TestQueryWritePreviewApplies(t *testing.T) {
    m := browseModel(t)
    m.table = "users"
    m.tab = 3
    m.focusDetail = true
    m.queryFocus = 1
    m.querySeq = 7
    m.width, m.height = 120, 40
    m.resizeBrowse()
    prev := &db.Sample{Columns: []string{"id"}, Rows: [][]string{{"1"}}}
    u, _ := m.Update(queryDoneMsg{sql: "insert into users values (1)", seq: 7, sample: prev, affected: 1, previewTable: "users", ms: 5})
    m = u.(Model)
    if m.querySample == nil || m.queryAffected != 1 {
        t.Fatalf("preview reply must store grid and count, got %+v affected=%d", m.querySample, m.queryAffected)
    }
    if m.queryPreviewTable != "users" {
        t.Fatalf("preview reply must store preview table, got %q", m.queryPreviewTable)
    }
    if len(m.queryTable.cols) != 1 || m.queryTable.cols[0] != "id" {
        t.Fatalf("preview reply must build grid columns, got %+v", m.queryTable.cols)
    }
}

func TestQueryWritePreviewStaleDropped(t *testing.T) {
    m := browseModel(t)
    m.tab = 3
    m.querySeq = 7
    prev := &db.Sample{Columns: []string{"id"}, Rows: [][]string{{"1"}}}
    u, _ := m.Update(queryDoneMsg{sql: "insert", seq: 6, sample: prev, affected: 1, previewTable: "users"})
    m = u.(Model)
    if m.querySample != nil {
        t.Fatal("stale preview reply must not apply")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run 'TestQueryWritePreview' -v`
Expected: FAIL with "previewTable undefined" (unknown field)

- [ ] **Step 3: Write minimal implementation**

`msg.go`:

```go
queryDoneMsg struct {
    sql          string
    sample       *dbpkg.Sample
    affected     int64 // rows affected for writes, -1 for row-returning queries
    previewTable string // auto-preview target for writes, "" when none
    isDDL        bool   // true when the write was DDL (refresh schemas, no grid)
    ms           int64
    seq          int
    conn         int
    err          error
}
```

`model.go` — add the field next to `queryAffected`:

```go
queryPreviewTable string // auto-preview target for the last write, "" when none
```

In `clearConnState` add `m.queryPreviewTable = ""`.

`commands.go` — replace `runQuery` body after `start := time.Now()`:

```go
return func() tea.Msg {
    start := time.Now()
    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()
    s, n, err := db.RunUserQuery(ctx, sql)
    ms := time.Since(start).Milliseconds()
    if err != nil {
        return queryDoneMsg{sql: sql, ms: ms, seq: seq, err: err, conn: conn}
    }
    if n < 0 {
        // SELECT or RETURNING path: returned rows are the result view.
        return queryDoneMsg{sql: sql, sample: s, affected: -1, ms: ms, seq: seq, conn: conn}
    }
    tbl, isDDL := dbpkg.PreviewTarget(sql)
    if isDDL {
        return queryDoneMsg{sql: sql, affected: n, isDDL: true, ms: ms, seq: seq, conn: conn}
    }
    if tbl == "" {
        return queryDoneMsg{sql: sql, affected: n, ms: ms, seq: seq, conn: conn}
    }
    preview, perr := db.PageRows(ctx, tbl, 20, 0)
    if perr != nil {
        // Swallowed by design: affected-count is the source of truth.
        return queryDoneMsg{sql: sql, affected: n, ms: time.Since(start).Milliseconds(), seq: seq, conn: conn}
    }
    return queryDoneMsg{sql: sql, sample: preview, affected: n, previewTable: tbl, ms: time.Since(start).Milliseconds(), seq: seq, conn: conn}
}
```

`update.go` — in the `queryDoneMsg` branch, replace the `if msg.affected >= 0` block:

```go
if msg.affected >= 0 {
    if msg.isDDL {
        // DDL: no grid; refresh the schema tree so the new/dropped
        // table shows up. Keep the affected count for the footer.
        m.querySample = nil
        m.queryPreviewTable = ""
        m.queryAffected = msg.affected
        m.queryMs = msg.ms
        m.queryTable.setData([]string{"rows"}, [][]string{{"(no rows)"}})
        m.sizeTables()
        m.loading = true
        return m, m.loadSchemas()
    }
    if msg.previewTable != "" && msg.sample != nil {
        m.querySample = msg.sample
        m.queryPreviewTable = msg.previewTable
        m.queryAffected = msg.affected
        m.queryMs = msg.ms
        m.err = ""
        var qcols []string
        var qrows [][]string
        qcols = append([]string(nil), msg.sample.Columns...)
        for _, r := range msg.sample.Rows {
            qrows = append(qrows, append([]string(nil), r...))
        }
        if len(qcols) == 0 {
            qcols = []string{"rows"}
            qrows = [][]string{{"(no rows)"}}
        }
        m.queryTable.setData(qcols, qrows)
        m.queryTable.SetHeaderStyles(dataFieldHeaderStyle, dimFieldHeaderStyle)
        m.sizeTables()
        // Best-effort count refresh for the previewed table.
        for _, s := range m.explorer.Schemas {
            for _, tb := range s.Tables {
                if tb.Name == msg.previewTable {
                    return m, m.loadOneCount(s.Name, tb.Name)
                }
            }
        }
        return m, nil
    }
    // Write path without preview: no grid, the view shows rows-affected instead.
    m.querySample = nil
    m.queryPreviewTable = ""
    m.queryAffected = msg.affected
    m.queryMs = msg.ms
    m.queryTable.setData([]string{"rows"}, [][]string{{"(no rows)"}})
    m.sizeTables()
    return m, nil
}
```

Also clear the field on the row-returning path: where the branch handles
`queryDoneMsg` with `msg.err == nil` and `msg.affected < 0` (SELECT /
RETURNING), set `m.queryPreviewTable = ""` alongside `m.querySample`,
`m.queryAffected`, `m.queryMs`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/ -run 'TestQueryWritePreview|TestQueryWriteShowsAffected|TestQueryRunSeqGuard' -v`
Expected: PASS. Then: `go test ./...`
Expected: PASS (full regression)

- [ ] **Step 5: Commit**

```bash
git add internal/tui/msg.go internal/tui/model.go internal/tui/commands.go internal/tui/update.go internal/tui/sidebar_test.go
git commit -m "feat(tui): preview affected table after successful writes"
```

### Task 4: Footer rendering for preview results

**Files:**
- Modify: `internal/tui/view_detail.go`
- Test: `internal/tui/sidebar_test.go` (append; reuse `browseModel(t)` helper)

**Interfaces:**
- Consumes: `Model.querySample`, `Model.queryAffected`, `Model.queryMs`, `Model.queryPreviewTable` from Task 3.
- Produces: footer line `N row(s) affected • preview of <table> • M ms` when a preview is present.

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/sidebar_test.go`:

```go
func TestQueryPreviewFooterNeedsTable(t *testing.T) {
    // Pins the singular/plural preview footer. Fails until view_detail.go
    // renders queryPreviewTable.
    m := browseModel(t)
    m.table = "users"
    m.tab = 3
    m.focusDetail = true
    m.queryFocus = 1
    m.querySeq = 8
    m.width, m.height = 120, 40
    m.resizeBrowse()
    prev := &db.Sample{Columns: []string{"id"}, Rows: [][]string{{"1"}, {"2"}}}
    u, _ := m.Update(queryDoneMsg{sql: "insert", seq: 8, sample: prev, affected: 2, previewTable: "users", ms: 6})
    m = u.(Model)
    if v := m.View(); !strings.Contains(v, "2 rows affected • preview of users") {
        t.Fatalf("plural preview footer missing, got:\n%s", v)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestQueryPreviewFooterNeedsTable -v`
Expected: FAIL (footer shows only "2 rows affected")

- [ ] **Step 3: Write minimal implementation**

`view_detail.go` — replace the `if m.querySample == nil` block (Task 3 already
owns `model.go`/`update.go`; do not touch them here):

```go
if m.querySample == nil {
    if m.queryAffected >= 0 {
        unit := "rows"
        if m.queryAffected == 1 {
            unit = "row"
        }
        b.WriteString(dimStyle.Render(fitText(fmt.Sprintf("%d %s affected • %d ms", m.queryAffected, unit, m.queryMs), m.paneW())) + "\n")
    } else {
        b.WriteString(dimStyle.Render("(no results — ctrl+r to run)") + "\n")
    }
} else {
    b.WriteString(gridView(&m.queryTable) + "\n")
    if m.queryAffected >= 0 && m.queryPreviewTable != "" {
        unit := "rows"
        if m.queryAffected == 1 {
            unit = "row"
        }
        b.WriteString(dimStyle.Render(fitText(fmt.Sprintf("%d %s affected • preview of %s • %d ms", m.queryAffected, unit, m.queryPreviewTable, m.queryMs), m.paneW())) + "\n")
    } else {
        b.WriteString(dimStyle.Render(fitText(fmt.Sprintf("%d rows • %d ms", len(m.querySample.Rows), m.queryMs), m.paneW())) + "\n")
    }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/ -run 'TestQueryWritePreview|TestQueryPreviewFooter|TestQueryWriteShowsAffected' -v`
Expected: PASS. Then: `go test ./...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tui/view_detail.go internal/tui/sidebar_test.go
git commit -m "feat(tui): render preview footer after writes"
```

### Task 5: Regression + manual verification

**Files:** none (verification only).

- [ ] **Step 1: Run the full suite**

Run: `go test ./...`
Expected: PASS, zero failures.

- [ ] **Step 2: Manual check against test.db**

Run:
```bash
go run . "./test.db" --query "CREATE TABLE IF NOT EXISTS _peek_probe (id INTEGER PRIMARY KEY, v TEXT)"
go run . "./test.db" --query "DELETE FROM _peek_probe"
```

Then open the TUI (`go run . "./test.db"`), go to tab `4 query`, run `INSERT INTO _peek_probe (v) VALUES ('hello')` with `ctrl+r`, and confirm the grid shows the `_peek_probe` rows plus the footer `1 row affected • preview of _peek_probe • N ms`. Run `INSERT INTO _peek_probe (v) VALUES ('w') RETURNING *` and confirm the returned row renders. Drop the probe table afterwards with `DROP TABLE _peek_probe`.

- [ ] **Step 3: Verify no stray files**

Run: `git status --short`
Expected: clean (probe table lives inside `test.db`; drop it in the TUI or via `--query` before finishing; no new files).
