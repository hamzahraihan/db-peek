package saved

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	return &Store{Path: filepath.Join(t.TempDir(), "connections.json")}
}

// blockedPath returns a path whose parent is a regular file, so any write
// fails on every platform (Windows ignores directory permissions).
func blockedPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("not a dir"), 0o600); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(blocker, "connections.json")
}

func readProfiles(t *testing.T, path string) []Profile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []Profile
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// A failed write must not leave the in-memory list ahead of the file, or
// the sidebar shows entries that a restart silently drops.
func TestUpsertFailureLeavesListUnchanged(t *testing.T) {
	s := tempStore(t)
	if err := s.Upsert("keep", "./a.db"); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	s.Path = blockedPath(t)

	if err := s.Upsert("lost", "./b.db"); err == nil {
		t.Fatal("expected the write to fail")
	}
	if got := s.List(); len(got) != 1 || got[0].Name != "keep" {
		t.Fatalf("store must roll back, got %+v", got)
	}
}

func TestDeleteFailureLeavesListUnchanged(t *testing.T) {
	s := tempStore(t)
	if err := s.Upsert("keep", "./a.db"); err != nil {
		t.Fatal(err)
	}
	s.Path = blockedPath(t)

	if err := s.Delete("keep"); err == nil {
		t.Fatal("expected the write to fail")
	}
	if _, ok := s.Get("keep"); !ok {
		t.Fatal("delete must roll back, entry vanished from the list")
	}
}

func TestUpsertRoundTripsThroughFile(t *testing.T) {
	s := tempStore(t)
	if err := s.Upsert("Prod", "postgres://u:p@localhost:5432/db"); err != nil {
		t.Fatal(err)
	}
	again := &Store{Path: s.Path, profiles: readProfiles(t, s.Path)}
	if p, ok := again.Get("prod"); !ok || p.Conn != "postgres://u:p@localhost:5432/db" {
		t.Fatalf("round-trip must preserve the profile, got %+v ok=%v", p, ok)
	}
}

// A hand-edited file with two case variants must not have both rewritten.
func TestUpsertReplacesFirstCaseVariantOnly(t *testing.T) {
	s := tempStore(t)
	if err := s.Upsert("Prod", "postgres://one@localhost/db"); err != nil {
		t.Fatal(err)
	}
	if err := s.Upsert("prod", "postgres://two@localhost/db"); err != nil {
		t.Fatal(err)
	}
	got := s.List()
	if len(got) != 1 {
		t.Fatalf("case variants must collapse to one entry, got %+v", got)
	}
	if got[0].Name != "Prod" {
		t.Fatalf("original name casing must win, got %q", got[0].Name)
	}
}

func TestDeleteRemovesSingleEntry(t *testing.T) {
	s := tempStore(t)
	for _, n := range []string{"a", "b", "c"} {
		if err := s.Upsert(n, "./"+n+".db"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Delete("b"); err != nil {
		t.Fatal(err)
	}
	got := s.List()
	if len(got) != 2 {
		t.Fatalf("delete must remove exactly one entry, got %+v", got)
	}
	for _, p := range got {
		if p.Name == "b" {
			t.Fatalf("b must be gone: %+v", got)
		}
	}
	if err := s.Delete("missing"); err == nil {
		t.Fatal("deleting an unknown name must error")
	}
}

// A config dir that cannot be resolved must report why, not "open :".
func TestPersistWithoutPathNamesTheCause(t *testing.T) {
	s := &Store{pathErr: os.ErrNotExist}
	err := s.persist([]Profile{{Name: "x", Conn: "y"}})
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "config dir unavailable") {
		t.Fatalf("must explain the cause, got %q", msg)
	}
}
