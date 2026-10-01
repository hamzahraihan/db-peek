package db

import "testing"

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
