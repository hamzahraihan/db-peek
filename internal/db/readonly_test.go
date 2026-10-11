package db

import (
	"testing"
)

func TestIsWriteStatement(t *testing.T) {
	for sql, want := range map[string]bool{
		"SELECT * FROM t":                          false,
		"  -- comment\nUPDATE t SET a = 1":         true,
		"WITH x AS (SELECT 1) SELECT * FROM x":     false,
		"INSERT INTO t(a) VALUES (1)":              true,
		"DELETE FROM t":                            true,
		"CREATE TABLE t(a INT)":                    true,
		"EXPLAIN SELECT * FROM t":                  false,
		"INSERT INTO t(a) VALUES (1) RETURNING a":  true,
		"DELETE FROM t RETURNING id":               true,
		"WITH d AS (DELETE FROM t) SELECT 1":       true,
		"WITH u AS (UPDATE t SET a = 1) SELECT 1":  true,
	} {
		if got := IsWriteStatement(sql); got != want {
			t.Fatalf("IsWriteStatement(%q) = %v, want %v", sql, got, want)
		}
	}
}

func TestIsRowReturning(t *testing.T) {
	for sql, want := range map[string]bool{
		"SELECT * FROM t":                         true,
		"INSERT INTO t(a) VALUES (1) RETURNING a": true,
		"DELETE FROM t RETURNING id":              true,
		"WITH x AS (SELECT 1) SELECT * FROM x":    true,
		"EXPLAIN SELECT * FROM t":                 true,
		"INSERT INTO t(a) VALUES (1)":             false,
		"DELETE FROM t":                           false,
		"UPDATE t SET a = 1":                      false,
		"CREATE TABLE t(a INT)":                   false,
	} {
		if got := IsRowReturning(sql); got != want {
			t.Fatalf("IsRowReturning(%q) = %v, want %v", sql, got, want)
		}
	}
}
