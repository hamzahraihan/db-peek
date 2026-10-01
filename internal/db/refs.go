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

func (d *DB) ForeignKeys(ctx context.Context, q QualTable) ([]ForeignKey, error) {
	switch d.Driver {
	case Postgres:
		rows, err := d.SQL.QueryContext(ctx, `
SELECT c.relname, a.attname, c2.relname, a2.attname
FROM pg_constraint o
JOIN pg_class c ON c.oid = o.conrelid
JOIN pg_class c2 ON c2.oid = o.confrelid
JOIN pg_namespace nc ON nc.oid = c.oid
JOIN pg_namespace nc2 ON nc2.oid = c2.oid
JOIN LATERAL unnest(o.conkey, o.confkey) WITH ORDINALITY AS k(attnum, confnum, ord) ON true
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = k.attnum
JOIN pg_attribute a2 ON a2.attrelid = c2.oid AND a2.attnum = k.confnum
WHERE o.contype = 'f' AND ((c.relname = $1 AND nc.nspname = $2) OR (c2.relname = $1 AND nc2.nspname = $2))
ORDER BY 1, 2`, q.Name, q.Schema, q.Name, q.Schema)
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
AND (TABLE_NAME = ? OR REFERENCED_TABLE_NAME = ?) ORDER BY 1, 2`, q.Name, q.Name)
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
		out, err := d.sqliteOutgoing(ctx, q.Name)
		if err != nil {
			return nil, err
		}
		// Incoming: scan all user tables' foreign_key_list for refs to table.
		// PRAGMA takes a bare table ident, never QuoteQual (see Columns).
		names, err := d.ListTables(ctx)
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			if n == q.Name {
				continue
			}
			qq := fmt.Sprintf(`PRAGMA foreign_key_list(%s)`, d.Driver.QuoteIdent(n))
			r, err := d.SQL.QueryContext(ctx, qq)
			if err != nil {
				return nil, err
			}
			var loopErr error
			func() {
				defer r.Close()
				for r.Next() {
					var id, seq int
					var to, from, toCol string
					var onUpd, onDel, match string
					if err := r.Scan(&id, &seq, &to, &from, &toCol, &onUpd, &onDel, &match); err != nil {
						loopErr = err
						return
					}
					if to == q.Name {
						out = append(out, ForeignKey{FromTable: n, FromColumn: from, ToTable: to, ToColumn: toCol})
					}
				}
				loopErr = r.Err()
			}()
			if loopErr != nil {
				return nil, loopErr
			}
		}
		return out, nil
	}
}

func (d *DB) sqliteOutgoing(ctx context.Context, table string) ([]ForeignKey, error) {
	var out []ForeignKey
	q := fmt.Sprintf(`PRAGMA foreign_key_list(%s)`, d.Driver.QuoteIdent(table))
	r, err := d.SQL.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	for r.Next() {
		var id, seq int
		var to, from, toCol string
		var onUpd, onDel, match string
		if err := r.Scan(&id, &seq, &to, &from, &toCol, &onUpd, &onDel, &match); err != nil {
			return nil, err
		}
		out = append(out, ForeignKey{FromTable: table, FromColumn: from, ToTable: to, ToColumn: toCol})
	}
	if err := r.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// AllForeignKeys aggregates ForeignKeys across tables and dedupes pairs.
// Empty input returns nil. Order: first-seen table order, then FK order.
func (d *DB) AllForeignKeys(ctx context.Context, tables []QualTable) ([]ForeignKey, error) {
	if len(tables) == 0 {
		return nil, nil
	}
	if d.Driver == SQLite {
		return d.sqliteAllForeignKeys(ctx, tables)
	}
	seen := map[string]bool{}
	var out []ForeignKey
	for _, t := range tables {
		fks, err := d.ForeignKeys(ctx, t)
		if err != nil {
			return nil, err
		}
		for _, fk := range fks {
			k := fk.FromTable + "|" + fk.FromColumn + "|" + fk.ToTable + "|" + fk.ToColumn
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, fk)
		}
	}
	return out, nil
}

// sqliteAllForeignKeys returns every FK in the database with a single
// ListTables and one PRAGMA foreign_key_list per table, instead of the
// N×N PRAGMA scan the per-table ForeignKeys path performs. Order is
// first-seen table order, matching AllForeignKeys' documented contract.
func (d *DB) sqliteAllForeignKeys(ctx context.Context, tables []QualTable) ([]ForeignKey, error) {
	want := make(map[string]bool, len(tables))
	for _, t := range tables {
		want[t.Name] = true
	}
	names, err := d.ListTables(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []ForeignKey
	for _, n := range names {
		if !want[n] {
			continue // only tables the caller asked about
		}
		q := fmt.Sprintf(`PRAGMA foreign_key_list(%s)`, d.Driver.QuoteIdent(n))
		r, err := d.SQL.QueryContext(ctx, q)
		if err != nil {
			return nil, err
		}
		var loopErr error
		func() {
			defer r.Close()
			for r.Next() {
				var id, seq int
				var to, from, toCol, onUpd, onDel, match string
				if err := r.Scan(&id, &seq, &to, &from, &toCol, &onUpd, &onDel, &match); err != nil {
					loopErr = err
					return
				}
				k := n + "|" + from + "|" + to + "|" + toCol
				if seen[k] {
					continue
				}
				seen[k] = true
				out = append(out, ForeignKey{FromTable: n, FromColumn: from, ToTable: to, ToColumn: toCol})
			}
			loopErr = r.Err()
		}()
		if loopErr != nil {
			return nil, loopErr
		}
	}
	return out, nil
}
