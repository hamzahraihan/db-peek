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

// IsRowReturning reports whether sql returns rows and should route through Query
// rather than ExecStmt (SELECT, WITH ... SELECT, VALUES, TABLE, EXPLAIN, or statements with RETURNING).
func IsRowReturning(sql string) bool {
	s := stripLeadingComments(sql)
	if s == "" {
		return false
	}
	if hasReturningClause(sql) {
		return true
	}
	toks := splitSQLWords(s)
	if len(toks) == 0 {
		return false
	}
	switch strings.ToUpper(toks[0]) {
	case "SELECT", "WITH", "VALUES", "TABLE", "EXPLAIN":
		return true
	default:
		return false
	}
}

// IsWriteStatement reports whether sql would modify data or schema
// (INSERT/UPDATE/DELETE/MERGE/REPLACE/DDL), including statements with
// a RETURNING clause or mutating CTEs. Callers gate read-only mode on this.
func IsWriteStatement(sql string) bool {
	s := stripLeadingComments(sql)
	if s == "" {
		return false
	}
	toks := splitSQLWords(s)
	if len(toks) == 0 {
		return false
	}
	first := strings.ToUpper(toks[0])
	switch first {
	case "INSERT", "UPDATE", "DELETE", "MERGE", "REPLACE",
		"CREATE", "DROP", "ALTER", "TRUNCATE", "GRANT", "REVOKE":
		return true
	case "WITH":
		// Detect mutating CTEs: WITH ... INSERT/UPDATE/DELETE
		for i, t := range toks {
			w := strings.ToUpper(t)
			if (w == "DELETE" && i+1 < len(toks) && strings.ToUpper(toks[i+1]) == "FROM") ||
				(w == "INSERT" && i+1 < len(toks) && strings.ToUpper(toks[i+1]) == "INTO") ||
				(w == "UPDATE") {
				return true
			}
		}
		return false
	case "SELECT", "VALUES", "TABLE", "EXPLAIN":
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
	if d.Engine != nil {
		return d.Engine.Explain(q, analyze)
	}
	q = trimStatement(q)
	switch d.Driver {
	case Postgres:
		if analyze {
			return "EXPLAIN (ANALYZE, BUFFERS) " + q
		}
		return "EXPLAIN " + q
	case MySQL, MariaDB:
		if analyze {
			return "EXPLAIN ANALYZE " + q
		}
		return "EXPLAIN " + q
	case ClickHouse, SQLServer:
		return "EXPLAIN " + q
	default:
		return "EXPLAIN QUERY PLAN " + q
	}
}

// Placeholder returns the driver-specific bind marker for the i-th
// (1-based) parameter: Postgres numbers them, MySQL and SQLite use "?".
func (d Driver) Placeholder(i int) string {
	switch d {
	case Postgres:
		return "$" + strconv.Itoa(i)
	case SQLServer:
		return "@p" + strconv.Itoa(i)
	default:
		return "?"
	}
}

// UpdateCell issues a single-parameter UPDATE identified by the full
// primary key. It runs as one autocommit statement, which is atomic on
// every supported engine.
//
// pkVals must carry the driver's own value types, not display strings:
// Postgres has no implicit cast, so binding a string for a bigint key
// fails with "operator does not exist: bigint = text".
func (d *DB) UpdateCell(ctx context.Context, q QualTable, pk []string, pkVals []any, col string, val any) (int64, error) {
	if d.Engine != nil {
		return d.Engine.UpdateCell(ctx, q, pk, pkVals, col, val)
	}
	if len(pk) == 0 || len(pk) != len(pkVals) {
		return 0, fmt.Errorf("update %s: primary key value count does not match the key", q.String())
	}
	qi := d.Driver.QuoteIdent
	var b strings.Builder
	fmt.Fprintf(&b, "UPDATE %s SET %s = %s WHERE ", d.Driver.QuoteQual(q), qi(col), d.Driver.Placeholder(1))
	for i, k := range pk {
		if i > 0 {
			b.WriteString(" AND ")
		}
		fmt.Fprintf(&b, "%s = %s", qi(k), d.Driver.Placeholder(i+2))
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

// RunUserQuery routes one or more editor statements. Row-returning statements
// go through Query (grid), while scripts and write statements run sequentially
// through ExecStmt, reporting total rows affected.
func (d *DB) RunUserQuery(ctx context.Context, sql string) (s *Sample, n int64, err error) {
	if d.Engine != nil {
		return d.Engine.RunUserQuery(ctx, sql)
	}
	stmts := SplitStatements(sql)
	if len(stmts) == 0 {
		return nil, 0, nil
	}
	if len(stmts) == 1 {
		stmt := stmts[0]
		if IsRowReturning(stmt) {
			s, err := d.Query(ctx, stmt)
			return s, -1, err
		}
		n, err = d.ExecStmt(ctx, stmt)
		return nil, n, err
	}

	var totalAffected int64
	for i, stmt := range stmts {
		isLast := i == len(stmts)-1
		if isLast && IsRowReturning(stmt) {
			s, err := d.Query(ctx, stmt)
			if err != nil {
				return nil, -1, fmt.Errorf("statement %d (%s): %w", i+1, truncate(stmt, 30), err)
			}
			return s, -1, nil
		}
		affected, err := d.ExecStmt(ctx, stmt)
		if err != nil {
			return nil, -1, fmt.Errorf("statement %d (%s): %w", i+1, truncate(stmt, 30), err)
		}
		if affected > 0 {
			totalAffected += affected
		}
	}
	return nil, totalAffected, nil
}

// SplitStatements splits a multi-statement SQL script into individual statements
// by semicolons, respecting single quotes, double quotes, backticks, and comments.
func SplitStatements(sql string) []string {
	var stmts []string
	var b strings.Builder
	inSingle := false
	inDouble := false
	inBacktick := false
	inLineComment := false
	inBlockComment := false

	n := len(sql)
	for i := 0; i < n; i++ {
		c := sql[i]
		next := byte(0)
		if i+1 < n {
			next = sql[i+1]
		}

		if inLineComment {
			b.WriteByte(c)
			if c == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			b.WriteByte(c)
			if c == '*' && next == '/' {
				b.WriteByte('/')
				i++
				inBlockComment = false
			}
			continue
		}

		if inSingle {
			b.WriteByte(c)
			if c == '\'' {
				if next == '\'' {
					b.WriteByte('\'')
					i++
				} else {
					inSingle = false
				}
			}
			continue
		}
		if inDouble {
			b.WriteByte(c)
			if c == '"' {
				if next == '"' {
					b.WriteByte('"')
					i++
				} else {
					inDouble = false
				}
			}
			continue
		}
		if inBacktick {
			b.WriteByte(c)
			if c == '`' {
				inBacktick = false
			}
			continue
		}

		if c == '-' && next == '-' {
			inLineComment = true
			b.WriteString("--")
			i++
			continue
		}
		if c == '/' && next == '*' {
			inBlockComment = true
			b.WriteString("/*")
			i++
			continue
		}

		if c == '\'' {
			inSingle = true
			b.WriteByte(c)
			continue
		}
		if c == '"' {
			inDouble = true
			b.WriteByte(c)
			continue
		}
		if c == '`' {
			inBacktick = true
			b.WriteByte(c)
			continue
		}

		if c == ';' {
			stmt := strings.TrimSpace(b.String())
			if stmt != "" {
				stmts = append(stmts, stmt)
			}
			b.Reset()
			continue
		}

		b.WriteByte(c)
	}

	if remaining := strings.TrimSpace(b.String()); remaining != "" {
		stmts = append(stmts, remaining)
	}
	return stmts
}
// ExecStmt runs a non-row-returning statement and reports rows affected.
func (d *DB) ExecStmt(ctx context.Context, sql string) (int64, error) {
	if d.Engine != nil {
		return d.Engine.ExecStmt(ctx, sql)
	}
	res, err := d.SQL.ExecContext(ctx, trimStatement(sql))
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
}
// ApplyReadOnly configures the connection at the session/engine level to refuse
// writes even if client-side validation is bypassed.
func (d *DB) ApplyReadOnly(ctx context.Context) error {
	if d == nil || d.SQL == nil {
		return nil
	}
	switch d.Driver {
	case SQLite:
		_, err := d.SQL.ExecContext(ctx, "PRAGMA query_only = ON")
		return err
	case Postgres:
		_, err := d.SQL.ExecContext(ctx, "SET SESSION CHARACTERISTICS AS TRANSACTION READ ONLY")
		return err
	case MySQL, MariaDB:
		_, err := d.SQL.ExecContext(ctx, "SET SESSION TRANSACTION READ ONLY")
		return err
	}
	return nil
}
