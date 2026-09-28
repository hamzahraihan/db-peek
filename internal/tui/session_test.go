package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"db-peek/internal/saved"
)

func sessionStore(t *testing.T, name, conn string) *saved.Store {
	t.Helper()
	s := &saved.Store{Path: filepath.Join(t.TempDir(), "connections.json")}
	if name != "" {
		if err := s.Upsert(name, conn); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

// A quit must record the work in progress, and the next launch must put it
// back — without the session ever holding a connection string.
func TestSessionRoundTripsThroughModel(t *testing.T) {
	store := sessionStore(t, "prod", "postgres://u:hunter2@localhost:5432/db")
	m := New("", store)
	m.connStr = "postgres://u:hunter2@localhost:5432/db" // the live connection
	m.table = "orders"
	m.tab = 3
	m.pageSize = 50
	m.explorer.SetFilter("ord")
	m.editor.SetText("select * from orders")
	m.saveActiveBuf()
	m.newQueryBuf()
	m.editor.SetText("select 1")
	m.saveActiveBuf()
	m.loadActiveBuf(1)

	sess := &saved.Session{}
	m.fillSession(sess)

	if sess.ConnProfile != "prod" {
		t.Fatalf("the session must name the profile, got %q", sess.ConnProfile)
	}
	if strings.Contains(sess.ConnMasked, "hunter2") {
		t.Fatalf("the session must not hold the password: %q", sess.ConnMasked)
	}
	if sess.Table != "orders" || sess.Tab != 3 || sess.PageSize != 50 || sess.Cur != 1 {
		t.Fatalf("session lost pane state: %+v", sess)
	}
	if sess.Filter != "ord" {
		t.Fatalf("session lost the sidebar filter: %q", sess.Filter)
	}
	if len(sess.Buffers) != 2 || sess.Buffers[0] != "select * from orders" || sess.Buffers[1] != "select 1" {
		t.Fatalf("session lost the query buffers: %+v", sess.Buffers)
	}

	// The next launch restores them.
	old := loadSessionFunc
	loadSessionFunc = func() (*saved.Session, error) { return sess, nil }
	t.Cleanup(func() { loadSessionFunc = old })

	m2 := New("", sessionStore(t, "prod", "postgres://u:hunter2@localhost:5432/db"))
	if m2.connStr != "postgres://u:hunter2@localhost:5432/db" {
		t.Fatalf("the saved profile must be reconnected, got %q", m2.connStr)
	}
	if m2.pageSize != 50 {
		t.Fatalf("page size must be restored, got %d", m2.pageSize)
	}
	if m2.qcur != 1 || m2.editor.Text() != "select 1" {
		t.Fatalf("the active query buffer must be restored, got qcur=%d %q", m2.qcur, m2.editor.Text())
	}
	if len(m2.qbufs) != 2 || m2.qbufs[0].editor.Text() != "select * from orders" {
		t.Fatalf("every query buffer must be restored, got %+v", m2.qbufs)
	}
	if m2.restoreTable != "orders" || m2.restoreTab != 3 || m2.restoreFilter != "ord" {
		t.Fatalf("the tree selection must be stashed for the schema load, got %q/%d/%q",
			m2.restoreTable, m2.restoreTab, m2.restoreFilter)
	}
}

// A profile that was edited (or deleted) since the last run must not be
// silently reconnected: the picker opens instead.
func TestSessionSkipsStaleProfile(t *testing.T) {
	conn := "postgres://u:p@localhost:5432/db"
	sess := &saved.Session{ConnProfile: "prod", ConnMasked: saved.Mask(conn)}

	edited := sessionStore(t, "prod", "postgres://u:p@localhost:5433/other")
	if got := (&Model{store: edited}).restoreConn(sess); got != "" {
		t.Fatalf("an edited profile must not reconnect, got %q", got)
	}
	deleted := sessionStore(t, "", "")
	if got := (&Model{store: deleted}).restoreConn(sess); got != "" {
		t.Fatalf("a deleted profile must not reconnect, got %q", got)
	}
	unchanged := sessionStore(t, "prod", conn)
	if got := (&Model{store: unchanged}).restoreConn(sess); got != conn {
		t.Fatalf("an unchanged profile must reconnect, got %q", got)
	}
}

// An explicit connection argument always wins over the saved session.
func TestSessionYieldsToCLIConn(t *testing.T) {
	conn := "./explicit.db"
	sess := &saved.Session{ConnProfile: "prod", ConnMasked: saved.Mask("postgres://u:p@h/db")}
	m := Model{connStr: conn, store: sessionStore(t, "prod", "postgres://u:p@h/db")}
	m.applySession(sess)
	if m.connStr != conn {
		t.Fatalf("the CLI connection must win, got %q", m.connStr)
	}
}

// A page size the app no longer offers must fall back to the default
// rather than producing an unreachable page size.
func TestSessionIgnoresUnknownPageSize(t *testing.T) {
	m := browseModel(t)
	m.pageSize = 50
	m.applySession(&saved.Session{PageSize: 7})
	if m.pageSize != 50 {
		t.Fatalf("an unknown page size must be ignored, got %d", m.pageSize)
	}
	m.applySession(&saved.Session{PageSize: 100})
	if m.pageSize != 100 {
		t.Fatalf("a known page size must be applied, got %d", m.pageSize)
	}
}
