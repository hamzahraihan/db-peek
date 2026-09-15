package tui

// ER diagram tab (tab 4): a text diagram of the current table's foreign
// keys. Outgoing links (this table references another) render on the left,
// incoming links (another table references this one) on the right, with
// the current table's columns in the center. Neighbor names are clickable
// (see erHit) and jump to that table's schema via inspectTable.

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// erSpan is one clickable neighbor name within the rendered diagram:
// content line y (0-based into the unscrolled layout) and cell range
// [x0, x1) of the name on that line.
type erSpan struct {
	y      int
	x0, x1 int
	name   string
}

// erLayout builds the diagram lines for the current table, clipped to
// innerW cells each (1-line width-capped rows), plus the clickable
// neighbor spans. Center lines carry no spans (center excluded).
func (m Model) erLayout(innerW int) ([]string, []erSpan) {
	fkCols := map[string]bool{}
	for _, fk := range m.erLinks {
		if fk.FromTable == m.table {
			fkCols[fk.FromColumn] = true
		}
	}

	// Center box: table name header + one row per column with the
	// explorer icon rules (PK from Extra prefix, FK from _id suffix
	// or the FK column set, else plain).
	lines := []string{m.table}
	for _, c := range m.cols {
		icon := "◇"
		switch {
		case strings.HasPrefix(c.Extra, "PK"):
			icon = "🔑"
		case strings.HasSuffix(c.Name, "_id") || fkCols[c.Name]:
			icon = "➤"
		}
		lines = append(lines, "  "+icon+" "+c.Name)
	}
	centerCount := len(lines)

	if len(m.erLinks) == 0 {
		lines = append(lines, dimStyle.Render(fitText("(no foreign keys)", innerW)))
		out := make([]string, len(lines))
		for i, ln := range lines {
			if i < centerCount {
				out[i] = fitText(ln, innerW)
			} else {
				out[i] = ln // already fitted styled line
			}
		}
		return out, nil
	}

	// Distinct neighbors preserving first-seen order, with their
	// from→to column labels joined per table.
	var outTables []string
	outLabels := map[string][]string{}
	for _, fk := range m.erLinks {
		if fk.FromTable != m.table {
			continue
		}
		if _, ok := outLabels[fk.ToTable]; !ok {
			outTables = append(outTables, fk.ToTable)
		}
		outLabels[fk.ToTable] = append(outLabels[fk.ToTable], fk.FromColumn+"→"+fk.ToColumn)
	}
	var inTables []string
	inLabels := map[string][]string{}
	for _, fk := range m.erLinks {
		if fk.ToTable != m.table {
			continue
		}
		if _, ok := inLabels[fk.FromTable]; !ok {
			inTables = append(inTables, fk.FromTable)
		}
		inLabels[fk.FromTable] = append(inLabels[fk.FromTable], fk.FromColumn+"→"+fk.ToColumn)
	}

	// Neighbor spans are tracked structurally while building the
	// connector rows (start cell + name cell width per row) instead of
	// re-parsing rendered text, so quoted/spaced/wide identifiers hit
	// correctly. Outgoing rows ("<left> ──▶ <center>") span the left
	// table at cell 0; incoming rows ("<center> ◀── <right>") span the
	// right table after the "<center> ◀── " prefix.
	type erMeta struct {
		name      string
		x0, nameW int
	}
	var metas []erMeta
	for _, to := range outTables {
		lines = append(lines, to+" ──▶ "+m.table+" "+strings.Join(outLabels[to], ", "))
		metas = append(metas, erMeta{name: to, x0: 0, nameW: lipgloss.Width(to)})
	}
	for _, from := range inTables {
		prefix := m.table + " ◀── "
		lines = append(lines, prefix+from+" "+strings.Join(inLabels[from], ", "))
		metas = append(metas, erMeta{name: from, x0: lipgloss.Width(prefix), nameW: lipgloss.Width(from)})
	}

	// Clip to innerW, intersecting each structural span with the visible
	// prefix (fitText keeps the first innerW-3 cells + "..." on overflow,
	// so a name truncated away yields no span).
	var spans []erSpan
	out := make([]string, len(lines))
	for i, ln := range lines {
		clipped := fitText(ln, innerW)
		out[i] = clipped
		if i < centerCount {
			continue
		}
		meta := metas[i-centerCount]
		if meta.name == "" || meta.nameW <= 0 {
			continue
		}
		x0, x1 := meta.x0, meta.x0+meta.nameW
		if innerW > 0 && lipgloss.Width(ln) > innerW {
			bound := innerW - 3
			if innerW <= 3 {
				continue // clipped to "...": no name cells visible
			}
			if x0 >= bound {
				continue
			}
			if x1 > bound {
				x1 = bound
			}
		}
		spans = append(spans, erSpan{y: i, x0: x0, x1: x1, name: meta.name})
	}
	return out, spans
}

// erClampOff bounds the scroll offset to the layout, shared by erView
// and erHit so hits always track what is on screen.
func (m Model) erClampOff(n int) int {
	off := m.erOffset
	if off < 0 {
		off = 0
	}
	if off > n-1 {
		off = n - 1
	}
	if off < 0 {
		off = 0
	}
	return off
}

// erView renders the diagram clipped to innerW columns and innerH rows.
// Loaded schemas show the focused 1-hop view by default (f toggles the
// full-schema grid); unloaded schemas fall back to the legacy text view.
func (m Model) erView(innerW, innerH int) string {
	if m.erSchema.loaded {
		if m.erIsFocused() {
			return m.erFocusedCanvasView(innerW, innerH)
		}
		return m.erCanvasView(innerW, innerH)
	}
	return m.erLegacyView(innerW, innerH)
}

func (m Model) erLegacyView(innerW, innerH int) string {
	lines, _ := m.erLayout(innerW)
	if innerH < 1 {
		innerH = 1
	}
	off := m.erClampOff(len(lines))
	end := off + innerH
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[off:end], "\n")
}

// erHit maps viewport coords to the topmost box; connectors never hit.
// Focused mode accounts for its `diagram · X` header row.
func (m Model) erHit(x, y int) (string, bool) {
	if !m.erSchema.loaded {
		return m.erLegacyHit(x, y)
	}
	if m.erIsFocused() {
		return m.erFocusedHit(x, y)
	}
	pos := erGridLayout(m.erSchema.tables, m.paneInnerW(), m.paneInnerH())
	cx := m.erPanX + x
	cy := (y - detailTableTop) + m.erPanY
	for name, r := range pos {
		if cx >= r.x && cx < r.x+r.w && cy >= r.y && cy < r.y+r.h {
			return name, true
		}
	}
	return "", false
}

// erLegacyHit maps border-relative content coords (x = content column,
// y = terminal row) to a neighbor table name. The center box is
// excluded: only connector-row neighbor spans hit.
func (m Model) erLegacyHit(x, y int) (string, bool) {
	lines, spans := m.erLayout(m.paneInnerW())
	rel := m.erClampOff(len(lines)) + (y - detailTableTop)
	if rel < 0 {
		return "", false
	}
	for _, s := range spans {
		if s.y != rel {
			continue
		}
		if x >= s.x0 && x < s.x1 {
			return s.name, true
		}
	}
	return "", false
}
