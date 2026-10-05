package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// TableDDL returns the CREATE TABLE statement for q.
//
// SQLite reads sqlite_master and MySQL uses SHOW CREATE TABLE, so both
// are native definitions. Postgres has no equivalent statement: callers
// use AssembleTableDDL over the already-loaded columns and indexes
// instead (no extra round trip).
func (d *DB) TableDDL(ctx context.Context, q QualTable) (string, error) {
	switch d.Driver {
	case MySQL:
		qq := fmt.Sprintf(`SHOW CREATE TABLE %s`, d.Driver.QuoteQual(q))
		var name, def string
		if err := d.SQL.QueryRowContext(ctx, qq).Scan(&name, &def); err != nil {
			return "", err
		}
		if strings.TrimSpace(def) == "" {
			return "", fmt.Errorf("table %q has no stored DDL", q.String())
		}
		return def, nil
	case Postgres:
		return "", fmt.Errorf("table %q: no native DDL on postgres (assemble from columns)", q.String())
	default:
		qq := `SELECT sql FROM sqlite_master WHERE type IN ('table','view') AND name = ` + d.Driver.Placeholder(1)
		var def sql.NullString
		if err := d.SQL.QueryRowContext(ctx, qq, q.Name).Scan(&def); err != nil {
			return "", err
		}
		if !def.Valid || strings.TrimSpace(def.String) == "" {
			return "", fmt.Errorf("table %q has no stored DDL", q.String())
		}
		return def.String, nil
	}
}

// AssembleTableDDL builds a best-effort CREATE TABLE for Postgres (which
// has no SHOW CREATE TABLE) from already-loaded columns and indexes.
// The pkey convention (<table>_pkey) maps to PRIMARY KEY; other indexes
// live on the indexes tab and are not repeated here.
func AssembleTableDDL(q QualTable, cols []Column, indexes []Index) string {
	qi := Postgres.QuoteIdent
	var b strings.Builder
	fmt.Fprintf(&b, "CREATE TABLE %s (\n", Postgres.QuoteQual(q))
	for i, c := range cols {
		fmt.Fprintf(&b, "  %s %s", qi(c.Name), c.Type)
		if strings.EqualFold(strings.TrimSpace(c.Nullable), "NO") {
			b.WriteString(" NOT NULL")
		}
		if strings.TrimSpace(c.Default) != "" {
			fmt.Fprintf(&b, " DEFAULT %s", strings.TrimSpace(c.Default))
		}
		if i < len(cols)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	if pk := pkeyCols(q, indexes); len(pk) > 0 {
		quoted := make([]string, len(pk))
		for i, p := range pk {
			quoted[i] = qi(strings.TrimSpace(p))
		}
		if len(cols) > 0 {
			// The last column line already ends with "\n" without a
			// comma when it is last; add the separator now.
			s := b.String()
			s = strings.TrimSuffix(s, "\n") + ",\n"
			s += fmt.Sprintf("  PRIMARY KEY (%s)\n", strings.Join(quoted, ", "))
			b.Reset()
			b.WriteString(s)
		} else {
			fmt.Fprintf(&b, "  PRIMARY KEY (%s)\n", strings.Join(quoted, ", "))
		}
	}
	b.WriteString(");")
	return b.String()
}

// pkeyCols returns the PRIMARY KEY columns via the Postgres <table>_pkey
// index convention, or nil when no such index exists.
func pkeyCols(q QualTable, indexes []Index) []string {
	for _, ix := range indexes {
		if ix.Name == q.Name+"_pkey" {
			var out []string
			for _, c := range strings.Split(ix.Columns, ",") {
				if c = strings.TrimSpace(c); c != "" {
					out = append(out, c)
				}
			}
			return out
		}
	}
	return nil
}
