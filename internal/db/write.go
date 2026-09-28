package db

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Write statements (INSERT/UPDATE/DELETE/DDL) and the PgBouncer-safe
// postgres connector knob live here. The query tab runs row-returning
// statements through Query and everything else through ExecStmt.

// withSimpleProtocol forces pgx onto the simple query protocol, which
// proxies such as PgBouncer/Supavisor support (unlike protocol-level
// prepared statements, which fail with 08P01 invalid message format).
// A user-supplied default_query_exec_mode always wins; non-postgres
// connection strings pass through untouched. pgx consumes the keyword
// client-side, so it is never sent to the server.
func withSimpleProtocol(raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	if !strings.HasPrefix(lower, "postgres://") && !strings.HasPrefix(lower, "postgresql://") {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	if q.Get("default_query_exec_mode") != "" {
		return raw
	}
	q.Set("default_query_exec_mode", "simple_protocol")
	u.RawQuery = q.Encode()
	return u.String()
}

// trimStatement drops surrounding whitespace and trailing semicolons so
// the classifier sees the real first keyword and drivers never receive
// an empty trailing statement.
func trimStatement(q string) string {
	s := strings.TrimSpace(q)
	for strings.HasSuffix(s, ";") {
		s = strings.TrimSpace(strings.TrimSuffix(s, ";"))
	}
	return s
}

// stripLeadingComments removes leading whitespace and -- / /* */ comments.
func stripLeadingComments(q string) string {
	s := strings.TrimSpace(q)
	for {
		if strings.HasPrefix(s, "--") {
			if i := strings.Index(s, "\n"); i >= 0 {
				s = strings.TrimSpace(s[i+1:])
				continue
			}
			return ""
		}
		if strings.HasPrefix(s, "/*") {
			if i := strings.Index(s, "*/"); i >= 0 {
				s = strings.TrimSpace(s[i+2:])
				continue
			}
			return s
		}
		return s
	}
}

// isWriteStatement reports whether sql should run through Exec (rows
// affected) rather than Query (result grid). Row-returning leading
// keywords stay on the query path.
func isWriteStatement(sql string) bool {
	s := stripLeadingComments(sql)
	if s == "" {
		return false
	}
	if hasReturningClause(sql) {
		return false
	}
	word := s
	for i, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '(' {
			word = s[:i]
			break
		}
	}
	switch strings.ToUpper(word) {
	case "SELECT", "WITH", "VALUES", "TABLE", "EXPLAIN":
		return false
	default:
		return true
	}
}

// hasReturningClause reports whether sql carries a standalone RETURNING
// keyword (INSERT/UPDATE/DELETE ... RETURNING). Matching is
// case-insensitive on word boundaries after stripping leading comments.
// A column literally named `returning` is an accepted false positive: it
// routes via Query and the driver error surfaces normally.
func hasReturningClause(sql string) bool {
	lower := strings.ToLower(stripLeadingComments(sql))
	for i := strings.Index(lower, "returning"); i >= 0; i = strings.Index(lower, "returning") {
		beforeOK := i == 0 || !isIdentChar(lower[i-1])
		afterIdx := i + len("returning")
		afterOK := afterIdx >= len(lower) || !isIdentChar(lower[afterIdx])
		if beforeOK && afterOK {
			return true
		}
		lower = lower[i+1:]
	}
	return false
}

func isIdentChar(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
}

// parseWriteTable extracts the single target table of a write statement:
// INSERT INTO <t>, UPDATE <t>, DELETE FROM <t>, or DDL
// (CREATE/DROP/ALTER TABLE <t>). Returns ("", "") when the target is
// unclear (SELECT/WITH/CTE/multi-statement). Quoting (`"foo"`, “ `foo` “,
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
			i += 2
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
	t = strings.Trim(t, "\"`[]\"")
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

// HintForError returns an actionable suffix for known infrastructure
// failures. A protocol-violation through a pooler almost always means the
// pooler mangled the wire flow, so point at the direct connection instead
// of the cryptic server code. Empty string when no hint applies.
func HintForError(connStr string, err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if !strings.Contains(msg, "08P01") && !strings.Contains(msg, "invalid message format") {
		return ""
	}
	if lower := strings.ToLower(connStr); strings.Contains(lower, ":6543") || strings.Contains(lower, "pooler") {
		return " (pooler detected — retry with the direct :5432 connection)"
	}
	return " (protocol error — retry with the direct database connection)"
}

// Explain returns the engine-appropriate EXPLAIN SQL for q. SQLite has
// no EXPLAIN ANALYZE, so analyze is ignored there.
func (d *DB) Explain(q string, analyze bool) string {
	q = trimStatement(q)
	switch d.Driver {
	case Postgres:
		if analyze {
			return "EXPLAIN (ANALYZE, BUFFERS) " + q
		}
		return "EXPLAIN " + q
	case MySQL:
		if analyze {
			return "EXPLAIN ANALYZE " + q
		}
		return "EXPLAIN " + q
	default:
		return "EXPLAIN QUERY PLAN " + q
	}
}

// Placeholder returns the driver-specific bind marker for the i-th
// (1-based) parameter: Postgres numbers them, MySQL and SQLite use "?".
func (d Driver) Placeholder(i int) string {
	if d == Postgres {
		return "$" + strconv.Itoa(i)
	}
	return "?"
}

// UpdateCell issues a single-parameter UPDATE identified by the full
// primary key. It runs as one autocommit statement, which is atomic on
// every supported engine.
//
// pkVals must carry the driver's own value types, not display strings:
// Postgres has no implicit cast, so binding a string for a bigint key
// fails with "operator does not exist: bigint = text".
func (d *DB) UpdateCell(ctx context.Context, table string, pk []string, pkVals []any, col string, val any) (int64, error) {
	if len(pk) == 0 || len(pk) != len(pkVals) {
		return 0, fmt.Errorf("update %s: primary key value count does not match the key", table)
	}
	q := d.Driver.QuoteIdent
	var b strings.Builder
	fmt.Fprintf(&b, "UPDATE %s SET %s = %s WHERE ", q(table), q(col), d.Driver.Placeholder(1))
	for i, k := range pk {
		if i > 0 {
			b.WriteString(" AND ")
		}
		fmt.Fprintf(&b, "%s = %s", q(k), d.Driver.Placeholder(i+2))
	}
	args := make([]any, 0, len(pkVals)+1)
	args = append(args, val)
	for _, v := range pkVals {
		args = append(args, v)
	}
	res, err := d.SQL.ExecContext(ctx, b.String(), args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// RunUserQuery routes one editor statement: row-returning statements go
// through Query (grid), everything else through ExecStmt (rows affected,
// returned as n with a nil sample). n is -1 for the query path.
func (d *DB) RunUserQuery(ctx context.Context, sql string) (s *Sample, n int64, err error) {
	if !isWriteStatement(sql) {
		s, err := d.Query(ctx, trimStatement(sql))
		return s, -1, err
	}
	n, err = d.ExecStmt(ctx, sql)
	return nil, n, err
}

// ExecStmt runs a non-row-returning statement and reports rows affected.
func (d *DB) ExecStmt(ctx context.Context, sql string) (int64, error) {
	res, err := d.SQL.ExecContext(ctx, trimStatement(sql))
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
}
