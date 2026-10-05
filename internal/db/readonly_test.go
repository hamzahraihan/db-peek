package db

import (
	"testing"
)

func TestIsWriteStatement(t *testing.T) {
	for sql, want := range map[string]bool{
		"SELECT * FROM t":                        false,
		"  -- comment\nUPDATE t SET a = 1":       true,
		"WITH x AS (SELECT 1) SELECT * FROM x":   false,
		"INSERT INTO t(a) VALUES (1)":            true,
		"DELETE FROM t":                          true,
		"CREATE TABLE t(a INT)":                  true,
		"EXPLAIN SELECT * FROM t":                false,
		"INSERT INTO t(a) VALUES (1) RETURNING a": false,
	} {
		if got := IsWriteStatement(sql); got != want {
			t.Fatalf("IsWriteStatement(%q) = %v, want %v", sql, got, want)
		}
	}
}
