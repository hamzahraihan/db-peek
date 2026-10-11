package tui

import (
	"fmt"
	"strings"
	"testing"

	dbpkg "db-peek/internal/db"
)

func TestGridYankCellAndRow(t *testing.T) {
	var copied string
	old := clipboardWriteAll
	clipboardWriteAll = func(s string) error {
		copied = s
		return nil
	}
	defer func() { clipboardWriteAll = old }()

	m := browseModel(t)
	m.tab = 2
	m.focusDetail = true
	m.sample = &dbpkg.Sample{
		Columns: []string{"id", "username"},
		Rows:    [][]string{{"42", "admin"}},
		Raw:     [][]any{{int64(42), "admin"}},
	}
	m.rowTable.setData(m.sample.Columns, m.sample.Rows)
	m.rowTable.Resize(80, 20)
	m.rowTable.SetCursor(0)
	m.rowTable.SetCol(1)

	// Press 'y' to copy cell
	u, _ := m.Update(testKey("y"))
	m = u.(Model)
	if copied != "admin" {
		t.Fatalf("y must copy 'admin', got %q", copied)
	}
	if !strings.Contains(m.status, "copied username") {
		t.Fatalf("expected status confirming username copy, got %q", m.status)
	}

	// Press 'Y' to copy row as JSON
	u, _ = m.Update(testKey("Y"))
	m = u.(Model)
	if !strings.Contains(copied, `"username":"admin"`) && !strings.Contains(copied, `"username": "admin"`) {
		t.Fatalf("Y must copy row JSON, got %q", copied)
	}
	if !strings.Contains(m.status, "copied row as JSON") {
		t.Fatalf("expected status confirming row copy, got %q", m.status)
	}
}

func TestGridQuickFilterCell(t *testing.T) {
	m := browseModel(t)
	m.tab = 2
	m.focusDetail = true
	m.sample = &dbpkg.Sample{
		Columns: []string{"id", "status"},
		Rows:    [][]string{{"1", "active"}, {"2", "inactive"}, {"3", "active"}},
		Raw:     [][]any{{int64(1), "active"}, {int64(2), "inactive"}, {int64(3), "active"}},
	}
	m.rowTable.setData(m.sample.Columns, m.sample.Rows)
	m.rowTable.Resize(80, 20)
	m.rowTable.SetCursor(0)
	m.rowTable.SetCol(1)

	// Press 'F' to filter on 'active'
	u, _ := m.Update(testKey("F"))
	m = u.(Model)
	if !strings.Contains(m.status, `filtered on "active"`) {
		t.Fatalf("expected status confirming filter, got %q", m.status)
	}
	if m.rowTable.FilterText() != "active" {
		t.Fatalf("rowTable filter must be active, got %q", m.rowTable.FilterText())
	}
}

func TestQueryResultsPagination(t *testing.T) {
	m := queryTabModel()
	m.tab = 3
	m.queryFocus = 1 // results focused

	// Create 120 rows
	rows := make([][]string, 120)
	raw := make([][]any, 120)
	for i := range 120 {
		rows[i] = []string{fmt.Sprintf("%d", i)}
		raw[i] = []any{int64(i)}
	}
	m.querySample = &dbpkg.Sample{
		Columns: []string{"id"},
		Rows:    rows,
		Raw:     raw,
	}
	m.queryPageSize = 50
	m.queryPage = 0
	m.rebuildQueryPage()

	if m.queryPageCount() != 3 {
		t.Fatalf("expected 3 pages for 120 rows with pageSize 50, got %d", m.queryPageCount())
	}
	if len(m.queryTable.rows) != 50 {
		t.Fatalf("expected 50 rows on page 0, got %d", len(m.queryTable.rows))
	}

	// Press 'n' for next page
	u, _ := m.Update(testKey("n"))
	m = u.(Model)
	if m.queryPage != 1 {
		t.Fatalf("expected page 1, got %d", m.queryPage)
	}
	if len(m.queryTable.rows) != 50 {
		t.Fatalf("expected 50 rows on page 1, got %d", len(m.queryTable.rows))
	}

	// Press 'n' again for page 2 (last page, 20 rows)
	u, _ = m.Update(testKey("n"))
	m = u.(Model)
	if m.queryPage != 2 {
		t.Fatalf("expected page 2, got %d", m.queryPage)
	}
	if len(m.queryTable.rows) != 20 {
		t.Fatalf("expected 20 rows on page 2, got %d", len(m.queryTable.rows))
	}

	// Press 'p' for previous page
	u, _ = m.Update(testKey("p"))
	m = u.(Model)
	if m.queryPage != 1 {
		t.Fatalf("expected page 1, got %d", m.queryPage)
	}

	// Press 's' to cycle page size: 50 -> 100
	u, _ = m.Update(testKey("s"))
	m = u.(Model)
	if m.queryPageSize != 100 {
		t.Fatalf("expected queryPageSize 100, got %d", m.queryPageSize)
	}
	if m.queryPageCount() != 2 {
		t.Fatalf("expected 2 pages with pageSize 100, got %d", m.queryPageCount())
	}
	if len(m.queryTable.rows) != 100 {
		t.Fatalf("expected 100 rows on page 0, got %d", len(m.queryTable.rows))
	}
}
