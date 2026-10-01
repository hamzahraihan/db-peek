package db

import (
	"context"
	"strings"
	"testing"
)

// The grid shows display strings truncated to 60 chars; an export must
// still get the whole value, so Sample carries both.
func TestSampleRawKeepsUntruncatedValues(t *testing.T) {
	d, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	long := strings.Repeat("x", 100)
	if _, err := d.SQL.Exec(`CREATE TABLE t(id INTEGER PRIMARY KEY, note TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SQL.Exec(`INSERT INTO t(id, note) VALUES (1, ?), (2, NULL)`, long); err != nil {
		t.Fatal(err)
	}

	s, err := d.PageRows(context.Background(), QualTable{Name: "t"}, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Rows) != 2 || len(s.Raw) != 2 {
		t.Fatalf("Raw must be parallel to Rows: %d rows, %d raw", len(s.Rows), len(s.Raw))
	}
	if !strings.HasSuffix(s.Rows[0][1], "...") {
		t.Fatalf("the display cell must be truncated, got %q", s.Rows[0][1])
	}
	got, ok := s.Raw[0][1].(string)
	if !ok {
		t.Fatalf("Raw must hold the driver value, got %T", s.Raw[0][1])
	}
	if got != long {
		t.Fatalf("Raw must keep all %d characters, got %d", len(long), len(got))
	}
	if s.Raw[1][1] != nil {
		t.Fatalf("SQL NULL must be nil in Raw, got %#v", s.Raw[1][1])
	}
	if s.Rows[1][1] != "NULL" {
		t.Fatalf("the grid still shows NULL, got %q", s.Rows[1][1])
	}
}
