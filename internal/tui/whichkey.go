package tui

// Which-key help overlay: a centered registry of every handled key,
// grouped into five canonical sections (Global, Sidebar, Query Editor,
// Results, Connections) with one 2-column key/desc table per section,
// flowed into two side-by-side columns to stay compact. helpRect is
// shared by render (helpView) and mouse hit-testing so the two can
// never drift.

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// KeyBinding is one handled key (slash-separated aliases) with its
// description and the comma-separated contexts it applies in.
type KeyBinding struct {
	Key      string
	Desc     string
	Contexts string
}

var keyRegistry = []KeyBinding{
	{"up/k", "move up", "Sidebar,Detail,Query-results,ER"},
	{"down/j", "move down", "Sidebar,Detail,Query-results,ER"},
	{"f", "focused/all tables", "ER"},
	{"n/p", "select box", "ER"},
	{"enter", "recenter diagram", "ER"},
	{"left/right", "collapse/expand", "Sidebar"},
	{"enter", "preview table", "Sidebar"},
	{"/", "filter tables (enter keeps, esc clears)", "Sidebar"},
	{"r", "refresh", "Sidebar,Detail"},
	{"c/esc", "back to connections", "Sidebar"},
	{"q", "quit", "Global"},
	{"tab/shift+tab", "switch pane", "Global"},
	{"h/l", "prev/next tab", "Detail"},
	{"esc/backspace", "back to sidebar", "Detail"},
	{"home/end", "top/bottom of grid", "Detail"},
	{"1/2/3/4/5", "schema/indexes/rows/query/er tab", "Detail"},
	{"n/p", "next/prev rows page", "Detail"},
	{"s", "cycle page size", "Detail"},
	{"pgup/pgdown", "page grid", "Detail"},
	{"ctrl+u/ctrl+d", "half-page grid", "Detail"},
	{"g/G", "top/bottom of grid", "Detail"},
	{"ctrl+r/F5", "run query", "Query editor"},
	{"ctrl+s", "copy query+error", "Query editor"},
	{"esc", "editor to results", "Query editor"},
	{"shift+up/down", "select block", "Query editor"},
	{"ctrl+d", "delete line/block", "Query editor"},
	{"ctrl+/", "toggle -- comment", "Query editor"},
	{"ctrl+t", "new query buffer", "Query editor"},
	{"ctrl+w", "close query buffer", "Query editor"},
	{"H/L", "prev/next query buffer", "Query-results"},
	{"t", "new query buffer", "Query-results"},
	{"X", "close query buffer", "Query-results"},
	{"?", "this help", "Global"},
	{"a/e/d", "add/edit/forget connection", "Connections"},
	{"enter", "connect", "Connections"},
}

// helpContextOrder is the canonical display order. Every binding folds
// into exactly these five sections so headers never repeat.
var helpContextOrder = []string{"Global", "Sidebar", "Query Editor", "Results", "Connections"}

// normalizeHelpContext maps legacy registry tokens (Detail, Query-results,
// ER, Query editor) onto the five canonical sections.
func normalizeHelpContext(tok string) string {
	switch strings.ToLower(strings.TrimSpace(tok)) {
	case "global":
		return "Global"
	case "sidebar":
		return "Sidebar"
	case "query editor":
		return "Query Editor"
	case "detail", "query-results", "er":
		return "Results"
	case "connections":
		return "Connections"
	default:
		return strings.TrimSpace(tok)
	}
}

// helpGroup is one canonical section with its (key, action) rows in
// registry order.
type helpGroup struct {
	header string
	rows   []KeyBinding
}

// helpGroups folds the registry into canonical context groups, preserving
// the registry's first-occurrence row order within each group. A binding
// with comma-separated contexts appears once under each mapped section;
// each header appears at most once, in helpContextOrder.
func helpGroups() []helpGroup {
	byHeader := map[string]*helpGroup{}
	for _, h := range helpContextOrder {
		byHeader[h] = &helpGroup{header: h}
	}
	seen := map[string]map[string]bool{} // header -> "key\x00desc" set
	for _, b := range keyRegistry {
		for _, tok := range strings.Split(b.Contexts, ",") {
			h := normalizeHelpContext(tok)
			g, ok := byHeader[h]
			if !ok {
				g = &helpGroup{header: h}
				byHeader[h] = g
			}
			key := b.Key + "\x00" + b.Desc
			if seen[h] == nil {
				seen[h] = map[string]bool{}
			}
			if seen[h][key] {
				continue
			}
			seen[h][key] = true
			g.rows = append(g.rows, KeyBinding{Key: b.Key, Desc: b.Desc})
		}
	}
	var groups []helpGroup
	for _, h := range helpContextOrder {
		if len(byHeader[h].rows) > 0 {
			groups = append(groups, *byHeader[h])
		}
	}
	// Any non-canonical headers (future contexts) append in first-seen order.
	for h, g := range byHeader {
		known := false
		for _, ch := range helpContextOrder {
			if ch == h {
				known = true
				break
			}
		}
		if !known && len(g.rows) > 0 {
			groups = append(groups, *g)
		}
	}
	return groups
}

