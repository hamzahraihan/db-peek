package db

import (
	"context"
	"database/sql"
	"testing"
)

func openFKMem(t *testing.T) *DB {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = sqlDB.Exec(`PRAGMA foreign_keys=ON;
CREATE TABLE customers(id INTEGER PRIMARY KEY, name TEXT);
CREATE TABLE orders(id INTEGER PRIMARY KEY, customer_id INTEGER REFERENCES customers(id), status TEXT);`)
	if err != nil {
		t.Fatal(err)
	}
	return &DB{SQL: sqlDB, Driver: SQLite, Display: ":memory:"}
}

func TestForeignKeysSQLite(t *testing.T) {
	d := openFKMem(t)
	defer d.SQL.Close()
	out, err := d.ForeignKeys(context.Background(), "orders")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, fk := range out {
		if fk.FromTable == "orders" && fk.FromColumn == "customer_id" && fk.ToTable == "customers" && fk.ToColumn == "id" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing orders→customers fk in %v", out)
	}
	in, err := d.ForeignKeys(context.Background(), "customers")
	if err != nil {
		t.Fatal(err)
	}
	back := false
	for _, fk := range in {
		if fk.FromTable == "orders" && fk.ToTable == "customers" {
			back = true
		}
	}
	if !back {
		t.Fatalf("missing incoming fk for customers in %v", in)
	}
}

func TestForeignKeysSQLiteError(t *testing.T) {
	d := openFKMem(t)
	d.SQL.Close()
	if _, err := d.ForeignKeys(context.Background(), "orders"); err == nil {
		t.Fatal("expected error on closed DB, got nil")
	}
}
