package db

import (
	"context"
	"fmt"
)

type ForeignKey struct {
	FromTable  string
	FromColumn string
	ToTable    string
	ToColumn   string
}

func (d *DB) ForeignKeys(ctx context.Context, table string) ([]ForeignKey, error) {
	switch d.Driver {
	case Postgres:
		rows, err := d.SQL.QueryContext(ctx, `
SELECT c.relname, a.attname, c2.relname, a2.attname
FROM pg_constraint o
JOIN pg_class c ON c.oid = o.conrelid
JOIN pg_class c2 ON c2.oid = o.confrelid
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = ANY(o.conkey)
JOIN pg_attribute a2 ON a2.attrelid = c2.oid AND a2.attnum = ANY(o.confkey)
WHERE o.contype = 'f' AND (c.relname = $1 OR c2.relname = $1)
ORDER BY 1, 2`, table)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []ForeignKey
		for rows.Next() {
			var fk ForeignKey
			if err := rows.Scan(&fk.FromTable, &fk.FromColumn, &fk.ToTable, &fk.ToColumn); err != nil {
				return nil, err
			}
			out = append(out, fk)
		}
		return out, rows.Err()
	case MySQL:
		rows, err := d.SQL.QueryContext(ctx, `
SELECT TABLE_NAME, COLUMN_NAME, REFERENCED_TABLE_NAME, REFERENCED_COLUMN_NAME
FROM information_schema.KEY_COLUMN_USAGE
WHERE TABLE_SCHEMA = DATABASE() AND REFERENCED_TABLE_NAME IS NOT NULL
AND (TABLE_NAME = ? OR REFERENCED_TABLE_NAME = ?) ORDER BY 1, 2`, table, table)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []ForeignKey
		for rows.Next() {
			var fk ForeignKey
			if err := rows.Scan(&fk.FromTable, &fk.FromColumn, &fk.ToTable, &fk.ToColumn); err != nil {
				return nil, err
			}
			out = append(out, fk)
		}
		return out, rows.Err()
	default:
		var out []ForeignKey
		out = append(out, d.sqliteOutgoing(ctx, table)...)
		// Incoming: scan all user tables' foreign_key_list for refs to table.
		names, err := d.ListTables(ctx)
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			if n == table {
				continue
			}
			q := fmt.Sprintf(`PRAGMA foreign_key_list(%s)`, d.Driver.QuoteIdent(n))
			r, err := d.SQL.QueryContext(ctx, q)
			if err != nil {
				continue
			}
			func() {
				defer r.Close()
				for r.Next() {
					var id, seq int
					var to, from, toCol string
					var onUpd, onDel, match string
					if err := r.Scan(&id, &seq, &to, &from, &toCol, &onUpd, &onDel, &match); err != nil {
						return
					}
					if to == table {
						out = append(out, ForeignKey{FromTable: n, FromColumn: from, ToTable: to, ToColumn: toCol})
					}
				}
			}()
		}
		return out, nil
	}
}

func (d *DB) sqliteOutgoing(ctx context.Context, table string) []ForeignKey {
	var out []ForeignKey
	q := fmt.Sprintf(`PRAGMA foreign_key_list(%s)`, d.Driver.QuoteIdent(table))
	r, err := d.SQL.QueryContext(ctx, q)
	if err != nil {
		return nil
	}
	defer r.Close()
	for r.Next() {
		var id, seq int
		var to, from, toCol string
		var onUpd, onDel, match string
		if err := r.Scan(&id, &seq, &to, &from, &toCol, &onUpd, &onDel, &match); err != nil {
			break
		}
		out = append(out, ForeignKey{FromTable: table, FromColumn: from, ToTable: to, ToColumn: toCol})
	}
	return out
}