// helpContentLines renders the overlay body: one aligned 2-column table
// (yellow header, cyan keys padded to the section's widest key, muted
// descriptions) per canonical section. Sections flow into two side-by-side
// columns — split at the prefix boundary that best balances heights — so
// the box stays compact and the UI behind it remains visible. On narrow
// terminals it falls back to a single stacked column. A title anchors the
// top and a `?/esc` footer anchors the bottom; content width is capped to
// the terminal so the box never overflows.
func (m Model) helpContentLines() []string {
	maxW := m.width - 6 // border + margin
	if m.width <= 0 {
		maxW = 1 << 30 // unknown width: no cap
	}
	groups := helpGroups()
	keyWs := make([]int, len(groups))
	for gi, g := range groups {
		for _, r := range g.rows {
			if w := lipgloss.Width(r.Key); w > keyWs[gi] {
				keyWs[gi] = w
			}
		}
	}
	// Section heights (header + rows); pick the prefix split that best
	// balances the two columns.
	heights := make([]int, len(groups))
	total := 0
	for i, g := range groups {
		heights[i] = 1 + len(g.rows)
		total += heights[i]
	}
	split, best := 0, total
	for k := 1; k < len(groups); k++ {
		left := 0
		for _, h := range heights[:k] {
			left += h
		}
		if taller := max(left, total-left); taller < best {
			best, split = taller, k
		}
	}
	lines := []string{helpTitleStyle.Render("Keybindings")}
	renderCol := func(list []int, budget int) (blk []string, w int) {
		for _, gi := range list {
			blk = append(blk, helpHeaderStyle.Render(groups[gi].header))
			if hw := lipgloss.Width(groups[gi].header); hw > w {
				w = hw
			}
			for _, r := range groups[gi].rows {
				row := helpPairRow(r.Key, r.Desc, keyWs[gi], budget)
				blk = append(blk, row)
				if rw := lipgloss.Width(row); rw > w {
					w = rw
				}
			}
		}
		return blk, w
	}
	if split > 0 {
		const gutter = 4
		budget := (maxW - gutter) / 2
		if budget > 0 {
			idxA, idxB := make([]int, 0, split), make([]int, 0, len(groups)-split)
			for i := range groups {
				if i < split {
					idxA = append(idxA, i)
				} else {
					idxB = append(idxB, i)
				}
			}
			colA, wA := renderCol(idxA, budget)
			colB, wB := renderCol(idxB, budget)
			if wA+wB+gutter <= maxW {
				for i := 0; i < max(len(colA), len(colB)); i++ {
					a, b := "", ""
					if i < len(colA) {
						a = colA[i]
					}
					if i < len(colB) {
						b = colB[i]
					}
					if aw := lipgloss.Width(a); aw < wA {
						a += strings.Repeat(" ", wA-aw)
					}
					if b != "" {
						if bw := lipgloss.Width(b); bw < wB {
							b += strings.Repeat(" ", wB-bw)
						}
					}
					if b == "" {
						lines = append(lines, a)
					} else {
						lines = append(lines, a+strings.Repeat(" ", gutter)+b)
					}
				}
				lines = append(lines, helpFooter(maxW)...)
				return lines
			}
		}
	}
	// Narrow fallback (or a single section): one stacked column.
	idx := make([]int, 0, len(groups))
	for i := range groups {
		idx = append(idx, i)
	}
	col, _ := renderCol(idx, maxW)
	lines = append(lines, col...)
	lines = append(lines, helpFooter(maxW)...)
	return lines
}

// helpFooter renders the anchored dismissal hint with a separator above it.
func helpFooter(maxW int) []string {
	sepW := lipgloss.Width("? / esc to close")
	if maxW > 0 && sepW > maxW {
		sepW = maxW
	}
	return []string{
		dimStyle.Render(strings.Repeat("─", sepW)),
		dimStyle.Render("? / esc to close"),
	}
}

// helpPairRow renders one 2-column row: cyan key padded to keyW cells,
// two-space gutter, muted description (truncated to fit maxW).
func helpPairRow(key, desc string, keyW, maxW int) string {
	padded := key + strings.Repeat(" ", max(0, keyW-lipgloss.Width(key)))
	descW := maxW - keyW - 2
	if descW < 0 {
		descW = 0
	}
	if maxW < 1<<30 && lipgloss.Width(desc) > descW {
		desc = fitText(desc, descW)
	}
	return helpKeyStyle.Render(padded) + "  " + helpDescStyle.Render(desc)
}

// helpBox renders the overlay box: content in a rounded gold border.
func (m Model) helpBox() string {
	lines := m.helpContentLines()
	if m.height > 0 && len(lines)+2 > m.height {
		lines = lines[:m.height-2] // terminal-capped, top-anchored
	}
	return paneBorder(true).Render(strings.Join(lines, "\n"))
}

// helpRect is the overlay rect in terminal cells, shared by render and
// mouse hit-testing. Derived from the rendered box so the two agree by
// construction.
func (m Model) helpRect() (x, y, w, h int) {
	lines := strings.Split(m.helpBox(), "\n")
	h = len(lines)
	for _, ln := range lines {
		if ww := lipgloss.Width(ln); ww > w {
			w = ww
		}
	}
	if m.width > 0 && w > m.width {
		w = m.width
	}
	if m.height > 0 && h > m.height {
		h = m.height
	}
	if m.width > w {
		x = (m.width - w) / 2
	}
	if m.height > h {
		y = (m.height - h) / 2
	}
	return x, y, w, h
}

// helpView renders the current screen with the help overlay centered on
// top. Each overlay line is spliced into its base line at the centered
// rect, so the UI beside the box — including its styling — survives; only
// the cells the box covers are replaced. Widths never exceed the terminal.
func (m Model) helpView() string {
	var base string
	switch m.screen {
	case screenConns:
		base = m.connsView()
	case screenForm:
		base = m.formView()
	default:
		base = m.browseView()
	}
	baseLines := strings.Split(base, "\n")
	boxLines := strings.Split(m.helpBox(), "\n")
	x, y, w, h := m.helpRect()
	termW := m.width
	if termW <= 0 {
		termW = w
	}
	for i := 0; i < h && i < len(boxLines) && y+i < len(baseLines); i++ {
		baseLines[y+i] = spliceCells(baseLines[y+i], boxLines[i], x, w, termW)
	}
	return strings.Join(baseLines, "\n")
}
