package tui

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	dbpkg "db-peek/internal/db"
)

// erRect is a box origin + size in canvas cells.
type erRect struct{ x, y, w, h int }

func erTypeHint(sqlType string) string {
	s := strings.ToLower(sqlType)
	switch {
	case strings.Contains(s, "int") || strings.Contains(s, "serial") || strings.Contains(s, "numeric") || strings.Contains(s, "decimal") || strings.Contains(s, "float") || strings.Contains(s, "double") || strings.Contains(s, "real"):
		return "123"
	case strings.Contains(s, "char") || strings.Contains(s, "text") || strings.Contains(s, "varchar") || strings.Contains(s, "name"):
		return "ABC"
	case strings.Contains(s, "time") || strings.Contains(s, "date") || strings.Contains(s, "stamp"):
		return "◷"
	case strings.Contains(s, "bool"):
		return "◯"
	default:
		if sqlType == "" {
			return ""
		}
		return fitText(sqlType, 8)
	}
}

func erVisibleCols(t erTable, total int) ([]dbpkg.Column, int) {
	if total <= 40 || len(t.cols) <= 12 {
		return t.cols, 0
	}
	var keys, rest []dbpkg.Column
	for _, c := range t.cols {
		if t.pk[c.Name] || t.fk[c.Name] {
			keys = append(keys, c)
		} else {
			rest = append(rest, c)
		}
	}
	out := append([]dbpkg.Column{}, keys...)
	more := 0
	for _, c := range rest {
		if len(out) >= len(keys)+8 {
			more++
			continue
		}
		out = append(out, c)
	}
	return out, more
}

func erBoxWidth(t erTable) int {
	w := lipgloss.Width("▦ " + t.name)
	for _, c := range t.cols {
		if rw := lipgloss.Width(c.Name) + 6; rw > w {
			w = rw
		}
	}
	if w > 28 {
		w = 28
	}
	if w < 12 {
		w = 12
	}
	return w
}

func erBoxHeight(t erTable, total int) int {
	cols, more := erVisibleCols(t, total)
	h := 1 + len(cols)
	if more > 0 {
		h++
	}
	return h
}

func erBoxLines(t erTable, total int, selected bool) []string {
	return erBoxLinesEx(t, total, selected, false)
}

// erBoxLinesEx renders one bordered box; the border is blue #005FD7,
// gold #CA8A04 when selected (selected wins), cyan #00D7FF on hover.
func erBoxLinesEx(t erTable, total int, selected, hovered bool) []string {
	cols, more := erVisibleCols(t, total)
	w := erBoxWidth(t)
	head := fitText("▦ "+t.name, w)
	// v2 Width is border-box: +2 keeps rows (built w wide below) fitting
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
		icon := "◇"
		switch {
		case t.pk[c.Name]:
			icon = "🔑"
		case t.fk[c.Name]:
			icon = "➤"
		}
		hint := erTypeHint(c.Type)
		left := fitText(icon+" "+c.Name, w-len(hint)-1)
		gap := w - lipgloss.Width(left) - lipgloss.Width(hint)
		if gap < 1 {
			gap = 1
		}
		row := left + strings.Repeat(" ", gap) + dimStyle.Render(hint)
		if t.pk[c.Name] {
			row = dataSelectedStyle.Render(left) + strings.Repeat(" ", gap) + dimStyle.Render(hint)
		}
		rows = append(rows, row)
	}
	if more > 0 {
		rows = append(rows, dimStyle.Render(fitText("… "+strconv.Itoa(more)+" more", w)))
	}
	box := style.Render(strings.Join(append([]string{head}, rows...), "\n"))
	return strings.Split(box, "\n")
}

// erGridLayout places boxes deterministically: sort by name, grid columns
// from pane aspect, gaps gx=4 gy=2. Uses max box size for cell stride.
func erGridLayout(tables []erTable, innerW, innerH int) map[string]erRect {
	cp := append([]erTable(nil), tables...)
	sort.Slice(cp, func(i, j int) bool { return cp[i].name < cp[j].name })
	n := len(cp)
	if n == 0 {
		return map[string]erRect{}
	}
	if innerH < 1 {
		innerH = 1
	}
	ar := float64(innerW) / float64(innerH) / 2.0
	if ar < 1.0 {
		ar = 1.0
	}
	cols := int(0.5 + math.Sqrt(float64(n))*ar)
	if cols < 1 {
		cols = 1
	}
	if cols > n {
		cols = n
	}
	maxW, maxH := 0, 0
	ws := make([]int, n)
	hs := make([]int, n)
	for i, t := range cp {
		// +2 covers the rounded border on each axis so erRect matches
		// the rendered display size (erHit and connectors track it).
		ws[i] = erBoxWidth(t) + 2
		hs[i] = erBoxHeight(t, n) + 2
		if ws[i] > maxW {
			maxW = ws[i]
		}
		if hs[i] > maxH {
			maxH = hs[i]
		}
	}
	pos := map[string]erRect{}
	for i, t := range cp {
		r, c := i/cols, i%cols
		pos[t.name] = erRect{x: c * (maxW + 4), y: r * (maxH + 2), w: ws[i], h: hs[i]}
	}
	return pos
}

