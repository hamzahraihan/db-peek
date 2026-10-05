package db

import (
	"context"
	"strings"
	"testing"
)

func TestTableDDLSqlite(t *testing.T) {
	d, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.SQL.Exec(`CREATE TABLE users(id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	got, err := d.TableDDL(context.Background(), QualTable{Name: "users"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "CREATE TABLE users") || !strings.Contains(got, "PRIMARY KEY") {
		t.Fatalf("DDL must hold the table definition, got %q", got)
	}
}

func TestAssembleTableDDLPostgres(t *testing.T) {
	cols := []Column{
		{Name: "id", Type: "integer", Nullable: "NO"},
		{Name: "name", Type: "text", Nullable: "NO", Default: "'anon'"},
	}
	indexes := []Index{{Name: "users_pkey", Columns: "id"}}
	got := AssembleTableDDL(QualTable{Schema: "public", Name: "users"}, cols, indexes)
	if !strings.Contains(got, `CREATE TABLE "public"."users"`) {
		t.Fatalf("must qualify schema table, got %q", got)
	}
	if !strings.Contains(got, `"id" integer NOT NULL`) || !strings.Contains(got, `PRIMARY KEY ("id")`) {
		t.Fatalf("must render columns and pk, got %q", got)
	}
	if !strings.Contains(got, `DEFAULT 'anon'`) {
		t.Fatalf("must render default, got %q", got)
	}
}
