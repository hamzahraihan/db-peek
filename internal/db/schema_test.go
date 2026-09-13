package db

import (
	"context"
	"database/sql"
	"testing"
)

func openMem(t *testing.T) *DB {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT); CREATE VIEW v_users AS SELECT * FROM users; INSERT INTO users(name) VALUES('a'),('b');`); err != nil {
		t.Fatal(err)
	}
	return &DB{SQL: sqlDB, Driver: SQLite, Display: ":memory:"}
}

func TestListSchemasSQLite(t *testing.T) {
	d := openMem(t)
	defer d.SQL.Close()
	got, err := d.ListSchemas(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "main" {
		t.Fatalf("want [main], got %v", got)
	}
}

func TestListTablesInSchemaSQLite(t *testing.T) {
	d := openMem(t)
	defer d.SQL.Close()
	refs, err := d.ListTablesInSchema(context.Background(), "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 2 {
		t.Fatalf("want 2 refs, got %v", refs)
	}
}

func TestApproxCountSQLite(t *testing.T) {
	d := openMem(t)
	defer d.SQL.Close()
	n, err := d.ApproxCount(context.Background(), "main", "users")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("want 2, got %d", n)
	}
}