// erRenderCanvas draws connectors first then boxes over them; returns all
// canvas rows (unclipped). Caller slices via erSliceViewport.
func erRenderCanvas(tables []erTable, links []dbpkg.ForeignKey, innerW, innerH int, sel string) []string {
	return erRenderCanvasHover(tables, links, innerW, innerH, sel, "")
}

// erBoxFn renders one box's display lines; matches erBoxLinesEx and
// erFocusBoxLinesEx so focused and full modes share the canvas composer.
type erBoxFn func(t erTable, total int, selected, hovered bool) []string

// erRenderCanvasHover is erRenderCanvas plus a hover highlight: the
// selected box gets the gold border, the hovered box the cyan border.
// Overlay composes each row from the ANSI-free connector base plus whole
// styled box lines (never slicing styled strings); all measurement uses
// lipgloss.Width and all slicing uses ansi.Cut, so wide runes and
// escapes stay intact.
func erRenderCanvasHover(tables []erTable, links []dbpkg.ForeignKey, innerW, innerH int, sel, hover string) []string {
	if len(tables) == 0 {
		return []string{"(no tables)"}
	}
	pos := erGridLayout(tables, innerW, innerH)
	return erRenderWithPos(tables, links, pos, erBoxLinesEx, sel, hover)
}

// erRenderWithPos draws connectors first then boxes over them for an
// explicit box layout; returns all canvas rows (unclipped). Connectors
// are orthogonal Manhattan lines with an arrowhead at the target edge
// (▶/◀) so 1-hop focused diagrams read like `A ──▶ B`.
func erRenderWithPos(tables []erTable, links []dbpkg.ForeignKey, pos map[string]erRect, boxFn erBoxFn, sel, hover string) []string {
	if len(tables) == 0 {
		return []string{"(no tables)"}
	}
	cw, chh := 0, 0
	for _, r := range pos {
		if r.x+r.w > cw {
			cw = r.x + r.w
		}
		if r.y+r.h > chh {
			chh = r.y + r.h
		}
	}
	grid := make([][]rune, chh+2)
	for i := range grid {
		grid[i] = []rune(strings.Repeat(" ", cw+8))
	}
	set := func(x, y int, ch rune) {
		if y < 0 || y >= len(grid) || x < 0 || x >= len(grid[y]) {
			return
		}
		grid[y][x] = ch
	}
	linkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	_ = linkStyle
	for _, l := range links {
		a, okA := pos[l.FromTable]
		b, okB := pos[l.ToTable]
		if !okA || !okB {
			continue
		}
		y1 := a.y + a.h/2
		y2 := b.y + b.h/2
		x1 := a.x + a.w
		x2 := b.x
		if l.FromTable == l.ToTable {
			for x := x1; x < x1+3; x++ {
				set(x, y1, '┄')
			}
			continue
		}
		if y1 == y2 {
			step := 1
			if x2 < x1 {
				step = -1
			}
			for x := x1; x != x2; x += step {
				set(x, y1, '┄')
			}
			// Arrowhead just before the target box edge.
			if x2-step >= 0 {
				if step > 0 {
					set(x2-step, y1, '▶')
				} else {
					set(x2-step, y1, '◀')
				}
			}
			continue
		}
		mid := (x1 + x2) / 2
		if mid <= x1 && x2 > x1 {
			mid = x1 + 2
		}
		stepX := 1
		if mid < x1 {
			stepX = -1
		}
		for x := x1; x != mid; x += stepX {
			set(x, y1, '┄')
		}
		top, bot := y1, y2
		if bot < top {
			top, bot = bot, top
		}
		for y := top; y <= bot; y++ {
			if y == y1 {
				continue
			}
			set(mid, y, '┆')
		}
		step := 1
		if x2 < mid {
			step = -1
		}
		for x := mid; x != x2; x += step {
			set(x, y2, '┄')
		}
		if x2-step >= 0 {
			if step > 0 {
				set(x2-step, y2, '▶')
			} else {
				set(x2-step, y2, '◀')
			}
		}
	}
	base := make([]string, len(grid))
	for i, r := range grid {
		base[i] = strings.TrimRight(string(r), " ")
	}
	// Overlay boxes (box wins over connectors).
	byName := map[string]erTable{}
	for _, t := range tables {
		byName[t.name] = t
	}
	type seg struct {
		x  int
		ln string
	}
	segsByRow := map[int][]seg{}
	for _, t := range tables {
		r, ok := pos[t.name]
		if !ok {
			continue
		}
		bl := boxFn(byName[t.name], len(tables), t.name == sel, t.name == hover)
		for dy, ln := range bl {
			segsByRow[r.y+dy] = append(segsByRow[r.y+dy], seg{r.x, ln})
		}
	}
	rows := make([]string, len(base))
	for y, b := range base {
		segs := segsByRow[y]
		if len(segs) == 0 {
			rows[y] = b
			continue
		}
		sort.Slice(segs, func(i, j int) bool { return segs[i].x < segs[j].x })
		var sb strings.Builder
		cursor := 0
		for _, s := range segs {
			if s.x < cursor {
				continue // layout gaps guarantee no overlap; defensive skip
			}
			sb.WriteString(erCutPlain(b, cursor, s.x))
			sb.WriteString(s.ln)
			cursor = s.x + lipgloss.Width(s.ln)
		}
		sb.WriteString(erPlainTail(b, cursor))
		// TrimRight strips ASCII spaces only, so trailing escapes survive.
		rows[y] = strings.TrimRight(sb.String(), " ")
	}
	return rows
}

