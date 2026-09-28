package saved

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// HistoryLimit caps the persisted history; recall only ever walks the
// most recent entries, so an unbounded file would grow forever.
const HistoryLimit = 200

// HistoryEntry is one executed query. Conn holds the Mask()ed connection
// string: it groups history per connection and never carries a password.
type HistoryEntry struct {
	SQL  string    `json:"sql"`
	Conn string    `json:"conn"`
	At   time.Time `json:"at"`
}

// History is the persisted query history, newest first.
type History struct {
	Path    string
	entries []HistoryEntry
}

func DefaultHistoryPath() (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "history.json"), nil
}

// LoadHistory reads the history file. A missing file is an empty history,
// not an error: history is a convenience, never a startup requirement.
func LoadHistory() (*History, error) {
	p, err := DefaultHistoryPath()
	if err != nil {
		return &History{}, err
	}
	h := &History{Path: p}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return h, nil
		}
		return h, err
	}
	if err := json.Unmarshal(data, &h.entries); err != nil {
		return &History{Path: p}, err
	}
	return h, nil
}

// Add records a query, newest first. Re-running the same query twice in a
// row is a no-op; a non-consecutive repeat is kept, so ctrl+p still walks
// back through the distinct statements.
func (h *History) Add(sql, maskedConn string) {
	sql = trimSQL(sql)
	if sql == "" {
		return
	}
	next := make([]HistoryEntry, 0, len(h.entries)+1)
	next = append(next, HistoryEntry{SQL: sql, Conn: maskedConn, At: time.Now().UTC()})
	next = append(next, h.entries...)
	if len(next) > 1 && next[1].SQL == sql && next[1].Conn == maskedConn {
		next = append(next[:1], next[2:]...)
	}
	if len(next) > HistoryLimit {
		next = next[:HistoryLimit]
	}
	h.entries = next
}

// Recent returns up to n of this connection's entries, newest first.
func (h *History) Recent(maskedConn string, n int) []HistoryEntry {
	if n <= 0 {
		return nil
	}
	var out []HistoryEntry
	for _, e := range h.entries {
		if e.Conn != maskedConn {
			continue
		}
		out = append(out, e)
		if len(out) == n {
			break
		}
	}
	return out
}

// Len reports how many entries are stored, across all connections.
func (h *History) Len() int { return len(h.entries) }

func (h *History) persist() error {
	data, err := json.MarshalIndent(h.entries, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(h.Path, append(data, '\n'), 0o600)
}

// Save writes the history atomically. Callers run it off the UI goroutine.
func (h *History) Save() error { return h.persist() }

func trimSQL(sql string) string {
	for len(sql) > 0 && (sql[len(sql)-1] == '\n' || sql[len(sql)-1] == ' ' || sql[len(sql)-1] == '\t') {
		sql = sql[:len(sql)-1]
	}
	for len(sql) > 0 && (sql[0] == ' ' || sql[0] == '\t' || sql[0] == '\n') {
		sql = sql[1:]
	}
	return sql
}
