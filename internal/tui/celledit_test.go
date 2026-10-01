package tui

import (
	"context"
	"strings"
	"testing"

	dbpkg "db-peek/internal/db"
)

func cellEditModel(t *testing.T, ddl string) Model {
	t.Helper()
	d, err := dbpkg.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if _, err := d.SQL.Exec(ddl); err != nil {
		t.Fatal(err)
	}
	m := browseModel(t)
	m.db = d
	m.focusDetail = true
	m.tab = 2
	m.table = "users"
	m.connSeq = 0
	m.detailSeq = 0
	m.loading = false
	return m
}

// insertUsers seeds rows and reloads the visible page, so the sample (and
// therefore the grid) matches the database.
func insertUsers(t *testing.T, m Model, rows ...[]any) Model {
	t.Helper()
	for _, r := range rows {
		args := make([]any, len(r))
		for i, v := range r {
			args[i] = v
		}
		ph := "?, ?, ?"
		if len(r) == 2 {
			ph = "?, ?"
		}
		if _, err := m.db.SQL.Exec(`INSERT INTO users(id, name) VALUES (`+ph+`)`, args...); err != nil {
			t.Fatal(err)
		}
	}
	s, err := m.db.PageRows(context.Background(), dbpkg.QualTable{Name: "users"}, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	m.sample = s
	m.buildRowTable()
	m.resizeBrowse()
	return m
}

// Editing a cell must reach the database. The long value proves the
// primary key is taken from the raw row, not the 60-char display text.
func TestCellEditUpdatesTheRow(t *testing.T) {
	m := cellEditModel(t, `CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT)`)
	long := strings.Repeat("x", 100)
	m = insertUsers(t, m, []any{int64(1), long}, []any{int64(2), "keep"})

	// Cursor: row 0, column 1 (name).
	m.rowTable.SetCursor(0)
	m.rowTable.SetCol(1)

	u, _ := m.Update(testKey("e"))
	m = u.(Model)
	if !m.cellEditing {
		t.Fatal("e must open the inline editor on the rows tab")
	}
	if got := m.cellInput.Value(); !strings.HasSuffix(got, "...") {
		t.Fatalf("the editor must show the grid's truncated display value, got %q", got)
	}
	m.cellInput.SetValue("edited")

	u, cmd2 := m.Update(testKey("enter"))
	m = u.(Model)
	if m.cellEditing {
		t.Fatal("enter must close the editor")
	}
	if cmd2 == nil {
		t.Fatal("enter must dispatch the UPDATE")
	}
	if msg, ok := cmd2().(cellUpdatedMsg); !ok {
		t.Fatalf("want a cellUpdatedMsg, got %T", cmd2())
	} else if msg.err != nil {
		t.Fatalf("the update failed: %v", msg.err)
	}
	nm, _ := m.Update(cmd2().(cellUpdatedMsg))
	m = nm.(Model)
	if !strings.HasPrefix(m.status, "updated users.name") {
		t.Fatalf("status = %q", m.status)
	}

	var got string
	if err := m.db.SQL.QueryRow(`SELECT name FROM users WHERE id = 1`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "edited" {
		t.Fatalf("the edited row must be persisted, got %q", got)
	}
	var other string
	if err := m.db.SQL.QueryRow(`SELECT name FROM users WHERE id = 2`).Scan(&other); err != nil {
		t.Fatal(err)
	}
	if other != "keep" {
		t.Fatalf("the other row must be untouched, got %q", other)
	}
}

// A table with no primary key cannot address a single row: the edit must
// be refused with an explanation and leave the data alone.
func TestCellEditNeedsAPrimaryKey(t *testing.T) {
	m := cellEditModel(t, `CREATE TABLE users(id INTEGER, name TEXT)`)
	m = insertUsers(t, m, []any{int64(1), "a"})
	m.rowTable.SetCursor(0)
	m.rowTable.SetCol(1)

	u, _ := m.Update(testKey("e"))
	m = u.(Model)
	if !m.cellEditing {
		t.Fatal("e must open the editor")
	}
	m.cellInput.SetValue("edited")
	u, cmd := m.Update(testKey("enter"))
	m = u.(Model)
	if cmd == nil {
		t.Fatal("enter must still dispatch the attempt")
	}
	msg, ok := cmd().(cellUpdatedMsg)
	if !ok {
		t.Fatalf("want a cellUpdatedMsg, got %T", cmd())
	}
	nm, _ := m.Update(msg)
	m = nm.(Model)
	if !strings.Contains(m.err, "no primary key") || !strings.Contains(m.err, "users") {
		t.Fatalf("the error must name the table and the limit, got %q", m.err)
	}
	var got string
	if err := m.db.SQL.QueryRow(`SELECT name FROM users WHERE id = 1`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "a" {
		t.Fatalf("a refused edit must not change the data, got %q", got)
	}
}

// esc abandons the edit without touching the database.
func TestCellEditEscCancels(t *testing.T) {
	m := cellEditModel(t, `CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT)`)
	m = insertUsers(t, m, []any{int64(1), "original"})
	s, err := m.db.PageRows(context.Background(), dbpkg.QualTable{Name: "users"}, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	m.sample = s
	m.buildRowTable()
	m.resizeBrowse()
	m.rowTable.SetCursor(0)
	m.rowTable.SetCol(1)

	u, _ := m.Update(testKey("e"))
	m = u.(Model)
	m.cellInput.SetValue("edited")
	u, cmd := m.Update(testKey("esc"))
	m = u.(Model)
	if m.cellEditing {
		t.Fatal("esc must close the editor")
	}
	if cmd != nil {
		t.Fatal("esc must not dispatch an UPDATE")
	}
	var got string
	if err := m.db.SQL.QueryRow(`SELECT name FROM users WHERE id = 1`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "original" {
		t.Fatalf("esc must leave the data alone, got %q", got)
	}
}

// The editor is a one-line overlay: the pane must keep its row count.
func TestCellEditOverlayKeepsRowCount(t *testing.T) {
	m := cellEditModel(t, `CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT)`)
	m = insertUsers(t, m, []any{int64(1), "a"}, []any{int64(2), "b"}, []any{int64(3), "c"})
	s, err := m.db.PageRows(context.Background(), dbpkg.QualTable{Name: "users"}, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	m.sample = s
	m.buildRowTable()
	m.resizeBrowse()

	before := strings.Count(m.detailView(), "\n")
	m.rowTable.SetCursor(1)
	m.rowTable.SetCol(1)
	u, _ := m.Update(testKey("e"))
	m = u.(Model)
	if after := strings.Count(m.detailView(), "\n"); after != before {
		t.Fatalf("the editor must not shift the pane: %d rows before, %d after", before, after)
	}
}

// The key parameter is bound as-is: its type must survive to the driver.
// Stringifying it works on SQLite (column affinity coerces the text) but
// makes every numeric key fail on Postgres with
// "operator does not exist: bigint = text", so the type is the contract.
func TestKeyParamPreservesDriverType(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   any
		want any
	}{
		{"int64 key", int64(42), int64(42)},
		{"int32 key", int32(7), int32(7)},
		{"float key", 1.5, 1.5},
		{"bool key", true, true},
		{"string key", "abc", "abc"},
		{"nil key", nil, nil},
	} {
		if got := keyParam(tc.in); got != tc.want {
			t.Fatalf("%s: got %#v (%T), want %#v (%T)", tc.name, got, got, tc.want, tc.want)
		}
	}
	// A []byte key must arrive as a string, not as a bytea.
	if got := keyParam([]byte("abc")); got != "abc" {
		t.Fatalf("[]byte key: got %#v (%T), want a string", got, got)
	}
}
