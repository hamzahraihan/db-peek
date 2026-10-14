package tui

import (
	"strings"
	"testing"

	dbpkg "peekerdb/internal/db"
)

func TestCellModalOpenAndDismiss(t *testing.T) {
	m := browseModel(t)
	m.tab = 2
	m.focusDetail = true
	m.sample = &dbpkg.Sample{
		Columns: []string{"id", "payload"},
		Rows:    [][]string{{"1", `{"foo":"bar"}`}},
		Raw:     [][]any{{int64(1), `{"foo":"bar"}`}},
	}
	m.rowTable.setData(m.sample.Columns, m.sample.Rows)
	m.rowTable.Resize(80, 20)
	m.rowTable.SetCursor(0)
	m.rowTable.SetCol(1)

	// Press 'v' to open modal
	u, _ := m.Update(testKey("v"))
	m = u.(Model)
	if !m.cellModal {
		t.Fatal("v must open cell modal")
	}
	if m.cellModalCol != "payload" {
		t.Fatalf("expected col payload, got %q", m.cellModalCol)
	}
	if !strings.Contains(m.cellModalVal, `"foo": "bar"`) {
		t.Fatalf("expected formatted JSON, got %q", m.cellModalVal)
	}

	// Press 'esc' to close modal
	u, _ = m.Update(testKey("esc"))
	m = u.(Model)
	if m.cellModal {
		t.Fatal("esc must close cell modal")
	}
}

func TestFormatCellRaw(t *testing.T) {
	if got := formatCellRaw(nil); got != "(NULL)" {
		t.Fatalf("formatCellRaw(nil) = %q, want (NULL)", got)
	}
	jsonStr := `{"a":1}`
	got := formatCellRaw(jsonStr)
	if !strings.Contains(got, "  \"a\": 1") {
		t.Fatalf("expected pretty json, got %q", got)
	}
}
