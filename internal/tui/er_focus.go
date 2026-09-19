package tui

import (
	"sort"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	dbpkg "db-peek/internal/db"
)

// erIsFocused reports whether the diagram should show the 1-hop focused
// view around erCenter. New models default erFocus=true but erCenter is
// empty until the ER tab is entered or schema loads, so legacy tests and
// the unloaded state keep the full/legacy paths.
func (m Model) erIsFocused() bool {
	return m.erFocus && m.erCenter != ""
}

// erFocusedSubset filters full-schema tables/links to the 1-hop
// neighborhood around center: center plus any table directly linked in
// either direction. Order preserves the input for stability; layout
// sorts per column. Links are pruned to both-ends-visible. If center is
// missing from tables (stale after reload), the full set is returned so
// the diagram never goes blank.
func erFocusedSubset(center string, tables []erTable, links []dbpkg.ForeignKey) ([]erTable, []dbpkg.ForeignKey) {
	if center == "" {
		return tables, links
	}
	byName := map[string]erTable{}
	for _, t := range tables {
		byName[t.name] = t
	}
	if _, ok := byName[center]; !ok {
		return tables, links
	}
	keep := map[string]bool{center: true}
	for _, l := range links {
		if l.FromTable == center {
			keep[l.ToTable] = true
		}
		if l.ToTable == center {
			keep[l.FromTable] = true
		}
	}
	var ft []erTable
	for _, t := range tables {
		if keep[t.name] {
			ft = append(ft, t)
		}
	}
	var fl []dbpkg.ForeignKey
	for _, l := range links {
		if keep[l.FromTable] && keep[l.ToTable] {
			fl = append(fl, l)
		}
	}
	return ft, fl
}

// erVisibleTables returns the tables currently on screen: the focused
// subset when focused, otherwise the full schema. Used by n/p cycling.
func (m Model) erVisibleTables() []erTable {
	if m.erIsFocused() {
		ft, _ := erFocusedSubset(m.erCenter, m.erSchema.tables, m.erSchema.links)
		return ft
	}
	return m.erSchema.tables
}

// erFocusLayout places boxes in three columns: incoming (references
// center) left, center middle, outgoing (referenced by center) right.
// Each column stacks vertically and columns are vertically centered
// against the tallest one so 1-hop connectors stay short and horizontal.
// A table linked in both directions goes right (outgoing wins). Gap
// gx=6 leaves room for `──▶` arrows, gy=2 separates stacked boxes.
func erFocusLayout(center string, tables []erTable, links []dbpkg.ForeignKey) map[string]erRect {
	byName := map[string]erTable{}
	for _, t := range tables {
		byName[t.name] = t
	}
	if _, ok := byName[center]; !ok {
		// Missing center: single vertical stack, sorted, so nothing is hidden.
		cp := append([]erTable(nil), tables...)
		sort.Slice(cp, func(i, j int) bool { return cp[i].name < cp[j].name })
		pos := map[string]erRect{}
		y := 0
		for _, t := range cp {
			w := erBoxWidth(t) + 2
			h := erBoxHeight(t, len(tables)) + 2
			pos[t.name] = erRect{x: 0, y: y, w: w, h: h}
			y += h + 2
		}
		return pos
	}
	outgoingSet := map[string]bool{}
	incomingSet := map[string]bool{}
	for _, l := range links {
		if l.FromTable == center {
			if _, ok := byName[l.ToTable]; ok && l.ToTable != center {
				outgoingSet[l.ToTable] = true
			}
		}
		if l.ToTable == center {
			if _, ok := byName[l.FromTable]; ok && l.FromTable != center {
				incomingSet[l.FromTable] = true
			}
		}
	}
	var incoming, outgoing []string
	for n := range incomingSet {
		if !outgoingSet[n] {
			incoming = append(incoming, n)
		}
	}
	for n := range outgoingSet {
		outgoing = append(outgoing, n)
	}
	sort.Strings(incoming)
	sort.Strings(outgoing)

	const gx, gy = 6, 2
	total := len(tables)
	ws := map[string]int{}
	hs := map[string]int{}
	colW := func(col []string) int {
		mx := 0
		for _, n := range col {
			w := erBoxWidth(byName[n]) + 2
			ws[n] = w
			if w > mx {
				mx = w
			}
		}
		return mx
	}
	colH := func(col []string) int {
		h := 0
		for i, n := range col {
			hh := erBoxHeight(byName[n], total) + 2
			hs[n] = hh
			h += hh
			if i < len(col)-1 {
				h += gy
			}
		}
		return h
	}
	centerW := erBoxWidth(byName[center]) + 2
	centerH := erBoxHeight(byName[center], total) + 2
	ws[center] = centerW
	hs[center] = centerH
	leftW := colW(incoming)
	_ = colW(outgoing)
	leftH := colH(incoming)
	rightH := colH(outgoing)
	maxH := centerH
	if leftH > maxH {
		maxH = leftH
	}
	if rightH > maxH {
		maxH = rightH
	}
	leftX := 0
	midX := leftX
	if len(incoming) > 0 {
		midX = leftX + leftW + gx
	}
	rightX := midX + centerW + gx

	pos := map[string]erRect{}
	y := (maxH - centerH) / 2
	if y < 0 {
		y = 0
	}
	pos[center] = erRect{x: midX, y: y, w: centerW, h: centerH}
	y = (maxH - leftH) / 2
	if y < 0 {
		y = 0
	}
	for _, n := range incoming {
		pos[n] = erRect{x: leftX, y: y, w: ws[n], h: hs[n]}
		y += hs[n] + gy
	}
	y = (maxH - rightH) / 2
	if y < 0 {
		y = 0
	}
	for _, n := range outgoing {
		pos[n] = erRect{x: rightX, y: y, w: ws[n], h: hs[n]}
		y += hs[n] + gy
	}
	return pos
}

var erPKTagStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#EAB308"))

// erFocusBoxLinesEx renders one bordered box with right-aligned PK/FK
// text labels (Image 1 style) instead of icons + type hints. Borders
// match erBoxLinesEx: blue #005FD7, gold #CA8A04 when selected (wins),
// cyan #00D7FF on hover.
func erFocusBoxLinesEx(t erTable, total int, selected, hovered bool) []string {
	cols, more := erVisibleCols(t, total)
	w := erBoxWidth(t)
	head := fitText("▦ "+t.name, w)
	// v2 Width is border-box: +2 keeps rows (built w wide above) fitting
	// the content area exactly, as under v1.
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#005FD7")).Width(w+2)
	switch {
	case selected:
		style = style.BorderForeground(lipgloss.Color("#CA8A04"))
	case hovered:
		style = style.BorderForeground(lipgloss.Color("#00D7FF"))
	}
	var rows []string
	for _, c := range cols {
		tag := ""
		tagStyled := ""
		left := "  " + c.Name
		switch {
		case t.pk[c.Name]:
			tag = "PK"
		case t.fk[c.Name]:
			tag = "FK"
		}
		left = fitText(left, w-len(tag)-1)
		gap := w - lipgloss.Width(left) - lipgloss.Width(tag)
		if gap < 1 {
			gap = 1
		}
		switch tag {
		case "PK":
			tagStyled = erPKTagStyle.Render(tag)
			rows = append(rows, dataSelectedStyle.Render(left)+strings.Repeat(" ", gap)+tagStyled)
		case "FK":
			tagStyled = dimStyle.Render(tag)
			rows = append(rows, left+strings.Repeat(" ", gap)+tagStyled)
		default:
			rows = append(rows, left)
		}
		_ = tagStyled
	}
	if more > 0 {
		rows = append(rows, dimStyle.Render(fitText("… "+strconv.Itoa(more)+" more", w)))
	}
	box := style.Render(strings.Join(append([]string{head}, rows...), "\n"))
	return strings.Split(box, "\n")
}

// erFocusedCanvasView renders the 1-hop diagram: a `diagram · <center>`
// header plus the filtered canvas sliced to the remaining viewport rows.
func (m Model) erFocusedCanvasView(innerW, innerH int) string {
	if innerH < 2 {
		innerH = 2
	}
	header := dimStyle.Render(fitText("diagram · "+m.erCenter, innerW))
	canvasH := innerH - 1
	ft, fl := erFocusedSubset(m.erCenter, m.erSchema.tables, m.erSchema.links)
	if len(ft) == 0 {
		return header + "\n" + strings.Join(erSliceViewport([]string{"(no tables)"}, m.erPanX, m.erPanY, innerW, canvasH), "\n")
	}
	pos := erFocusLayout(m.erCenter, ft, fl)
	canvas := erRenderWithPos(ft, fl, pos, erFocusBoxLinesEx, m.erSel, m.hoverER)
	lines := erSliceViewport(canvas, m.erPanX, m.erPanY, innerW, canvasH)
	if len(fl) == 0 && len(ft) > 0 {
		note := dimStyle.Render(fitText("(no foreign keys — boxes only)", innerW))
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines[len(lines)-1] = note
		} else {
			lines = append(lines, note)
			lines = lines[len(lines)-canvasH:]
		}
	}
	return header + "\n" + strings.Join(lines, "\n")
}

// erFocusedHit maps viewport coords to a box in the focused layout.
// Row detailTableTop is the header and never hits.
func (m Model) erFocusedHit(x, y int) (string, bool) {
	ft, fl := erFocusedSubset(m.erCenter, m.erSchema.tables, m.erSchema.links)
	pos := erFocusLayout(m.erCenter, ft, fl)
	rel := y - detailTableTop
	if rel <= 0 {
		return "", false // header row
	}
	cx := m.erPanX + x
	cy := (rel - 1) + m.erPanY
	for name, r := range pos {
		if cx >= r.x && cx < r.x+r.w && cy >= r.y && cy < r.y+r.h {
			return name, true
		}
	}
	return "", false
}
