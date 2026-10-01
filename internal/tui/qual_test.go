package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"

	dbpkg "db-peek/internal/db"
)

func TestCandidatesAmbiguous(t *testing.T) {
	e := NewExplorer("", []string{})
	e.Schemas = []SchemaNode{
		{Name: "public", Tables: []TableNode{{Name: "users"}}},
		{Name: "analytics", Tables: []TableNode{{Name: "users"}}},
	}
	got := e.candidates("users")
	if len(got) != 2 {
		t.Fatalf("candidates = %v", got)
	}
	if got[0] != "public" || got[1] != "analytics" {
		t.Fatalf("order = %v", got)
	}
	if len(e.candidates("missing")) != 0 {
		t.Fatal("expected no candidates")
	}
}

// An ER-driven selection of an ambiguous pg table must surface the
// resolver-style error with zero DB calls (nil cmd = no load issued).
func TestRecenterERAmbiguousPg(t *testing.T) {
	m := browseModel(t)
	m.db = &dbpkg.DB{Driver: dbpkg.Postgres, Display: "pg"}
	m.explorer = NewExplorer("pg", []string{"public", "analytics"})
	m.explorer.Schemas[0].Tables = []TableNode{{Schema: "public", Name: "users"}}
	m.explorer.Schemas[1].Tables = []TableNode{{Schema: "analytics", Name: "users"}}

	m2, cmd := m.recenterER("users")
	m = m2
	if cmd != nil {
		t.Fatal("ambiguous selection must not issue a DB load")
	}
	want := dbpkg.AmbiguousErr("users", []dbpkg.QualTable{
		{Schema: "public", Name: "users"},
		{Schema: "analytics", Name: "users"},
	}).Error()
	if m.err != want {
		t.Fatalf("err = %q, want %q", m.err, want)
	}
}

// A bare single-match pg selection still loads (bare fallback unchanged
// only in the sense that resolution picks the owner, no error).
func TestRecenterERSingleMatchPgLoads(t *testing.T) {
	m := browseModel(t)
	m.db = &dbpkg.DB{Driver: dbpkg.Postgres, Display: "pg"}
	m.explorer = NewExplorer("pg", []string{"public"})
	m.explorer.Schemas[0].Tables = []TableNode{{Schema: "public", Name: "users"}}

	m2, cmd := m.recenterER("users")
	m = m2
	if cmd == nil {
		t.Fatal("single-match selection must issue a detail load")
	}
	if m.err != "" {
		t.Fatalf("err = %q, want empty", m.err)
	}
}

// sqlite ambiguity is untouched: no candidates gate, always loads.
func TestRecenterERAmbiguousSQLiteLoads(t *testing.T) {
	m := browseModel(t)
	m.explorer = NewExplorer("", []string{"a", "b"})
	m.explorer.Schemas[0].Tables = []TableNode{{Schema: "a", Name: "users"}}
	m.explorer.Schemas[1].Tables = []TableNode{{Schema: "b", Name: "users"}}

	m2, cmd := m.recenterER("users")
	m = m2
	if cmd == nil {
		t.Fatal("sqlite selection must issue a detail load")
	}
	if m.err != "" {
		t.Fatalf("err = %q, want empty", m.err)
	}
}

// An ER-diagram reload over an ambiguous pg center must surface the
// resolver-style error with zero DB calls (nil cmd = no load issued).
func TestERReloadAmbiguousPg(t *testing.T) {
	m := browseModel(t)
	m.db = &dbpkg.DB{Driver: dbpkg.Postgres, Display: "pg"}
	m.explorer = NewExplorer("pg", []string{"public", "analytics"})
	m.explorer.Schemas[0].Tables = []TableNode{{Schema: "public", Name: "users"}}
	m.explorer.Schemas[1].Tables = []TableNode{{Schema: "analytics", Name: "users"}}
	m.table = "users"

	nm, cmd := m.erKeys(tea.KeyPressMsg{}, "r")
	m = nm.(Model)
	if cmd != nil {
		t.Fatal("ambiguous reload must not issue a DB load")
	}
	want := dbpkg.AmbiguousErr("users", []dbpkg.QualTable{
		{Schema: "public", Name: "users"},
		{Schema: "analytics", Name: "users"},
	}).Error()
	if m.err != want {
		t.Fatalf("err = %q, want %q", m.err, want)
	}
	if m.loading {
		t.Fatal("loading must be false after the ambiguity gate")
	}
}

// A write-preview over an ambiguous pg target must error on the query-done
// path instead of previewing a first-match schema. The pg driver is
// spoofed onto a live sqlite conn: the write itself executes, the preview
// read is gated with zero preview roundtrip.
func TestRunQueryPreviewAmbiguousPg(t *testing.T) {
	m := browseModel(t)
	d, err := dbpkg.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err := d.ExecStmt(context.Background(), "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)"); err != nil {
		t.Fatal(err)
	}
	d.Driver = dbpkg.Postgres
	m.db = d
	m.explorer = NewExplorer("pg", []string{"public", "analytics"})
	m.explorer.Schemas[0].Tables = []TableNode{{Schema: "public", Name: "users"}}
	m.explorer.Schemas[1].Tables = []TableNode{{Schema: "analytics", Name: "users"}}
	m.editor.SetText("INSERT INTO users (name) VALUES ('amy')")

	cmd := m.runQuery()
	if cmd == nil {
		t.Fatal("runQuery must return a cmd")
	}
	msg, ok := cmd().(queryDoneMsg)
	if !ok {
		t.Fatalf("msg = %T, want queryDoneMsg", msg)
	}
	want := dbpkg.AmbiguousErr("users", []dbpkg.QualTable{
		{Schema: "public", Name: "users"},
		{Schema: "analytics", Name: "users"},
	}).Error()
	if msg.err == nil || msg.err.Error() != want {
		t.Fatalf("err = %v, want %q", msg.err, want)
	}
	if msg.affected != 1 {
		t.Fatalf("affected = %d, want 1 (the write itself succeeded)", msg.affected)
	}
}
