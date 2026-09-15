package tui

import (
	"path/filepath"
	"testing"

	"db-peek/internal/db"
	"db-peek/internal/saved"
)

// Switching connections must not leak the previous database's UI state:
// the detail pane, ER cache, query results and status all belong to one
// connection. Stale async replies from the old connection must be
// dropped via the connection generation.

func openMemoryDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open :memory: failed: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func localStateModel(t *testing.T) Model {
	t.Helper()
	m := browseModel(t)
	m.db = openMemoryDB(t)
	m.table = "users"
	m.cols = []db.Column{{Name: "id"}}
	m.buildTables()
	m.count = 7
	m.status = "1 tables • test (sqlite)"
	m.erSchema = erSchemaState{loaded: true, tables: []erTable{{name: "users"}}}
	m.erCache = map[string][]db.ForeignKey{}
	m.erCenter, m.erSel = "users", "users"
	m.querySample = &db.Sample{Columns: []string{"a"}}
	return m
}

func TestDisconnectClearsBrowseState(t *testing.T) {
	m := localStateModel(t)
	conn0 := m.connSeq
	m.disconnect()
	if m.screen != screenConns {
		t.Fatalf("want conns screen, got %d", m.screen)
	}
	if m.db != nil {
		t.Fatal("disconnect must close the db handle")
	}
	if m.connSeq != conn0+1 {
		t.Fatalf("disconnect must bump conn generation, got %d want %d", m.connSeq, conn0+1)
	}
	if m.table != "" || len(m.cols) != 0 || m.sample != nil || m.count != -1 {
		t.Fatalf("detail state must reset, got table=%q cols=%d count=%d", m.table, len(m.cols), m.count)
	}
	if m.erSchema.loaded || m.erCache != nil || m.erCenter != "" || m.erSel != "" {
		t.Fatalf("ER state must reset: %+v", m.erSchema)
	}
	if m.querySample != nil || m.status != "" {
		t.Fatal("query results and status must reset")
	}
}

func TestStaleConnMessagesDropped(t *testing.T) {
	m := browseModel(t)
	m.explorer = fixtureExplorer()
	m.connSeq = 5
	old := 4

	// Count reply from the old connection must not badge the new session.
	for si := range m.explorer.Schemas {
		for ti := range m.explorer.Schemas[si].Tables {
			m.explorer.Schemas[si].Tables[ti].CountOK = false
			m.explorer.Schemas[si].Tables[ti].Count = -1
		}
	}
	u, _ := m.Update(tableCountMsg{schema: "public", table: "customers", count: 777, conn: old})
	m = u.(Model)
	if tb, _ := m.explorer.tableByName("public", "customers"); tb.CountOK || tb.Count == 777 {
		t.Fatalf("stale-conn count must be dropped, got %+v", tb)
	}

	// Columns reply from the old connection must not populate the tree.
	u, _ = m.Update(columnsLoadedMsg{schema: "public", table: "orders", columns: []db.Column{{Name: "stale"}}, conn: old})
	m = u.(Model)
	if tb, _ := m.explorer.tableByName("public", "orders"); len(tb.Columns) != 2 || tb.Columns[0].Name != "id" {
		t.Fatalf("stale-conn columns must be dropped, got %+v", tb.Columns)
	}

	// Detail reply with a matching detailSeq but old conn must not apply.
	m.detailSeq = 9
	m.cols = []db.Column{{Name: "keep"}}
	u, _ = m.Update(detailLoadedMsg{table: "orders", seq: 9, cols: []db.Column{{Name: "stale"}}, conn: old})
	m = u.(Model)
	if len(m.cols) != 1 || m.cols[0].Name != "keep" {
		t.Fatalf("stale-conn detail must be dropped, got %+v", m.cols)
	}

	// ER schema reply from the old connection must not mark loaded.
	u, _ = m.Update(erSchemaLoadedMsg{tables: []erTable{{name: "stale"}}, seq: m.erSeq, conn: old})
	m = u.(Model)
	if m.erSchema.loaded {
		t.Fatal("stale-conn ER schema must be dropped")
	}
}

func TestActivateConnBumpsGenerationAndSyncsConnStr(t *testing.T) {
	dir := t.TempDir()
	s := &saved.Store{Path: filepath.Join(dir, "connections.json")}
	if err := s.Upsert("supa", "postgres://user:pass@localhost:5432/db"); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	m := browseModel(t)
	m.store = s
	m.connSeq = 3
	m.db = openMemoryDB(t) // simulate switching while connected
	m, cmd := m.activateConn("supa")
	if m.connSeq != 4 {
		t.Fatalf("activate must bump conn generation, got %d", m.connSeq)
	}
	if m.db != nil {
		t.Fatal("activate must close the previous db handle")
	}
	if m.connStr != "postgres://user:pass@localhost:5432/db" {
		t.Fatalf("activate must sync connStr, got %q", m.connStr)
	}
	if m.screen != screenBrowse || !m.loading || cmd == nil {
		t.Fatal("activate must enter loading browse with a connect cmd")
	}
}

func TestConnectSuccessResetsStaleTable(t *testing.T) {
	m := localStateModel(t)
	newDB := openMemoryDB(t)
	u, cmd := m.Update(connectMsg{db: newDB})
	m = u.(Model)
	if cmd == nil {
		t.Fatal("connect must kick off a schema load")
	}
	if m.table != "" {
		t.Fatalf("connect must clear the previous table, got %q", m.table)
	}
	if m.erSchema.loaded {
		t.Fatal("connect must clear the previous ER schema so it reloads")
	}
	if !m.loading {
		t.Fatal("connect must show loading until schemas arrive")
	}
	// A failing schema load must not resurrect the old table either.
	u, _ = m.Update(schemasLoadedMsg{err: errTestCount})
	m = u.(Model)
	if m.table != "" {
		t.Fatalf("failed schema load must leave table empty, got %q", m.table)
	}
	_ = newDB.Close()
}
