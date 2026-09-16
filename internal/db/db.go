package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// Driver is the detected database engine.
type Driver string

const (
	Postgres Driver = "postgres"
	MySQL    Driver = "mysql"
	SQLite   Driver = "sqlite"
)

func (d Driver) QuoteIdent(s string) string {
	switch d {
	case MySQL:
		return "`" + strings.ReplaceAll(s, "`", "``") + "`"
	default:
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
}

// DB wraps a sql.DB with its detected driver.
type DB struct {
	SQL     *sql.DB
	Driver  Driver
	Display string // human label for title bar (dbname / file), never contains password
}

func redact(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.User != nil {
		u.User = url.User(u.User.Username())
		return u.Redacted()
	}
	return raw
}

// Open dials connStr. Accepted forms:
//
//	postgres://... | postgresql://...
//	mysql://user:pass@host:port/dbname?params
//	[user:pass@tcp(host:port)/dbname] (raw MySQL DSN passthrough via mysql:// fallback)
//	/path/to/file.db | file.db | :memory: | sqlite://path | file:...
func Open(connStr string) (*DB, error) {
	raw := strings.TrimSpace(strings.Trim(connStr, `"'`))
	if raw == "" {
		return nil, fmt.Errorf("empty connection string (pass postgres://, mysql://, or a sqlite file path)")
	}
	lower := strings.ToLower(raw)

	switch {
	case strings.HasPrefix(lower, "postgres://") || strings.HasPrefix(lower, "postgresql://"):
		sqlDB, err := sql.Open("pgx", withSimpleProtocol(raw))
		if err != nil {
			return nil, err
		}
		name := "postgres"
		if u, err := url.Parse(raw); err == nil && u.Path != "" && u.Path != "/" {
			name = strings.TrimPrefix(u.Path, "/")
		}
		return &DB{SQL: sqlDB, Driver: Postgres, Display: name}, nil

	case strings.HasPrefix(lower, "mysql://"):
		dsn, name, err := mysqlURLToDSN(raw)
		if err != nil {
			return nil, err
		}
		sqlDB, err := sql.Open("mysql", dsn)
		if err != nil {
			return nil, err
		}
		return &DB{SQL: sqlDB, Driver: MySQL, Display: name}, nil

	case strings.HasPrefix(lower, "sqlite://") || strings.HasPrefix(lower, "file:") || lower == ":memory:" ||
		strings.HasSuffix(lower, ".db") || strings.HasSuffix(lower, ".db3") ||
		strings.HasSuffix(lower, ".sqlite") || strings.HasSuffix(lower, ".sqlite3"):
		path := strings.TrimPrefix(strings.TrimPrefix(raw, "sqlite://"), "sqlite:")
		if lower != ":memory:" {
			if _, err := os.Stat(path); err != nil {
				return nil, fmt.Errorf("sqlite file %q not found: want postgres://, mysql://, or an existing sqlite path", raw)
			}
		}
		sqlDB, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		return &DB{SQL: sqlDB, Driver: SQLite, Display: displayBase(path)}, nil

	default:
		// Bare path without a sqlite extension (e.g. ./app, /tmp/demo):
		// open only when the file exists — never auto-create on a typo.
		if strings.Contains(raw, "://") {
			return nil, fmt.Errorf("unrecognized connection string %q: want postgres://, mysql://, or a sqlite file path", redact(raw))
		}
		if _, err := os.Stat(raw); err != nil {
			return nil, fmt.Errorf("file %q not found: want postgres://, mysql://, or an existing sqlite path", raw)
		}
		sqlDB, err := sql.Open("sqlite", raw)
		if err != nil {
			return nil, err
		}
		return &DB{SQL: sqlDB, Driver: SQLite, Display: displayBase(raw)}, nil
	}
}

func displayBase(p string) string {
	if p == ":memory:" {
		return ":memory:"
	}
	if i := strings.LastIndexAny(p, "/\\"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// mysqlURLToDSN converts mysql://user:pass@host:port/db?opt=v to
// go-sql-driver DSN user:pass@tcp(host:port)/db?opt=v.
func mysqlURLToDSN(raw string) (dsn, dbName string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", fmt.Errorf("bad mysql url: %w", err)
	}
	user := u.User.Username()
	pass, _ := u.User.Password()
	host := u.Host
	if host == "" {
		host = "127.0.0.1:3306"
	}
	if !strings.Contains(host, ":") {
		host += ":3306"
	}
	dbName = strings.TrimPrefix(u.Path, "/")
	if dbName == "" {
		return "", "", fmt.Errorf("mysql url missing database name (mysql://user:pass@host/dbname)")
	}
	q := u.Query()
	q.Set("parseTime", "true")
	var creds string
	if pass != "" {
		creds = user + ":" + pass
	} else {
		creds = user
	}
	return fmt.Sprintf("%s@tcp(%s)/%s?%s", creds, host, dbName, q.Encode()), dbName, nil
}

func (d *DB) Ping(ctx context.Context) error { return d.SQL.PingContext(ctx) }
func (d *DB) Close() error                   { return d.SQL.Close() }

// Column is a normalized column description across engines.
type Column struct {
	Name     string
	Type     string
	Nullable string // "YES"/"NO" (pg,mysql) or "" for sqlite
	Default  string
	Extra    string // pg: char_maximum_length/precision | mysql: key+extra | sqlite: PK/AUTOINC
}

// Index is a normalized index description.
type Index struct {
	Name    string
	Columns string
	Unique  string // "yes"/"no" or backing ddl snippet
	DDL     string // full definition when available (pg/sqlite)
}

// Sample holds a bounded result set; every cell is pre-formatted for display.
type Sample struct {
	Columns []string
	Rows    [][]string
	Total   int64 // -1 when unknown
}

// ListTables returns user tables ordered by name.
func (d *DB) ListTables(ctx context.Context) ([]string, error) {
	var q string
	switch d.Driver {
	case Postgres:
		q = `SELECT tablename FROM pg_tables WHERE schemaname NOT IN ('pg_catalog','information_schema') ORDER BY tablename`
	case MySQL:
		q = `SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE' ORDER BY table_name`
	default:
		q = `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`
	}
	rows, err := d.SQL.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Columns returns ordered column metadata for one table.
func (d *DB) Columns(ctx context.Context, table string) ([]Column, error) {
	switch d.Driver {
	case Postgres:
		rows, err := d.SQL.QueryContext(ctx, `
SELECT column_name, data_type, is_nullable,
       COALESCE(column_default, ''),
       COALESCE(CASE WHEN character_maximum_length IS NOT NULL
                     THEN '(' || character_maximum_length || ')' ELSE '' END, '')
FROM information_schema.columns
WHERE table_name = $1 ORDER BY ordinal_position`, table)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []Column
		for rows.Next() {
			var c Column
			var typ, extra string
			if err := rows.Scan(&c.Name, &typ, &c.Nullable, &c.Default, &extra); err != nil {
				return nil, err
			}
			c.Type = typ + extra
			out = append(out, c)
		}
		return out, rows.Err()
	case MySQL:
		rows, err := d.SQL.QueryContext(ctx, `
SELECT column_name, column_type, is_nullable, COALESCE(column_default, ''),
       CONCAT(column_key, CASE WHEN extra <> '' THEN ' ' || extra ELSE '' END)
FROM information_schema.columns
WHERE table_schema = DATABASE() AND table_name = ? ORDER BY ordinal_position`, table)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []Column
		for rows.Next() {
			var c Column
			var def sql.NullString
			if err := rows.Scan(&c.Name, &c.Type, &c.Nullable, &def, &c.Extra); err != nil {
				return nil, err
			}
			if def.Valid {
				c.Default = def.String
			}
			out = append(out, c)
		}
		return out, rows.Err()
	default:
		q := fmt.Sprintf(`PRAGMA table_info(%s)`, d.Driver.QuoteIdent(table))
		rows, err := d.SQL.QueryContext(ctx, q)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []Column
		for rows.Next() {
			var cid int
			var name, typ string
			var notnull, pk int
			var dflt sql.NullString
			if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
				return nil, err
			}
			c := Column{Name: name, Type: typ}
			if notnull == 0 && pk == 0 {
				c.Nullable = "YES"
			} else {
				c.Nullable = "NO"
			}
			if dflt.Valid {
				c.Default = dflt.String
			}
			if pk > 0 {
				c.Extra = fmt.Sprintf("PK(%d)", pk)
			}
			out = append(out, c)
		}
		return out, rows.Err()
	}
}

// Indexes returns index metadata for one table.
func (d *DB) Indexes(ctx context.Context, table string) ([]Index, error) {
	switch d.Driver {
	case Postgres:
		rows, err := d.SQL.QueryContext(ctx,
			`SELECT indexname, indexdef FROM pg_indexes WHERE tablename = $1 ORDER BY indexname`, table)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []Index
		for rows.Next() {
			var name, def string
			if err := rows.Scan(&name, &def); err != nil {
				return nil, err
			}
			uniq := "no"
			if strings.Contains(strings.ToUpper(def), "UNIQUE") {
				uniq = "yes"
			}
			out = append(out, Index{Name: name, Unique: uniq, Columns: shortIndexCols(def), DDL: def})
		}
		return out, rows.Err()
	case MySQL:
		q := fmt.Sprintf(`SHOW INDEX FROM %s`, d.Driver.QuoteIdent(table))
		rows, err := d.SQL.QueryContext(ctx, q)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		type key struct {
			name   string
			unique string
			cols   []string
		}
		order := []string{}
		byName := map[string]*key{}
		for rows.Next() {
			var (
				keyName, colName sql.NullString
			)
			// SHOW INDEX has 13-15 cols depending on version; scan generically.
			cols, err := rows.Columns()
			if err != nil {
				return nil, err
			}
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				return nil, err
			}
			m := map[string]string{}
			for i, c := range cols {
				m[strings.ToLower(c)] = b2s(vals[i])
			}
			keyName.String, keyName.Valid = m["key_name"], m["key_name"] != ""
			colName.String, colName.Valid = m["column_name"], true
			nu := m["non_unique"]
			k, ok := byName[keyName.String]
			if !ok {
				k = &key{name: keyName.String, unique: "yes"}
				if nu == "1" {
					k.unique = "no"
				}
				byName[keyName.String] = k
				order = append(order, keyName.String)
			}
			if colName.String != "" {
				k.cols = append(k.cols, colName.String)
			}
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		out := make([]Index, 0, len(order))
		for _, n := range order {
			k := byName[n]
			out = append(out, Index{Name: k.name, Unique: k.unique, Columns: strings.Join(k.cols, ", ")})
		}
		return out, nil
	default:
		// PRAGMA index_list(table) reports the authoritative unique flag
		// (auto-index DDL is NULL in sqlite_master, so DDL sniffing misses it).
		uniqByName := map[string]string{}
		if lr, err := d.SQL.QueryContext(ctx, fmt.Sprintf(`PRAGMA index_list(%s)`, d.Driver.QuoteIdent(table))); err == nil {
			defer lr.Close()
			for lr.Next() {
				var seq, unique int
				var name, origin string
				var partial int
				if err := lr.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
					break
				}
				if unique == 1 {
					uniqByName[name] = "yes"
				} else {
					uniqByName[name] = "no"
				}
			}
		}
		rows, err := d.SQL.QueryContext(ctx,
			`SELECT name, sql FROM sqlite_master WHERE type = 'index' AND tbl_name = ? ORDER BY name`, table)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []Index
		for rows.Next() {
			var name string
			var ddl sql.NullString
			if err := rows.Scan(&name, &ddl); err != nil {
				return nil, err
			}
			cols, err := d.sqliteIndexColumns(ctx, name)
			if err != nil {
				cols = ""
			}
			uniq, ok := uniqByName[name]
			if !ok { // fallback for exotic builds: sniff the DDL text
				uniq = "no"
				if ddl.Valid && strings.Contains(strings.ToUpper(ddl.String), "UNIQUE") {
					uniq = "yes"
				}
			}
			out = append(out, Index{Name: name, Columns: cols, Unique: uniq, DDL: ddl.String})
		}
		return out, rows.Err()
	}
}

func (d *DB) sqliteIndexColumns(ctx context.Context, index string) (string, error) {
	r, err := d.SQL.QueryContext(ctx, fmt.Sprintf(`PRAGMA index_info(%s)`, d.Driver.QuoteIdent(index)))
	if err != nil {
		return "", err
	}
	defer r.Close()
	var out []string
	for r.Next() {
		var seqno, cid int
		var name string
		if err := r.Scan(&seqno, &cid, &name); err != nil {
			return "", err
		}
		out = append(out, name)
	}
	if err := r.Err(); err != nil {
		return "", err
	}
	return strings.Join(out, ", "), nil
}

// Count returns the exact row count for one table.
func (d *DB) Count(ctx context.Context, table string) (int64, error) {
	var n int64
	q := fmt.Sprintf(`SELECT COUNT(*) FROM %s`, d.Driver.QuoteIdent(table))
	if err := d.SQL.QueryRowContext(ctx, q).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// SampleRows returns up to limit recent rows (highest ROWID/ctid order is
// engine-defined; we use no ORDER BY to keep it a cheap heap scan preview).
func (d *DB) SampleRows(ctx context.Context, table string, limit int) (*Sample, error) {
	return d.PageRows(ctx, table, limit, 0)
}

// PageRows returns one page of rows: up to limit rows skipping offset.
// LIMIT/OFFSET syntax is shared by postgres, mysql, and sqlite.
func (d *DB) PageRows(ctx context.Context, table string, limit, offset int) (*Sample, error) {
	if limit <= 0 || limit > 1000 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}
	q := fmt.Sprintf(`SELECT * FROM %s LIMIT %d OFFSET %d`, d.Driver.QuoteIdent(table), limit, offset)
	return d.queryToSample(ctx, q)
}

// Query runs an arbitrary SELECT and captures up to 200 rows.
func (d *DB) Query(ctx context.Context, q string) (*Sample, error) {
	return d.queryToSample(ctx, q)
}

func (d *DB) queryToSample(ctx context.Context, q string) (*Sample, error) {
	rows, err := d.SQL.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	s := &Sample{Columns: cols, Total: -1}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		for i := range ptrs {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		rec := make([]string, len(cols))
		for i, v := range vals {
			rec[i] = FormatCell(v)
		}
		s.Rows = append(s.Rows, rec)
		if len(s.Rows) >= 200 {
			break
		}
	}
	return s, rows.Err()
}

func b2s(v any) string { return FormatCell(v) }

// FormatCell renders one driver value for terminal display.
func FormatCell(v any) string {
	switch t := v.(type) {
	case nil:
		return "NULL"
	case string:
		return truncate(t, 60)
	case []byte:
		if isMostlyText(t) {
			return truncate(string(t), 60)
		}
		return fmt.Sprintf("<binary %dB>", len(t))
	case int64:
		return fmt.Sprintf("%d", t)
	case int32:
		return fmt.Sprintf("%d", t)
	case int:
		return fmt.Sprintf("%d", t)
	case float64:
		return fmt.Sprintf("%v", t)
	case float32:
		return fmt.Sprintf("%v", t)
	case bool:
		return fmt.Sprintf("%v", t)
	default:
		s := fmt.Sprintf("%v", t)
		return truncate(s, 60)
	}
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\t", " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-3]) + "..."
}

func isMostlyText(b []byte) bool {
	if len(b) == 0 {
		return true
	}
	n := len(b)
	if n > 512 {
		n = 512
	}
	printable := 0
	for _, c := range b[:n] {
		if c >= 32 && c < 127 || c == '\n' || c == '\r' || c == '\t' {
			printable++
		}
	}
	return printable*10 >= n*8
}

func shortIndexCols(def string) string {
	// Extract "(a, b)" tail of a CREATE INDEX statement for the Columns cell.
	i := strings.LastIndex(def, "(")
	j := strings.LastIndex(def, ")")
	if i >= 0 && j > i {
		return def[i+1 : j]
	}
	return ""
}
