package tui

// ER diagram tab (tab 4): a text diagram of the current table's foreign
// keys. Outgoing links (this table references another) render on the left,
// incoming links (another table references this one) on the right, with
// the current table's columns in the center. Neighbor names are clickable
// (see erHit) and jump to that table's schema via inspectTable.

import (
	"strings"
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

	for _, to := range outTables {
		lines = append(lines, to+" ──▶ "+m.table+" "+strings.Join(outLabels[to], ", "))
	}
	for _, from := range inTables {
		lines = append(lines, m.table+" ◀── "+from+" "+strings.Join(inLabels[from], ", "))
	}

	// Clip to innerW and record neighbor spans on the connector rows.
	// Outgoing rows ("<left> ──▶ <center>") span the left table;
	// incoming rows ("<center> ◀── <right>") span the right table.
	var spans []erSpan
	out := make([]string, len(lines))
	for i, ln := range lines {
		clipped := fitText(ln, innerW)
		out[i] = clipped
		if i < centerCount {
			continue
		}
		switch {
		case strings.Contains(ln, "──▶"):
			name := strings.TrimSpace(strings.SplitN(ln, "──▶", 2)[0])
			if x := strings.Index(clipped, name); x >= 0 && name != "" {
				spans = append(spans, erSpan{y: i, x0: x, x1: x + len(name), name: name})
			}
		case strings.Contains(ln, "◀──"):
			rest := strings.SplitN(ln, "◀──", 2)[1]
			fields := strings.Fields(rest)
			if len(fields) == 0 {
				continue
			}
			name := fields[0]
			// The center name sorts first on the line; the neighbor
			// sits after the arrow, so search from there.
			x := -1
			if ax := strings.Index(clipped, "◀──"); ax >= 0 {
				if nx := strings.Index(clipped[ax:], name); nx >= 0 {
					x = ax + nx
				}
			}
			if x >= 0 {
				spans = append(spans, erSpan{y: i, x0: x, x1: x + len(name), name: name})
			}
		}
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

// erView renders the diagram clipped to innerW columns and innerH rows
// with vertical scroll via erOffset (clamped here so keys need no bounds
// checks).
func (m Model) erView(innerW, innerH int) string {
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

// erHit maps border-relative content coords (x = content column,
// y = terminal row) to a neighbor table name. The center box is
// excluded: only connector-row neighbor spans hit.
func (m Model) erHit(x, y int) (string, bool) {
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
