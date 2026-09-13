package db

import (
	"context"
	"fmt"
)

type TableRef struct {
	Name   string
	IsView bool
}

func (d *DB) ListSchemas(ctx context.Context) ([]string, error) {
	switch d.Driver {
	case Postgres:
		rows, err := d.SQL.QueryContext(ctx, `SELECT schema_name FROM information_schema.schemata WHERE schema_name NOT IN ('pg_catalog','information_schema') ORDER BY 1`)
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

func (d *DB) ApproxCount(ctx context.Context, schema, table string) (int64, error) {
	if d.Driver == Postgres {
		var n int64
		err := d.SQL.QueryRowContext(ctx, `SELECT reltuples::bigint FROM pg_class WHERE relname=$1`, table).Scan(&n)
		if err == nil && n >= 0 {
			return n, nil
		}
	}
	var n int64
	q := fmt.Sprintf(`SELECT COUNT(*) FROM %s`, d.Driver.QuoteIdent(table))
	if err := d.SQL.QueryRowContext(ctx, q).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
