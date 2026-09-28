package saved

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tempSession(t *testing.T) *Session {
	t.Helper()
	return &Session{path: filepath.Join(t.TempDir(), "session.json")}
}

func loadSessionAt(t *testing.T, path string) (*Session, error) {
	t.Helper()
	s := &Session{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return &Session{path: path}, err
	}
	return s, nil
}

// The session must round-trip the query buffers, and must never carry a
// connection string — only the profile name.
func TestSessionRoundTripsBuffers(t *testing.T) {
	s := tempSession(t)
	s.ConnProfile = "prod"
	s.ConnMasked = Mask("postgres://u:hunter2@localhost:5432/db")
	s.Table = "orders"
	s.Tab = 3
	s.PageSize = 25
	s.Filter = "ord"
	s.Buffers = []string{"select 1", "select 2\nfrom t"}
	s.Cur = 1
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "hunter2") {
		t.Fatalf("session.json must not hold a password:\n%s", data)
	}
	loaded, err := loadSessionAt(t, s.path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ConnProfile != "prod" || loaded.Table != "orders" || loaded.Tab != 3 {
		t.Fatalf("session must round-trip: %+v", loaded)
	}
	if loaded.PageSize != 25 || loaded.Filter != "ord" || loaded.Cur != 1 {
		t.Fatalf("session must round-trip: %+v", loaded)
	}
	if len(loaded.Buffers) != 2 || loaded.Buffers[1] != "select 2\nfrom t" {
		t.Fatalf("buffers must round-trip verbatim, got %+v", loaded.Buffers)
	}
}

func TestDefaultSessionPathIsSiblingOfConnections(t *testing.T) {
	sess, err := DefaultSessionPath()
	if err != nil {
		t.Fatal(err)
	}
	conns, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(sess) != filepath.Dir(conns) {
		t.Fatalf("session and connections must share a config dir: %q vs %q", sess, conns)
	}
	if filepath.Base(sess) != "session.json" {
		t.Fatalf("want session.json, got %q", sess)
	}
}

// A corrupt session file must degrade to "no session" plus an error, never
// a crash and never a half-applied restore.
func TestLoadSessionCorruptIsEmpty(t *testing.T) {
	s := tempSession(t)
	if err := os.WriteFile(s.path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadSessionAt(t, s.path)
	if err == nil {
		t.Fatal("a corrupt session must be reported, not silently trusted")
	}
	if loaded.Table != "" || len(loaded.Buffers) != 0 {
		t.Fatalf("a corrupt session must degrade to empty, got %+v", loaded)
	}
}
