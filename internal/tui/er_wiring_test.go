package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"db-peek/internal/db"
)

// Entering the ER tab issues two commands — the per-table FK load and the
// full-schema load. They must share one cancellable context: a second
// context cancels the first, and the legacy view would keep saying
// "(no foreign keys)" forever.
func TestEnteringERTabLoadsRealForeignKeys(t *testing.T) {
	d, err := db.Open("../../ecommerce.db")
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	defer d.Close()

	m := browseModel(t)
	m = apply(t, m, connectMsg{db: d})
	m = apply(t, m, schemasLoadedMsg{
		schemas: []string{"main"},
		tables:  map[string][]db.TableRef{"main": {{Name: "categories"}, {Name: "products"}}},
	})
	if m.table == "" {
		t.Fatalf("the schema load must select a table, got %q", m.table)
	}

	m = apply(t, m, testKey("5"))
	m = applyAll(t, m, m.setTab(4))

	if !m.erSchema.loaded {
		t.Fatalf("the ER schema must load: %+v", m.erSchema)
	}
	if len(m.erSchema.links) == 0 {
		t.Fatal("the fixture declares foreign keys; none were loaded")
	}
	if len(m.erLinks) == 0 {
		t.Fatal("the per-table foreign keys must be cached: the legacy view would show (no foreign keys)")
	}
	if m.erCache[m.table] == nil {
		t.Fatalf("erCache must hold %q, got %+v", m.table, m.erCache)
	}
	if out := m.erView(100, 20); strings.Contains(out, "(no foreign keys") {
		t.Fatalf("the diagram must render real connectors:\n%s", out)
	}
}

// apply sends one message to the model and ignores the reply.
func apply(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	nm, _ := m.Update(msg)
	return nm.(Model)
}

// applyAll runs a command and feeds every message it produces back
// through Update, descending into tea.Batch.
func applyAll(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	return applyCmd(t, m, cmd, 0)
}

func applyCmd(t *testing.T, m Model, cmd tea.Cmd, depth int) Model {
	t.Helper()
	if cmd == nil || depth > 4 {
		return m
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		nm, next := m.Update(msg)
		return applyCmd(t, nm.(Model), next, depth+1)
	}
	var next tea.Cmd
	for i, sub := range batch {
		if sub == nil {
			continue
		}
		subMsg := sub()
		nm, subNext := m.Update(subMsg)
		m = nm.(Model)
		if i == 0 {
			next = subNext
		}
	}
	return applyCmd(t, m, next, depth+1)
}
