package tui

import (
	"testing"

	dbpkg "db-peek/internal/db"
	"db-peek/internal/saved"
)

func historyModel(t *testing.T) Model {
	t.Helper()
	m := queryTabModel()
	m.connStr = "postgres://u:hunter2@localhost:5432/db"
	m.history = &saved.History{Path: t.TempDir() + "/history.json"}
	return m
}

// ctrl+p walks back through this connection's queries and ctrl+n walks
// forward, restoring the live buffer past the newest entry.
func TestHistoryRecallWalksAndRestores(t *testing.T) {
	m := historyModel(t)
	m.editor.SetText("select live")
	m.history.Add("select older", saved.Mask(m.connStr))
	m.history.Add("select newest", saved.Mask(m.connStr))
	m.history.Add("select other", "some-other-conn")

	m.historyPrev()
	if got := m.editor.Text(); got != "select newest" {
		t.Fatalf("ctrl+p must load the newest entry, got %q", got)
	}
	m.historyPrev()
	if got := m.editor.Text(); got != "select older" {
		t.Fatalf("a second ctrl+p must walk further back, got %q", got)
	}
	if m.historyPrev() {
		t.Fatal("history is exhausted; ctrl+p must report failure")
	}
	if got := m.editor.Text(); got != "select older" {
		t.Fatalf("an exhausted ctrl+p must not change the buffer, got %q", got)
	}
	m.historyNext()
	if got := m.editor.Text(); got != "select newest" {
		t.Fatalf("ctrl+n must walk forward, got %q", got)
	}
	m.historyNext()
	if got := m.editor.Text(); got != "select live" {
		t.Fatalf("ctrl+n past the newest entry must restore the buffer, got %q", got)
	}
	if m.histIdx != -1 {
		t.Fatalf("restoring the buffer must end recall mode, histIdx=%d", m.histIdx)
	}
}

// Recall is per connection: another connection's history never appears.
func TestHistoryRecallIsPerConnection(t *testing.T) {
	m := historyModel(t)
	m.history.Add("secret query", "masked-other")
	if m.historyPrev() {
		t.Fatal("another connection's history must not be offered")
	}
}

// A text edit ends recall, so the next ctrl+p starts from the newest entry
// instead of a stale index.
func TestHistoryEditResetsRecall(t *testing.T) {
	m := historyModel(t)
	m.history.Add("select one", saved.Mask(m.connStr))
	m.historyPrev()
	if m.histIdx != 0 {
		t.Fatalf("expected recall mode, histIdx=%d", m.histIdx)
	}
	m.editor.Insert(' ')
	m.refreshCompletionAfterEdit()
	if m.histIdx != -1 {
		t.Fatalf("an edit must leave recall mode, histIdx=%d", m.histIdx)
	}
}

// Every run is recorded, successes and failures alike, and the write
// happens off the UI goroutine.
func TestQueryDoneRecordsHistory(t *testing.T) {
	m := historyModel(t)
	m.ensureQueryBufs()
	qid := m.qbufs[m.qcur].id
	m.querySeq = 1
	m.qbufs[m.qcur].seq = 1

	u, cmd := m.Update(queryDoneMsg{
		sql: "select 1", seq: 1, conn: m.connSeq, qbufID: qid, affected: -1,
		sample: &dbpkg.Sample{Columns: []string{"a"}, Rows: [][]string{{"1"}}},
	})
	m = u.(Model)
	if cmd == nil {
		t.Fatal("a completed query must dispatch the history write")
	}
	if msg, ok := cmd().(historySavedMsg); !ok || msg.err != nil {
		t.Fatalf("history write must succeed, got %#v", cmd())
	}
	if got := m.history.Recent(saved.Mask(m.connStr), 1); len(got) != 1 || got[0].SQL != "select 1" {
		t.Fatalf("the query must be recorded, got %+v", got)
	}

	m.qbufs[m.qcur].seq = 2
	u, _ = m.Update(queryDoneMsg{sql: "select bad", seq: 2, conn: m.connSeq, qbufID: qid, affected: -1, err: errTestCount})
	m = u.(Model)
	if got := m.history.Recent(saved.Mask(m.connStr), 1); len(got) != 1 || got[0].SQL != "select bad" {
		t.Fatalf("a failed query must still be recorded, got %+v", got)
	}
}

// A history write failure is a status note, never an error banner.
func TestHistorySaveFailureIsStatusOnly(t *testing.T) {
	m := queryTabModel()
	m.status = "ok"
	u, _ := m.Update(historySavedMsg{err: errTestCount})
	got := u.(Model)
	if got.err != "" {
		t.Fatalf("a history write failure must not set m.err, got %q", got.err)
	}
	if got.status != "history not saved: "+errTestCount.Error() {
		t.Fatalf("status = %q", got.status)
	}
}
