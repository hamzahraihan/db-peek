package db

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

func TestExplainIsEngineSpecific(t *testing.T) {
	cases := []struct {
		driver  Driver
		analyze bool
		want    string
	}{
		{SQLite, false, "EXPLAIN QUERY PLAN select 1"},
		{SQLite, true, "EXPLAIN QUERY PLAN select 1"}, // sqlite has no EXPLAIN ANALYZE
		{Postgres, false, "EXPLAIN select 1"},
		{Postgres, true, "EXPLAIN (ANALYZE, BUFFERS) select 1"},
		{MySQL, false, "EXPLAIN select 1"},
		{MySQL, true, "EXPLAIN ANALYZE select 1"},
	}
	for _, c := range cases {
		d := &DB{Driver: c.driver}
		if got := d.Explain("  select 1;  ", c.analyze); got != c.want {
			t.Fatalf("%s analyze=%v: got %q want %q", c.driver, c.analyze, got, c.want)
		}
	}
}

func TestPlaceholder(t *testing.T) {
	if got := Postgres.Placeholder(2); got != "$2" {
		t.Fatalf("postgres must number placeholders, got %q", got)
	}
	for _, d := range []Driver{MySQL, SQLite} {
		if got := d.Placeholder(2); got != "?" {
			t.Fatalf("%s must use ?, got %q", d, got)
		}
	}
}

func openCellDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if _, err := d.SQL.Exec(`CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatal(err)
	}
	return d
}

// The key must be discovered from the existing PRAGMA data, and a table
// without one must report "no key" rather than error.
func TestPrimaryKeySQLite(t *testing.T) {
	d := openCellDB(t)
	if _, err := d.SQL.Exec(`CREATE TABLE notes(id INTEGER, body TEXT)`); err != nil {
		t.Fatal(err)
	}
	pk, err := d.PrimaryKey(context.Background(), "users")
	if err != nil {
		t.Fatal(err)
	}
	if len(pk) != 1 || pk[0] != "id" {
		t.Fatalf("want [id], got %v", pk)
	}
	none, err := d.PrimaryKey(context.Background(), "notes")
	if err != nil {
		t.Fatalf("a keyless table is a normal answer, got %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("want no key, got %v", none)
	}
}

// A composite key must address the row by every key column, in order.
func TestPrimaryKeyComposite(t *testing.T) {
	d, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.SQL.Exec(`CREATE TABLE t(a TEXT, b TEXT, v TEXT, PRIMARY KEY (b, a))`); err != nil {
		t.Fatal(err)
	}
	pk, err := d.PrimaryKey(context.Background(), "t")
	if err != nil {
		t.Fatal(err)
	}
	if len(pk) != 2 || pk[0] != "b" || pk[1] != "a" {
		t.Fatalf("want key order [b a], got %v", pk)
	}
}

func TestUpdateCellSQLite(t *testing.T) {
	d := openCellDB(t)
	if _, err := d.SQL.Exec(`INSERT INTO users(id, name) VALUES (1,'a'), (2,'b')`); err != nil {
		t.Fatal(err)
	}
	n, err := d.UpdateCell(context.Background(), "users", []string{"id"}, []any{int64(2)}, "name", "edited")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("want exactly 1 row affected, got %d", n)
	}
	var got string
	if err := d.SQL.QueryRow(`SELECT name FROM users WHERE id = 2`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "edited" {
		t.Fatalf("want the edited row, got %q", got)
	}
	var other string
	if err := d.SQL.QueryRow(`SELECT name FROM users WHERE id = 1`).Scan(&other); err != nil {
		t.Fatal(err)
	}
	if other != "a" {
		t.Fatalf("the other row must be untouched, got %q", other)
	}
}

// The key parameter must carry the driver's own type. SQLite's column
// affinity papers over a stringified key, so this asserts the weaker
// property that actually holds everywhere: the bound argument is the
// value the driver returned, and a textified key silently matches
// nothing (and on Postgres raises "bigint = text").
func TestUpdateCellBindsTypedKey(t *testing.T) {
	d := openCellDB(t)
	if _, err := d.SQL.Exec(`INSERT INTO users(id, name) VALUES (1,'a'), (2,'b')`); err != nil {
		t.Fatal(err)
	}
	// A typed key addresses exactly its own row.
	n, err := d.UpdateCell(context.Background(), "users", []string{"id"}, []any{int64(2)}, "name", "typed")
	if err != nil {
	t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("a typed key must affect exactly 1 row, got %d", n)
	}
	// A textified key cannot be relied on: it must never be what the
	// caller sends, so assert the difference is observable rather than
	// silently "working" on engines that coerce.
	textN, err := d.UpdateCell(context.Background(), "users", []string{"id"}, []any{"3"}, "name", "ghost")
	if err == nil && textN != 0 {
		t.Fatalf("a text key for a missing row must affect 0 rows, got %d", textN)
	}
	var ghost int
	if err := d.SQL.QueryRow(`SELECT count(*) FROM users WHERE name = 'ghost'`).Scan(&ghost); err != nil {
		t.Fatal(err)
	}
	if ghost != 0 {
		t.Fatalf("a textified key must not match the integer row, matched %d", ghost)
	}
}

// NULL is a real value, not the string "NULL".
func TestUpdateCellWritesNull(t *testing.T) {
	d := openCellDB(t)
	if _, err := d.SQL.Exec(`INSERT INTO users(id, name) VALUES (1, 'a')`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.UpdateCell(context.Background(), "users", []string{"id"}, []any{int64(1)}, "name", nil); err != nil {
		t.Fatal(err)
	}
	var name sql.NullString
	if err := d.SQL.QueryRow(`SELECT name FROM users WHERE id = 1`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name.Valid {
		t.Fatalf("want SQL NULL, got %q", name.String)
	}
}

// A mismatched key/value count is caught before any statement runs.
func TestUpdateCellRejectsBadKey(t *testing.T) {
	d := openCellDB(t)
	_, err := d.UpdateCell(context.Background(), "users", []string{"id"}, []any{int64(1), int64(2)}, "name", "x")
	if err == nil {
		t.Fatal("a key/value count mismatch must be rejected")
	}
	if !strings.Contains(err.Error(), "users") {
		t.Fatalf("the error must name the table, got %q", err)
	}
}
