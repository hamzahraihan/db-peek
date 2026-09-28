package saved

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tempHistory(t *testing.T) *History {
	t.Helper()
	return &History{Path: filepath.Join(t.TempDir(), "history.json")}
}

func TestHistoryAddDropsConsecutiveDuplicate(t *testing.T) {
	h := tempHistory(t)
	h.Add("select 1", "a")
	h.Add("select 2", "a")
	h.Add("select 2", "a") // same as the newest entry
	if h.Len() != 2 {
		t.Fatalf("a consecutive duplicate must be dropped, got %d entries", h.Len())
	}
	if got := h.Recent("a", 5); len(got) != 2 || got[0].SQL != "select 2" || got[1].SQL != "select 1" {
		t.Fatalf("want newest-first distinct queries, got %+v", got)
	}
	h.Add("select 1", "a") // non-consecutive repeat is kept
	if h.Len() != 3 {
		t.Fatalf("a non-consecutive repeat must be kept, got %d", h.Len())
	}
}

func TestHistoryAddCapsAtLimit(t *testing.T) {
	h := tempHistory(t)
	for i := range HistoryLimit + 25 {
		h.Add("select "+strings.Repeat("x", i), "a")
	}
	if h.Len() != HistoryLimit {
		t.Fatalf("history must cap at %d, got %d", HistoryLimit, h.Len())
	}
	// The newest entry survives the cap.
	if got := h.Recent("a", 1); len(got) != 1 || got[0].SQL != "select "+strings.Repeat("x", HistoryLimit+24) {
		t.Fatalf("the newest entry must survive the cap, got %+v", got)
	}
}

func TestHistoryRecentIsPerConnection(t *testing.T) {
	h := tempHistory(t)
	h.Add("from a", "masked-a")
	h.Add("from b", "masked-b")
	h.Add("from a2", "masked-a")
	got := h.Recent("masked-a", 2)
	if len(got) != 2 {
		t.Fatalf("want 2 entries for masked-a, got %+v", got)
	}
	if got[0].SQL != "from a2" || got[1].SQL != "from a" {
		t.Fatalf("want newest-first for this connection, got %+v", got)
	}
	if other := h.Recent("masked-b", 5); len(other) != 1 || other[0].SQL != "from b" {
		t.Fatalf("connections must not mix, got %+v", other)
	}
	if empty := h.Recent("masked-z", 5); len(empty) != 0 {
		t.Fatalf("an unknown connection has no history, got %+v", empty)
	}
}

// history.json must never hold a password: callers pass a masked conn, and
// the file must be created 0600 like the other stores.
func TestHistorySaveKeepsConnMasked(t *testing.T) {
	h := tempHistory(t)
	h.Add("select 1", Mask("postgres://u:hunter2@localhost:5432/db"))
	if err := h.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(h.Path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "hunter2") {
		t.Fatalf("history.json must not hold the password:\n%s", data)
	}
	if !strings.Contains(string(data), Mask("postgres://u:hunter2@localhost:5432/db")) {
		t.Fatalf("history.json should hold the masked conn:\n%s", data)
	}
}

func TestHistorySaveRoundTrips(t *testing.T) {
	h := tempHistory(t)
	h.Add("select 1", "a")
	if err := h.Save(); err != nil {
		t.Fatal(err)
	}
	loaded := &History{Path: h.Path}
	data, err := os.ReadFile(h.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &loaded.entries); err != nil {
		t.Fatal(err)
	}
	if got := loaded.Recent("a", 1); len(got) != 1 || got[0].SQL != "select 1" {
		t.Fatalf("history must round-trip, got %+v", got)
	}
}
