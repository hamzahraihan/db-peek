package tui

import (
	"strings"
	"testing"
)

func TestBufferStripDemotesFromTabPill(t *testing.T) {
	m := queryTabModel()
	m.editor.SetText("SELECT 1")
	m.saveActiveBuf()
	out := m.queryStripView(120)
	activePill := activeTab.Render("[1 SELECT 1]")
	if strings.Contains(out, activePill) {
		t.Fatalf("active buffer must not wear the blue detail-tab pill, got:\n%s", out)
	}
	if !strings.Contains(out, activeBuf.Render("[1 SELECT 1]")) {
		t.Fatalf("active buffer must wear the underline chip, got:\n%s", out)
	}
	if !strings.Contains(out, "[+]") {
		t.Fatalf("strip must keep [+] affordance, got:\n%s", out)
	}
}
