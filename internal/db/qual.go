package db

import (
	"fmt"
	"sort"
	"strings"
)

// QualTable is a possibly-schema-qualified table reference. Schema == ""
// means bare (sqlite/mysql always; pg before resolution).
type QualTable struct {
	Schema string
	Name   string
}

func (q QualTable) String() string {
	if q.Schema == "" {
		return q.Name
	}
	return q.Schema + "." + q.Name
}

func unquotePart(p string) string {
	p = strings.TrimSpace(p)
	for {
		if len(p) >= 2 && p[0] == '"' && p[len(p)-1] == '"' {
			p = p[1 : len(p)-1]
			continue
		}
		if len(p) >= 2 && p[0] == '`' && p[len(p)-1] == '`' {
			p = p[1 : len(p)-1]
			continue
		}
		if len(p) >= 2 && p[0] == '[' && p[len(p)-1] == ']' {
			p = p[1 : len(p)-1]
			continue
		}
		return p
	}
}

// ParseQualTable splits "schema.table" or bare "table". More than one dot,
// or an empty side, is an error: db.schema.table is out of scope.
func ParseQualTable(s string) (QualTable, error) {
	t := strings.TrimSpace(s)
	if t == "" {
		return QualTable{}, fmt.Errorf("bad table name %q: want [schema.]table", s)
	}
	if strings.Count(t, ".") > 1 {
		return QualTable{}, fmt.Errorf("bad table name %q: want [schema.]table", s)
	}
	if strings.Count(t, ".") == 0 {
		name := unquotePart(t)
		if name == "" || strings.ContainsAny(name, " \t\n\r\"'`()[].,;") {
			return QualTable{}, fmt.Errorf("bad table name %q: want [schema.]table", s)
		}
		return QualTable{Name: name}, nil
	}
	i := strings.LastIndex(t, ".")
	schema := unquotePart(t[:i])
	name := unquotePart(t[i+1:])
	if schema == "" || name == "" {
		return QualTable{}, fmt.Errorf("bad table name %q: want [schema.]table", s)
	}
	if strings.ContainsAny(schema, " \t\n\r\"'`()[].,;") || strings.ContainsAny(name, " \t\n\r\"'`()[].,;") {
		return QualTable{}, fmt.Errorf("bad table name %q: want [schema.]table", s)
	}
	return QualTable{Schema: schema, Name: name}, nil
}

// QuoteQual renders a qualified ident: each part goes through QuoteIdent,
// so embedded quotes escape (`we"ird` → `"we""ird"`). Empty schema keeps
// today's single-ident rendering (sqlite/mysql compatible).
func (d Driver) QuoteQual(q QualTable) string {
	if q.Schema == "" {
		return d.QuoteIdent(q.Name)
	}
	return d.QuoteIdent(q.Schema) + "." + d.QuoteIdent(q.Name)
}

// AmbiguousErr formats the duplicate-name error: public first, cap 5.
// Exported: the TUI surfaces it without a DB roundtrip (Task 4) and the
// resolver returns it (Task 3).
func AmbiguousErr(name string, quals []QualTable) error {
	cp := append([]QualTable(nil), quals...)
	sort.Slice(cp, func(i, j int) bool {
		if (cp[i].Schema == "public") != (cp[j].Schema == "public") {
			return cp[i].Schema == "public"
		}
		if cp[i].Schema != cp[j].Schema {
			return cp[i].Schema < cp[j].Schema
		}
		return cp[i].Name < cp[j].Name
	})
	strs := make([]string, len(cp))
	for i, q := range cp {
		strs[i] = q.String()
	}
	if len(strs) > 5 {
		strs = append(strs[:5], "(+N more)")
	}
	return fmt.Errorf("table %q is ambiguous: %s — qualify as schema.table", name, strings.Join(strs, ", "))
}
