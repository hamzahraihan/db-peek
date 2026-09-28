package tui

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dbpkg "db-peek/internal/db"
)

func exportModel(t *testing.T) Model {
	t.Helper()
	m := browseModel(t)
	m.focusDetail = true
	m.tab = 2
	m.table = "orders"
	m.sample = &dbpkg.Sample{
		Columns: []string{"id", "note"},
		Rows:    [][]string{{"1", strings.Repeat("y", 80) + "..."}, {"2", "NULL"}},
		Raw: [][]any{
			{int64(1), strings.Repeat("y", 80)},
			{int64(2), nil},
		},
	}
	m.buildRowTable()
	m.resizeBrowse()
	return m
}

// E opens the prompt with a pre-filled path, and the prompt takes over
// the pane's status line without changing the row count above it.
func TestExportOpensPrefilledPrompt(t *testing.T) {
	m := exportModel(t)
	before := strings.Count(m.detailView(), "\n")

	u, _ := m.Update(testKey("E"))
	m = u.(Model)
	if !m.exporting {
		t.Fatal("E must open the export prompt")
	}
	if got := m.exportInput.Value(); !strings.HasSuffix(got, ".csv") || !strings.Contains(got, "orders") {
		t.Fatalf("prompt must be pre-filled with a table path, got %q", got)
	}
	if after := strings.Count(m.detailView(), "\n"); after != before {
		t.Fatalf("the prompt must not shift the pane: %d rows before, %d after", before, after)
	}
	if !strings.Contains(m.detailView(), "export") {
		t.Fatalf("the prompt must be visible:\n%s", m.detailView())
	}
}

// enter writes the untruncated values; the grid's 60-char display cut
// must not reach the file.
func TestExportWritesUntruncatedRows(t *testing.T) {
	m := exportModel(t)
	path := filepath.Join(t.TempDir(), "out.csv")

	u, _ := m.Update(testKey("E"))
	m = u.(Model)
	m.exportInput.SetValue(path)
	u, _ = m.Update(testKey("enter"))
	m = u.(Model)

	if m.exporting {
		t.Fatal("enter must close the prompt")
	}
	if m.err != "" {
		t.Fatalf("export failed: %s", m.err)
	}
	if !strings.HasPrefix(m.status, "exported 2 rows") {
		t.Fatalf("status = %q", m.status)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	recs, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("header + 2 rows expected, got %d", len(recs))
	}
	if recs[0][0] != "id" || recs[0][1] != "note" {
		t.Fatalf("header must be the grid's columns, got %v", recs[0])
	}
	if len(recs[1][1]) != 80 {
		t.Fatalf("the export must keep the full 80 characters, got %d", len(recs[1][1]))
	}
	if recs[2][1] != "" {
		t.Fatalf("SQL NULL must export as an empty field, got %q", recs[2][1])
	}
}

// esc cancels without writing anything.
func TestExportEscCancels(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.csv")
	m := exportModel(t)

	u, _ := m.Update(testKey("E"))
	m = u.(Model)
	m.exportInput.SetValue(path)
	u, _ = m.Update(testKey("esc"))
	m = u.(Model)

	if m.exporting {
		t.Fatal("esc must close the prompt")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("esc must not write a file (stat err = %v)", err)
	}
}

// An unsupported extension is refused before anything is created.
func TestExportRejectsUnknownExtension(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.xml")
	m := exportModel(t)

	u, _ := m.Update(testKey("E"))
	m = u.(Model)
	m.exportInput.SetValue(path)
	u, _ = m.Update(testKey("enter"))
	m = u.(Model)

	if !strings.Contains(m.err, ".csv") {
		t.Fatalf("the error must name the supported formats, got %q", m.err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("nothing must be written for a bad extension (stat err = %v)", err)
	}
}

// The query tab exports querySample, not the table behind it.
func TestExportFromQueryResults(t *testing.T) {
	m := queryTabModel()
	m.queryFocus = 1
	m.querySample = &dbpkg.Sample{
		Columns: []string{"n"},
		Rows:    [][]string{{"7"}},
		Raw:     [][]any{{int64(7)}},
	}
	path := filepath.Join(t.TempDir(), "q.json")

	u, _ := m.Update(testKey("E"))
	m = u.(Model)
	if !strings.HasSuffix(m.exportInput.Value(), ".json") {
		t.Fatalf("query results default to JSON, got %q", m.exportInput.Value())
	}
	m.exportInput.SetValue(path)
	u, _ = m.Update(testKey("enter"))
	m = u.(Model)
	if m.err != "" {
		t.Fatalf("export failed: %s", m.err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"n": 7`) {
		t.Fatalf("query export must carry the raw value:\n%s", data)
	}
}
