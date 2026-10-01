package db

import (
	"context"
	"testing"
)

func TestParseQualTable(t *testing.T) {
	q, err := ParseQualTable("analytics.users")
	if err != nil {
		t.Fatal(err)
	}
	if q.Schema != "analytics" || q.Name != "users" {
		t.Fatalf("got %+v", q)
	}
	if q.String() != "analytics.users" {
		t.Fatalf("string = %q", q.String())
	}
	bare, err := ParseQualTable("users")
	if err != nil || bare.Schema != "" || bare.Name != "users" || bare.String() != "users" {
		t.Fatalf("bare = %+v, err = %v", bare, err)
	}
	if got := Postgres.QuoteQual(q); got != `"analytics"."users"` {
		t.Fatalf("quote = %q", got)
	}
}

func TestParseQualTableRejects(t *testing.T) {
	for _, s := range []string{"", "a.", ".b", "a.b.c", `""`, "a b"} {
		if _, err := ParseQualTable(s); err == nil {
			t.Fatalf("ParseQualTable(%q) = nil error", s)
		}
	}
}

func TestQuoteQualEscapes(t *testing.T) {
	q := QualTable{Schema: `we"ird`, Name: "t"}
	if got := Postgres.QuoteQual(q); got != `"we""ird"."t"` {
		t.Fatalf("quote = %q", got)
	}
	if got := MySQL.QuoteQual(QualTable{Schema: "s", Name: "t"}); got != "`s`.`t`" {
		t.Fatalf("mysql quote = %q", got)
	}
	if got := SQLite.QuoteQual(QualTable{Name: "t"}); got != `"t"` {
		t.Fatalf("bare quote = %q", got)
	}
}

func TestQualSchemaIgnoredSQLite(t *testing.T) {
	d := openMem(t)
	defer d.SQL.Close()
	ctx := context.Background()
	bare, err := d.Columns(ctx, QualTable{Name: "users"})
	if err != nil {
		t.Fatal(err)
	}
	qual, err := d.Columns(ctx, QualTable{Schema: "main", Name: "users"})
	if err != nil {
		t.Fatal(err)
	}
	if len(bare) != len(qual) {
		t.Fatalf("bare %d cols vs qual %d cols", len(bare), len(qual))
	}
	if _, err := d.Count(ctx, QualTable{Schema: "main", Name: "users"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.PageRows(ctx, QualTable{Schema: "main", Name: "users"}, 5, 0); err != nil {
		t.Fatal(err)
	}
}
