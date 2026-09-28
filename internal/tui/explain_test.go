package tui

import (
	"context"
	"strings"
	"testing"

	dbpkg "db-peek/internal/db"
)

// ctrl+e runs the buffer through EXPLAIN without rewriting the editor,
// and the plan comes back in the results grid.
func TestExplainRendersPlanAndKeepsBuffer(t *testing.T) {
	d, err := dbpkg.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.SQL.Exec(`CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT); INSERT INTO users VALUES (1,'a')`); err != nil {
		t.Fatal(err)
	}

	m := queryTabModel()
	m.db = d
	m.connSeq = 0
	m.editor.SetText("select * from users")
	u, cmd := m.Update(testKey("ctrl+e"))
	m = u.(Model)
	if cmd == nil {
		t.Fatal("ctrl+e must dispatch a run")
	}
	if m.editor.Text() != "select * from users" {
		t.Fatalf("the buffer must not be rewritten, got %q", m.editor.Text())
	}
	if m.pendingExplain || m.pendingAnalyze {
		t.Fatal("the explain flag applies to one run only")
	}
	msg, ok := cmd().(queryDoneMsg)
	if !ok {
		t.Fatalf("want a queryDoneMsg, got %T", cmd())
	}
	if !strings.HasPrefix(msg.sql, "EXPLAIN QUERY PLAN") {
		t.Fatalf("SQLite must get EXPLAIN QUERY PLAN, got %q", msg.sql)
	}
	if msg.err != nil {
		t.Fatalf("the plan must run: %v", msg.err)
	}
	if msg.sample == nil || len(msg.sample.Rows) == 0 {
		t.Fatal("the plan must come back as rows")
	}

	nm, _ := m.Update(msg)
	got := nm.(Model)
	if got.querySample == nil || len(got.querySample.Rows) == 0 {
		t.Fatal("the plan must render in the results grid")
	}
	if got.editor.Text() != "select * from users" {
		t.Fatalf("the buffer must survive the plan, got %q", got.editor.Text())
	}
}

// ctrl+y is EXPLAIN ANALYZE, which SQLite does not have; the statement
// must still be valid.
func TestExplainAnalyzeOnSQLite(t *testing.T) {
	d, err := dbpkg.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.SQL.Exec(`CREATE TABLE t(a); INSERT INTO t VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	m := queryTabModel()
	m.db = d
	m.editor.SetText("select * from t")
	u, cmd := m.Update(testKey("ctrl+y"))
	m = u.(Model)
	msg, ok := cmd().(queryDoneMsg)
	if !ok {
		t.Fatalf("want a queryDoneMsg, got %T", cmd())
	}
	if !strings.HasPrefix(msg.sql, "EXPLAIN QUERY PLAN") {
		t.Fatalf("SQLite ignores analyze, got %q", msg.sql)
	}
	if msg.err != nil {
		t.Fatalf("the statement must be valid on SQLite: %v", msg.err)
	}
}

// Without a connection there is nothing to explain and nothing to break.
func TestExplainWithoutDBDoesNotPanic(t *testing.T) {
	m := queryTabModel()
	m.db = nil
	m.editor.SetText("select 1")
	u, _ := m.Update(testKey("ctrl+e"))
	if got := u.(Model); got.editor.Text() != "select 1" {
		t.Fatalf("the buffer must be untouched, got %q", got.editor.Text())
	}
	_ = context.Background()
}
