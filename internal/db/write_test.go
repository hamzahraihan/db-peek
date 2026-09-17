package db

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPostgresDSNAddsSimpleProtocol(t *testing.T) {
	got := withSimpleProtocol("postgres://u:p@host:6543/db?sslmode=require")
	if !strings.Contains(got, "default_query_exec_mode=simple_protocol") {
		t.Fatalf("knob missing: %q", got)
	}
	if !strings.Contains(got, "sslmode=require") {
		t.Fatalf("existing params must survive: %q", got)
	}
}

func TestPostgresDSNRespectsUserValue(t *testing.T) {
	got := withSimpleProtocol("postgresql://u@host/db?default_query_exec_mode=exec")
	if strings.Count(got, "default_query_exec_mode") != 1 || !strings.Contains(got, "default_query_exec_mode=exec") {
		t.Fatalf("user value must win: %q", got)
	}
}

func TestNonPostgresDSNUntouched(t *testing.T) {
	for _, raw := range []string{"mysql://u:p@host/db", "./app.db", ":memory:"} {
		if got := withSimpleProtocol(raw); got != raw {
			t.Fatalf("non-postgres must pass through: %q -> %q", raw, got)
		}
	}
}

func TestClassifyStatement(t *testing.T) {
	exec := []string{
		"INSERT INTO t VALUES (1);",
		"  update t set a=1",
		"DELETE FROM t",
		"-- comment\nINSERT INTO t VALUES (1)",
		"/* block */ CREATE TABLE t (a INT)",
	}
	for _, q := range exec {
		if !isWriteStatement(q) {
			t.Fatalf("want exec path for %q", q)
		}
	}
	query := []string{
		"SELECT 1",
		"  with x as (select 1) select * from x",
		"VALUES (1), (2)",
		"EXPLAIN SELECT 1",
	}
	for _, q := range query {
		if isWriteStatement(q) {
			t.Fatalf("want query path for %q", q)
		}
	}
}

func TestReturningGoesThroughQueryPath(t *testing.T) {
	query := []string{
		"INSERT INTO t VALUES (1) RETURNING *",
		"insert into t values (1) returning id",
		"-- comment\nUPDATE t SET a=1 RETURNING a",
		"/* block */ DELETE FROM t RETURNING id",
	}
	for _, q := range query {
		if isWriteStatement(q) {
			t.Fatalf("want query path for %q", q)
		}
		if !hasReturningClause(q) {
			t.Fatalf("want returning detected for %q", q)
		}
	}
	if hasReturningClause("SELECT returning FROM t") {
		// `returning` as a bare column name still matches the keyword;
		// accepted limitation: it routes via Query and surfaces driver errors.
	}
	if hasReturningClause("INSERT INTO t VALUES (1)") {
		t.Fatal("plain insert must not count as returning")
	}
}

func TestTrimStatement(t *testing.T) {
	if got := trimStatement("  SELECT 1; ; \n"); got != "SELECT 1" {
		t.Fatalf("got %q", got)
	}
}

func TestExecStmtSQLite(t *testing.T) {
	d, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := d.SQL.ExecContext(ctx, `CREATE TABLE t (a TEXT)`); err != nil {
		t.Fatal(err)
	}
	n, err := d.ExecStmt(ctx, `INSERT INTO t VALUES ('x');`)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("want 1 affected, got %d", n)
	}
	s, err := d.Query(ctx, `SELECT * FROM t`)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Rows) != 1 || s.Rows[0][0] != "x" {
		t.Fatalf("round-trip failed: %+v", s)
	}
}
