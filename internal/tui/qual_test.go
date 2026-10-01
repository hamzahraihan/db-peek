package tui

import (
	"testing"

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
