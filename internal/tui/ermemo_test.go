package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	dbpkg "db-peek/internal/db"
)

func memoModel(t *testing.T) Model {
	t.Helper()
	m := browseModel(t)
	tables, links := focusFixture()
	m.setERSchema(erSchemaState{loaded: true, tables: tables, links: links})
	m.erFocus = true
	m.erCenter = "orders"
	m.erSel = "orders"
	return m
}

// The canvas is cached across frames, so a second render of the same
// state must be identical, and any state that feeds the render must
// still change the output.
func TestERMemoRendersAreStableAndResponsive(t *testing.T) {
	m := memoModel(t)

	first := m.erView(80, 20)
	if second := m.erView(80, 20); first != second {
		t.Fatalf("repeated render must be identical:\n%q\nvs\n%q", first, second)
	}
	if m.erMemo.canvas == nil {
		t.Fatal("the canvas must be memoized after a render")
	}

	// Selection changes box styling, so it must invalidate the canvas.
	sel := m
	sel.erSel = "customers"
	if sel.erView(80, 20) == first {
		t.Fatal("a different selection must change the render")
	}

	// New schema data invalidates the memo: a new neighbour of the center
	// adds a box to the focused diagram.
	fresh := m
	tables, links := focusFixture()
	tables = append(tables, erTable{name: "extra", cols: tables[0].cols})
	links = append(links, dbpkg.ForeignKey{FromTable: "extra", FromColumn: "id", ToTable: "orders", ToColumn: "id"})
	fresh.setERSchema(erSchemaState{loaded: true, tables: tables, links: links})
	if fresh.erView(80, 20) == first {
		t.Fatal("new schema data must change the render")
	}
	// Panning must not be cached: the visible slice changes, the canvas
	// the slice is cut from does not.
	panned := memoModel(t)
	if panned.erView(80, 20) != first {
		t.Fatal("a fresh model must render the same canvas")
	}
	canvasBefore := panned.erMemo.canvas
	panned.erPanX, panned.erPanY = 3, 2
	out := panned.erView(80, 20)
	if out == first {
		t.Fatal("panning must change the visible slice")
	}
	if &panned.erMemo.canvas[0] != &canvasBefore[0] {
		t.Fatal("panning must reuse the cached canvas, not redraw it")
	}
}

// The hit-test and the renderer must never disagree: a stale memo here
// would make a rendered box unclickable.
func TestERHitAgreesWithRenderAfterPan(t *testing.T) {
	m := memoModel(t)
	m.erPanX, m.erPanY = 4, 1

	rendered := m.erView(80, 20)
	rows := strings.Split(rendered, "\n")
	hits := 0
	for y := detailTableTop + 1; y < detailTableTop+1+len(rows); y++ {
		row := rows[y-detailTableTop-1]
		for x := range 60 {
			if _, ok := m.erHit(x, y); !ok {
				continue
			}
			hits++
			// A hit must land on a row the renderer actually drew: a memo
			// that disagreed between hit-test and render would put boxes
			// under coordinates the canvas never painted.
			if strings.TrimSpace(ansi.Strip(row)) == "" {
				t.Fatalf("hit at (%d,%d) lands on a blank row:\n%s", x, y, rendered)
			}
		}
	}
	if hits == 0 {
		t.Fatalf("expected at least one box under the panned viewport:\n%s", rendered)
	}
}
