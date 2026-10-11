package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "github.com/ClickHouse/clickhouse-go/v2"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/microsoft/go-mssqldb"
	_ "modernc.org/sqlite"
)

// Driver is the detected database engine.
type Driver string

const (
	Postgres   Driver = "postgres"
	MySQL      Driver = "mysql"
	SQLite     Driver = "sqlite"
	MariaDB    Driver = "mariadb"
	SQLServer  Driver = "sqlserver"
	ClickHouse Driver = "clickhouse"
	Redis      Driver = "redis"
	MongoDB    Driver = "mongodb"
)

func (d Driver) QuoteIdent(s string) string {
	switch d {
	case MySQL, MariaDB, ClickHouse:
		return "`" + strings.ReplaceAll(s, "`", "``") + "`"
	case SQLServer:
		return "[" + strings.ReplaceAll(s, "]", "]]") + "]"
	default:
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
}

// DB wraps a sql.DB with its detected driver.
type DB struct {
	SQL     *sql.DB
	Driver  Driver
	Display string // human label for title bar (dbname / file), never contains password
	Engine  Engine // non-nil for NoSQL backends (Redis, MongoDB)
}

func redact(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.User != nil {
		u.User = url.User(u.User.Username())
		return u.Redacted()
	}
	return raw
}
func configurePool(sqlDB *sql.DB) {
	if sqlDB != nil {
		sqlDB.SetMaxOpenConns(10)
		sqlDB.SetMaxIdleConns(2)
		sqlDB.SetConnMaxLifetime(15 * time.Minute)
	}
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
		configurePool(sqlDB)
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
		configurePool(sqlDB)
		return &DB{SQL: sqlDB, Driver: MySQL, Display: name}, nil

	case strings.HasPrefix(lower, "mariadb://"):
		dsn, name, err := mysqlURLToDSN(raw)
		if err != nil {
			return nil, err
		}
		sqlDB, err := sql.Open("mysql", dsn)
		if err != nil {
			return nil, err
		}
		configurePool(sqlDB)
		return &DB{SQL: sqlDB, Driver: MariaDB, Display: name}, nil

	case strings.HasPrefix(lower, "sqlserver://") || strings.HasPrefix(lower, "mssql://"):
		sqlDB, err := sql.Open("sqlserver", raw)
		if err != nil {
			return nil, err
		}
		name := "sqlserver"
		if u, err := url.Parse(raw); err == nil {
			if dbParam := u.Query().Get("database"); dbParam != "" {
				name = dbParam
			} else if u.Path != "" && u.Path != "/" {
				name = strings.TrimPrefix(u.Path, "/")
			}
		}
		configurePool(sqlDB)
		return &DB{SQL: sqlDB, Driver: SQLServer, Display: name}, nil

	case strings.HasPrefix(lower, "clickhouse://") || strings.HasPrefix(lower, "clickhouse:"):
		sqlDB, err := sql.Open("clickhouse", raw)
		if err != nil {
			return nil, err
		}
		name := "default"
		if u, err := url.Parse(raw); err == nil && u.Path != "" && u.Path != "/" {
			name = strings.TrimPrefix(u.Path, "/")
		}
		configurePool(sqlDB)
		return &DB{SQL: sqlDB, Driver: ClickHouse, Display: name}, nil

	case strings.HasPrefix(lower, "redis://") || strings.HasPrefix(lower, "rediss://"):
		eng, name, err := openRedis(raw)
		if err != nil {
			return nil, err
		}
		return &DB{Driver: Redis, Display: name, Engine: eng}, nil

	case strings.HasPrefix(lower, "mongodb://") || strings.HasPrefix(lower, "mongodb+srv://"):
		eng, name, err := openMongo(raw)
		if err != nil {
			return nil, err
		}
		return &DB{Driver: MongoDB, Display: name, Engine: eng}, nil
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
		configurePool(sqlDB)
		return &DB{SQL: sqlDB, Driver: SQLite, Display: displayBase(path)}, nil

	default:
		// Bare path without a sqlite extension (e.g. ./app, /tmp/demo):
		// open only when the file exists — never auto-create on a typo.
		if strings.Contains(raw, "://") {
			return nil, fmt.Errorf("unrecognized connection string %q: want postgres://, mysql://, mariadb://, sqlserver://, clickhouse://, redis://, mongodb://, or a sqlite file path", redact(raw))
		}
		if _, err := os.Stat(raw); err != nil {
			return nil, fmt.Errorf("file %q not found: want postgres://, mysql://, or an existing sqlite path", raw)
		}
		sqlDB, err := sql.Open("sqlite", raw)
		if err != nil {
			return nil, err
		}
		configurePool(sqlDB)
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
	lower := strings.ToLower(raw)
	if !strings.HasPrefix(lower, "mysql://") && !strings.HasPrefix(lower, "mariadb://") {
		return raw, "", nil
	}
	rawNorm := raw
	if strings.HasPrefix(lower, "mariadb://") {
		rawNorm = "mysql://" + raw[len("mariadb://"):]
	}
	u, err := url.Parse(rawNorm)
	if err != nil {
		return "", "", fmt.Errorf("bad connection url: %w", err)
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

func (d *DB) Ping(ctx context.Context) error {
	if d.Engine != nil {
		return d.Engine.Ping(ctx)
	}
	return d.SQL.PingContext(ctx)
}

func (d *DB) Close() error {
	if d.Engine != nil {
		return d.Engine.Close()
	}
	return d.SQL.Close()
}

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
	// Raw holds the untruncated driver values, parallel to Rows; nil is
	// SQL NULL. Rows is display text (truncated to 60 chars), so export
	// and cell editing must read this instead or lose data.
Raw  [][]any
	Total int64 // -1 when unknown
}

// ListTables returns user tables ordered by name, skipping
// Postgres-internal (pg_*) and Supabase-managed schemas just like
// ListSchemas — so --list matches the explorer (public-first UX).
func (d *DB) ListTables(ctx context.Context) ([]string, error) {
	if d.Engine != nil {
		refs, err := d.Engine.ListTablesInSchema(ctx, "")
		if err != nil {
			return nil, err
		}
		out := make([]string, len(refs))
		for i, r := range refs {
			out[i] = r.Name
		}
		return out, nil
	}
	var q string
	switch d.Driver {
	case Postgres:
		q = `SELECT tablename FROM pg_tables WHERE schemaname NOT IN ('pg_catalog','information_schema','auth','storage','realtime','extensions','graphql','graphql_public','supabase_functions','supabase_migrations','vault','pgsodium','pgsodium_masks','net','postgis','_postgis','tiger','tiger_data','topology','cron','_realtime','_supabase') AND schemaname NOT LIKE 'pg\_%' ESCAPE '\' AND schemaname NOT LIKE 'pg_temp_%' AND schemaname NOT LIKE 'pg_toast%' ORDER BY tablename`
	case MySQL, MariaDB:
		q = `SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE' ORDER BY table_name`
	case SQLServer:
		q = `SELECT TABLE_NAME FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_TYPE = 'BASE TABLE' AND TABLE_SCHEMA NOT IN ('sys', 'INFORMATION_SCHEMA', 'guest') ORDER BY TABLE_NAME`
	case ClickHouse:
		q = `SELECT name FROM system.tables WHERE database = currentDatabase() AND is_temporary = 0 ORDER BY name`
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
func (d *DB) Columns(ctx context.Context, q QualTable) ([]Column, error) {
	if d.Engine != nil {
		return d.Engine.Columns(ctx, q)
	}
	switch d.Driver {
	case Postgres:
		rows, err := d.SQL.QueryContext(ctx, `
SELECT column_name, data_type, is_nullable,
       COALESCE(column_default, ''),
       COALESCE(CASE WHEN character_maximum_length IS NOT NULL
                     THEN '(' || character_maximum_length || ')' ELSE '' END, '')
FROM information_schema.columns
WHERE table_name = $1 AND table_schema = $2 ORDER BY ordinal_position`, q.Name, q.Schema)
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
	case MySQL, MariaDB:
		schemaClause := "table_schema = DATABASE()"
		var args []any
		if q.Schema != "" {
			schemaClause = "table_schema = ?"
			args = append(args, q.Schema)
		}
		args = append(args, q.Name)
		rows, err := d.SQL.QueryContext(ctx, fmt.Sprintf(`
SELECT column_name, column_type, is_nullable, COALESCE(column_default, ''),
       CONCAT(column_key, CASE WHEN extra <> '' THEN ' ' || extra ELSE '' END)
FROM information_schema.columns
WHERE %s AND table_name = ? ORDER BY ordinal_position`, schemaClause), args...)
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
	case SQLServer:
		schemaClause := "(c.TABLE_SCHEMA = @p2 OR @p2 = '')"
		schemaVal := q.Schema
		if schemaVal == "" {
			schemaVal = "dbo"
		}
		rows, err := d.SQL.QueryContext(ctx, `
SELECT c.COLUMN_NAME,
       c.DATA_TYPE + COALESCE('(' + CASE WHEN c.CHARACTER_MAXIMUM_LENGTH = -1 THEN 'max' ELSE CAST(c.CHARACTER_MAXIMUM_LENGTH AS VARCHAR) END + ')', ''),
       c.IS_NULLABLE,
       COALESCE(c.COLUMN_DEFAULT, '')
FROM INFORMATION_SCHEMA.COLUMNS c
WHERE c.TABLE_NAME = @p1 AND `+schemaClause+`
ORDER BY c.ORDINAL_POSITION`, q.Name, schemaVal)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []Column
		for rows.Next() {
			var c Column
			var def sql.NullString
			if err := rows.Scan(&c.Name, &c.Type, &c.Nullable, &def); err != nil {
				return nil, err
			}
			if def.Valid {
				c.Default = def.String
			}
			out = append(out, c)
		}
		return out, rows.Err()
	case ClickHouse:
		dbClause := "database = currentDatabase()"
		var args []any
		if q.Schema != "" {
			dbClause = "database = ?"
			args = append(args, q.Schema)
		}
		args = append(args, q.Name)
		rows, err := d.SQL.QueryContext(ctx, fmt.Sprintf(`
SELECT name, type, is_in_primary_key, COALESCE(default_expression, '')
FROM system.columns
WHERE %s AND table = ?
ORDER BY position`, dbClause), args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []Column
		for rows.Next() {
			var c Column
			var isPK uint8
			var def string
			if err := rows.Scan(&c.Name, &c.Type, &isPK, &def); err != nil {
				return nil, err
			}
			c.Nullable = "YES"
			if strings.HasPrefix(c.Type, "Nullable(") {
				c.Nullable = "YES"
			}
			if isPK == 1 {
				c.Extra = "PK"
				c.Nullable = "NO"
			}
			c.Default = def
			out = append(out, c)
		}
		return out, rows.Err()
	default:
		// PRAGMA takes a bare table ident, not a qualified "s"."t"
		// (syntax error); sqlite ignores Schema, quoting only the name.
		qq := fmt.Sprintf(`PRAGMA table_info(%s)`, d.Driver.QuoteIdent(q.Name))
		rows, err := d.SQL.QueryContext(ctx, qq)
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

// PrimaryKey returns the table's primary-key columns in key order, or
// nil when it has none. Cell editing needs the key to address exactly
// one row, so "no key" is a normal answer, not an error.
func (d *DB) PrimaryKey(ctx context.Context, q QualTable) ([]string, error) {
	if d.Engine != nil {
		return d.Engine.PrimaryKey(ctx, q)
	}
	var (
		rows *sql.Rows
		err  error
	)
	switch d.Driver {
	case Postgres:
		rows, err = d.SQL.QueryContext(ctx, `
SELECT a.attname
FROM pg_index i
JOIN pg_class c ON c.oid = i.indrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
WHERE c.relname = $1 AND n.nspname = $2 AND i.indisprimary
ORDER BY array_position(i.indkey, a.attnum)`, q.Name, q.Schema)
	case MySQL, MariaDB:
		schemaClause := "TABLE_SCHEMA = DATABASE()"
		var args []any
		if q.Schema != "" {
			schemaClause = "TABLE_SCHEMA = ?"
			args = append(args, q.Schema)
		}
		args = append(args, q.Name)
		rows, err = d.SQL.QueryContext(ctx, fmt.Sprintf(`
SELECT COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE
WHERE %s AND TABLE_NAME = ? AND CONSTRAINT_NAME = 'PRIMARY'
ORDER BY ORDINAL_POSITION`, schemaClause), args...)
	case SQLServer:
		schemaVal := q.Schema
		if schemaVal == "" {
			schemaVal = "dbo"
		}
		rows, err = d.SQL.QueryContext(ctx, `
SELECT kcu.COLUMN_NAME
FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc
JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE kcu
  ON tc.CONSTRAINT_NAME = kcu.CONSTRAINT_NAME AND tc.TABLE_SCHEMA = kcu.TABLE_SCHEMA
WHERE tc.CONSTRAINT_TYPE = 'PRIMARY KEY' AND tc.TABLE_NAME = @p1 AND tc.TABLE_SCHEMA = @p2
ORDER BY kcu.ORDINAL_POSITION`, q.Name, schemaVal)
	case ClickHouse:
		cols, cerr := d.Columns(ctx, q)
		if cerr != nil {
			return nil, cerr
		}
		var out []string
		for _, c := range cols {
			if c.Extra == "PK" {
				out = append(out, c.Name)
			}
		}
		return out, nil
	default:
		// PRAGMA table_info already encodes the key as Extra = "PK(n)";
		// reuse it rather than issuing a second statement.
		cols, cerr := d.Columns(ctx, q)
		if cerr != nil {
			return nil, cerr
		}
		type pkCol struct {
			name string
			pos  int
		}
		var keys []pkCol
		for _, c := range cols {
			if !strings.HasPrefix(c.Extra, "PK(") {
				continue
			}
			pos, perr := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(c.Extra, "PK("), ")"))
			if perr != nil {
				continue
			}
			keys = append(keys, pkCol{name: c.Name, pos: pos})
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i].pos < keys[j].pos })
		out := make([]string, len(keys))
		for i, k := range keys {
			out[i] = k.name
		}
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// Indexes returns index metadata for one table.
func (d *DB) Indexes(ctx context.Context, q QualTable) ([]Index, error) {
	if d.Engine != nil {
		return d.Engine.Indexes(ctx, q)
	}
	switch d.Driver {
	case Postgres:
		rows, err := d.SQL.QueryContext(ctx,
			`SELECT indexname, indexdef FROM pg_indexes WHERE tablename = $1 AND schemaname = $2 ORDER BY indexname`, q.Name, q.Schema)
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
	case MySQL, MariaDB:
		q := fmt.Sprintf(`SHOW INDEX FROM %s`, d.Driver.QuoteQual(q))
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
	case SQLServer:
		schemaVal := q.Schema
		if schemaVal == "" {
			schemaVal = "dbo"
		}
		rows, err := d.SQL.QueryContext(ctx, `
SELECT i.name, i.is_unique, col.name
FROM sys.indexes i
JOIN sys.tables t ON i.object_id = t.object_id
JOIN sys.schemas s ON t.schema_id = s.schema_id
JOIN sys.index_columns ic ON i.object_id = ic.object_id AND i.index_id = ic.index_id
JOIN sys.columns col ON ic.object_id = col.object_id AND ic.column_id = col.column_id
WHERE t.name = @p1 AND s.name = @p2
ORDER BY i.name, ic.key_ordinal`, q.Name, schemaVal)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		type key struct {
			name   string
			unique string
			cols   []string
		}
		var order []string
		byName := map[string]*key{}
		for rows.Next() {
			var name, col string
			var isUnique bool
			if err := rows.Scan(&name, &isUnique, &col); err != nil {
				return nil, err
			}
			k, ok := byName[name]
			if !ok {
				u := "no"
				if isUnique {
					u = "yes"
				}
				k = &key{name: name, unique: u}
				byName[name] = k
				order = append(order, name)
			}
			k.cols = append(k.cols, col)
		}
		out := make([]Index, 0, len(order))
		for _, n := range order {
			k := byName[n]
			out = append(out, Index{Name: k.name, Unique: k.unique, Columns: strings.Join(k.cols, ", ")})
		}
		return out, nil
	case ClickHouse:
		dbClause := "database = currentDatabase()"
		var args []any
		if q.Schema != "" {
			dbClause = "database = ?"
			args = append(args, q.Schema)
		}
		args = append(args, q.Name)
		rows, err := d.SQL.QueryContext(ctx, fmt.Sprintf(`
SELECT name, type, expr
FROM system.data_skipping_indices
WHERE %s AND table = ?
ORDER BY name`, dbClause), args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []Index
		for rows.Next() {
			var name, typ, expr string
			if err := rows.Scan(&name, &typ, &expr); err != nil {
				return nil, err
			}
			out = append(out, Index{Name: name, Unique: "no", Columns: expr + " (" + typ + ")"})
		}
		return out, nil
	default:
		// PRAGMA index_list(table) reports the authoritative unique flag
		// (auto-index DDL is NULL in sqlite_master, so DDL sniffing misses it).
		uniqByName := map[string]string{}
		// PRAGMA takes a bare table ident (see Columns); sqlite ignores Schema.
		if lr, err := d.SQL.QueryContext(ctx, fmt.Sprintf(`PRAGMA index_list(%s)`, d.Driver.QuoteIdent(q.Name))); err == nil {
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
			`SELECT name, sql FROM sqlite_master WHERE type = 'index' AND tbl_name = ? ORDER BY name`, q.Name)
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
func (d *DB) Count(ctx context.Context, q QualTable) (int64, error) {
	if d.Engine != nil {
		return d.Engine.Count(ctx, q)
	}
	var n int64
	qq := fmt.Sprintf(`SELECT COUNT(*) FROM %s`, d.Driver.QuoteQual(q))
	if err := d.SQL.QueryRowContext(ctx, qq).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// SampleRows returns up to limit recent rows (highest ROWID/ctid order is
// engine-defined; we use no ORDER BY to keep it a cheap heap scan preview).
func (d *DB) SampleRows(ctx context.Context, q QualTable, limit int) (*Sample, error) {
	return d.PageRows(ctx, q, limit, 0)
}

// PageRows returns one page of rows: up to limit rows skipping offset.
// LIMIT/OFFSET syntax is shared by postgres, mysql, and sqlite.
func (d *DB) PageRows(ctx context.Context, q QualTable, limit, offset int) (*Sample, error) {
	if d.Engine != nil {
		return d.Engine.PageRows(ctx, q, limit, offset)
	}
	if limit <= 0 || limit > 1000 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}
	var qq string
	if d.Driver == SQLServer {
		qq = fmt.Sprintf(`SELECT * FROM %s ORDER BY (SELECT NULL) OFFSET %d ROWS FETCH NEXT %d ROWS ONLY`, d.Driver.QuoteQual(q), offset, limit)
	} else {
		qq = fmt.Sprintf(`SELECT * FROM %s LIMIT %d OFFSET %d`, d.Driver.QuoteQual(q), limit, offset)
	}
	return d.queryToSample(ctx, qq)
}
// Query runs an arbitrary SELECT and captures up to 200 rows.
func (d *DB) Query(ctx context.Context, q string) (*Sample, error) {
	if d.Engine != nil {
		return d.Engine.Query(ctx, q)
	}
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
		raw := make([]any, len(cols))
		for i, v := range vals {
			rec[i] = FormatCell(v)
			// Clone []byte: the driver may recycle the backing array
			// once Scan returns, and Raw outlives this loop.
			if b, ok := v.([]byte); ok {
				raw[i] = slices.Clone(b)
				continue
			}
			raw[i] = v
		}
		s.Rows = append(s.Rows, rec)
		s.Raw = append(s.Raw, raw)
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
