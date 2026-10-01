package db

import (
	"context"
	"fmt"
)

type TableRef struct {
	Name   string
	IsView bool
}

// SystemSchemas are Postgres-internal or Supabase-managed schemas that
// clutter the explorer (TablePlus/DBeaver hide these by default and show
// public). Exported so the TUI and CLI can share the same filter.
var SystemSchemas = []string{
	"pg_catalog", "information_schema",
	// Supabase-managed services (auth, storage, realtime, etc.).
	"auth", "storage", "realtime", "extensions",
	"graphql", "graphql_public",
	"supabase_functions", "supabase_migrations",
	"vault", "pgsodium", "pgsodium_masks", "net",
	// Common extension schemas users rarely want in the explorer.
	"postgis", "_postgis", "tiger", "tiger_data", "topology",
	"cron", "_realtime", "_supabase",
}

// IsSystemSchema reports whether s is a Postgres-internal (pg_*) or a
// known Supabase/extension-managed schema. User schemas — including
// "public" and any custom schema — return false.
func IsSystemSchema(s string) bool {
	if len(s) >= 3 && (s == "pg_catalog" || len(s) > 3 && s[:3] == "pg_") {
		return true
	}
	// pg_temp_* / pg_toast* backends per-connection.
	if len(s) >= 7 && s[:7] == "pg_temp" {
		return true
	}
	if len(s) >= 8 && s[:8] == "pg_toast" {
		return true
	}
	for _, sys := range SystemSchemas {
		if s == sys {
			return true
		}
	}
	return false
}

func (d *DB) ListSchemas(ctx context.Context) ([]string, error) {
	switch d.Driver {
	case Postgres:
		rows, err := d.SQL.QueryContext(ctx, `SELECT schema_name FROM information_schema.schemata
WHERE schema_name NOT IN ('pg_catalog','information_schema','auth','storage','realtime','extensions','graphql','graphql_public','supabase_functions','supabase_migrations','vault','pgsodium','pgsodium_masks','net','postgis','_postgis','tiger','tiger_data','topology','cron','_realtime','_supabase')
AND schema_name NOT LIKE 'pg\_%' ESCAPE '\'
AND schema_name NOT LIKE 'pg_temp_%'
AND schema_name NOT LIKE 'pg_toast%'
ORDER BY CASE WHEN schema_name='public' THEN 0 ELSE 1 END, schema_name`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				return nil, err
			}
			out = append(out, s)
		}
		return out, rows.Err()
	case MySQL:
		var name string
		if err := d.SQL.QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&name); err != nil {
			return nil, err
		}
		if name == "" {
			name = d.Display
		}
		return []string{name}, nil
	default:
		return []string{"main"}, nil
	}
}

func (d *DB) ListTablesInSchema(ctx context.Context, schema string) ([]TableRef, error) {
	switch d.Driver {
	case Postgres:
		rows, err := d.SQL.QueryContext(ctx, `
SELECT tablename AS n, false AS v FROM pg_tables WHERE schemaname=$1
UNION ALL SELECT viewname AS n, true AS v FROM pg_views WHERE schemaname=$1
ORDER BY 1`, schema)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []TableRef
		for rows.Next() {
			var r TableRef
			if err := rows.Scan(&r.Name, &r.IsView); err != nil {
				return nil, err
			}
			out = append(out, r)
		}
		return out, rows.Err()
	case MySQL:
		rows, err := d.SQL.QueryContext(ctx, `SELECT table_name, table_type FROM information_schema.tables WHERE table_schema = DATABASE() ORDER BY 1`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []TableRef
		for rows.Next() {
			var name, typ string
			if err := rows.Scan(&name, &typ); err != nil {
				return nil, err
			}
			out = append(out, TableRef{Name: name, IsView: typ == "VIEW"})
		}
		return out, rows.Err()
	default:
		rows, err := d.SQL.QueryContext(ctx, `SELECT name, type FROM sqlite_master WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY name`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []TableRef
		for rows.Next() {
			var name, typ string
			if err := rows.Scan(&name, &typ); err != nil {
				return nil, err
			}
			out = append(out, TableRef{Name: name, IsView: typ == "view"})
		}
		return out, rows.Err()
	}
}

func (d *DB) ApproxCount(ctx context.Context, q QualTable) (int64, error) {
	if d.Driver == Postgres {
		var n int64
		err := d.SQL.QueryRowContext(ctx, `SELECT reltuples::bigint FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE c.relname = $1 AND n.nspname = $2`, q.Name, q.Schema).Scan(&n)
		if err == nil && n >= 0 {
			return n, nil
		}
	}
	var n int64
	qq := fmt.Sprintf(`SELECT COUNT(*) FROM %s`, d.Driver.QuoteQual(q))
	if err := d.SQL.QueryRowContext(ctx, qq).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (d *DB) ResolveTable(ctx context.Context, name string) (QualTable, error) {
	q, err := ParseQualTable(name)
	if err != nil {
		return QualTable{}, err
	}
	if d.Driver != Postgres {
		// DEVIATION from brief verbatim (see task-3 report): the verbatim
		// passthrough returned nil for missing sqlite/mysql tables, but
		// TestResolveTableSQLitePassthrough demands a not-found error.
		var existsQ string
		switch d.Driver {
		case MySQL:
			existsQ = `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?)`
		default:
			existsQ = `SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type IN ('table','view') AND name = ?)`
		}
		var ok bool
		if err := d.SQL.QueryRowContext(ctx, existsQ, q.Name).Scan(&ok); err != nil {
			return QualTable{}, err
		}
		if !ok {
			if q.Schema == "" {
				return QualTable{}, fmt.Errorf("table %q not found", q.Name)
			}
			return QualTable{}, fmt.Errorf("table %q not found", q.String())
		}
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
