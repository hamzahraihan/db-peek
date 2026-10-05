package tui

import (
	"strings"
	"testing"
)

func TestSchemaDDLEchoAndCopy(t *testing.T) {
	old := clipboardWriteAll
	var got string
	clipboardWriteAll = func(s string) error { got = s; return nil }
	defer func() { clipboardWriteAll = old }()

	m := queryTabModel()
	m.tab, m.focusDetail = 0, true
	m.width, m.height = 120, 40
	m.resizeBrowse()
	m.tableDDL = "CREATE TABLE users (\n  id integer\n);"
	view := m.detailView()
	if !strings.Contains(view, "CREATE TABLE users") {
		t.Fatalf("schema tab must echo table DDL, got:\n%s", view)
	}
	u, _ := m.Update(testKey("d"))
	m = u.(Model)
	if !strings.Contains(got, "CREATE TABLE users") {
		t.Fatalf("d must copy full DDL, got %q", got)
	}
}