// erCutPlain returns display cells [from, to) of an ANSI-free string,
// padding with spaces when the string is shorter. ansi.Cut keeps wide
// runes intact.
func erCutPlain(s string, from, to int) string {
	if to <= from {
		return ""
	}
	part := ansi.Cut(s, from, to)
	if w := lipgloss.Width(part); w < to-from {
		part += strings.Repeat(" ", to-from-w)
	}
	return part
}

// erPlainTail returns display cells [from, width) of an ANSI-free string.
func erPlainTail(s string, from int) string {
	w := lipgloss.Width(s)
	if from >= w {
		return ""
	}
	return ansi.Cut(s, from, w)
}

func erSliceViewport(canvas []string, panX, panY, w, h int) []string {
	if h < 1 {
		h = 1
	}
	if panY < 0 {
		panY = 0
	}
	if panX < 0 {
		panX = 0
	}
	if panY > len(canvas)-1 {
		panY = len(canvas) - 1
	}
	if panY < 0 {
		panY = 0
	}
	out := []string{}
	for i := panY; i < panY+h && i < len(canvas); i++ {
		if w <= 0 {
			out = append(out, canvas[i])
			continue
		}
		// ansi.Cut is ANSI- and width-safe: canvas rows carry styled
		// box borders that rune slicing would tear apart.
		out = append(out, ansi.Cut(canvas[i], panX, panX+w))
	}
	for len(out) < h {
		out = append(out, "")
	}
	return out
}

func (m Model) erCanvasView(innerW, innerH int) string {
	if !m.erSchema.loaded {
		if m.loading {
			return "loading..."
		}
		return "(no ER data — press 5 to load)"
	}
	canvas := erRenderCanvasHover(m.erSchema.tables, m.erSchema.links, innerW, innerH, m.erSel, m.hoverER)
	lines := erSliceViewport(canvas, m.erPanX, m.erPanY, innerW, innerH)
	if len(m.erSchema.links) == 0 && len(m.erSchema.tables) > 0 {
		if innerH < 1 {
			innerH = 1
		}
		note := dimStyle.Render(fitText("(no foreign keys — boxes only)", innerW))
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines[len(lines)-1] = note // reuse trailing padding: keep box rows
		} else {
			lines = append(lines, note)
			lines = lines[len(lines)-innerH:]
		}
	}
	return strings.Join(lines, "\n")
}

// erNextBox cycles box selection in sorted order (n/p keys) because tab
// is reserved for pane tab-switching.
func erNextBox(tables []erTable, cur string, dir int) string {
	if len(tables) == 0 {
		return cur
	}
	names := make([]string, len(tables))
	for i, t := range tables {
		names[i] = t.name
	}
	sort.Strings(names)
	idx := 0
	for i, n := range names {
		if n == cur {
			idx = i
			break
		}
	}
	idx = (idx + dir + len(names)) % len(names)
	return names[idx]
}
