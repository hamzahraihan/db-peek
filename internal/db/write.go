package db

import (
	"context"
	"net/url"
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
