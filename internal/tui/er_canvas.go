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
			row = erPKFieldStyle.Render(left) + strings.Repeat(" ", gap) + dimStyle.Render(hint)
		}
		rows = append(rows, row)
	}
	if more > 0 {
		rows = append(rows, dimStyle.Render(fitText("… "+strconv.Itoa(more)+" more", w)))
	}
	box := style.Render(strings.Join(append([]string{head}, rows...), "\n"))
	return strings.Split(box, "\n")
}

// erGridLayout places boxes deterministically: connected tables in
// breadth-first order from root (alphabetically within each ring),
// grid columns from pane aspect, gaps gx=4 gy=2. Uses max box size
// for cell stride. Ordering by name instead scattered a table's
// references two columns away, and every one of those links then needed
// a lane across the whole diagram.
func erGridLayout(tables []erTable, links []dbpkg.ForeignKey, root string, innerW, innerH int) map[string]erRect {
	cp := erGridOrder(tables, links, root)
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
	place := func(c int) map[string]erRect {
		if c < 1 {
			c = 1
		}
		if c > n {
			c = n
		}
		p := map[string]erRect{}
		for i, t := range cp {
			r, col := i/c, i%c
			p[t.name] = erRect{x: col * (maxW + 12), y: r * (maxH + 2), w: ws[i], h: hs[i]}
		}
		return p
	}
	// The aspect ratio says how wide the grid looks right; what the
	// reader notices is a link that has to climb over a box to reach
	// its target. Narrow the grid until same-row links are neighbours —
	// anything else jumps a box and needs a lane across the diagram.
	// The stack is capped at half again the pane height: a cleaner
	// diagram is not worth one you cannot see, and a big schema would
	// otherwise collapse into a single tower.
	pos := place(cols)
	tallest := innerH * 3 / 2
	for c := cols - 1; c >= 2; c-- {
		trial := place(c)
		if erGridHeight(trial) > tallest {
			break
		}
		if erGridRowSpan(links, trial, maxW+4) <= 1 {
			pos = trial
			break
		}
	}
	erCenterIn(pos, innerW, innerH)
	return pos
}

// erGridRowSpan is the widest gap, counted in grid columns, between
// boxes that share a row: those are the links whose line has to cross
// whatever sits between them.
func erGridRowSpan(links []dbpkg.ForeignKey, pos map[string]erRect, stride int) int {
	if stride < 1 {
		stride = 1
	}
	worst := 0
	for _, l := range links {
		a, okA := pos[l.FromTable]
		b, okB := pos[l.ToTable]
		if !okA || !okB || a.y != b.y {
			continue
		}
		// Left edges, not centres: boxes vary in width, so a centre
		// difference is not a whole number of strides and integer
		// division quietly floors two columns down to one.
		d := (a.x - b.x) / stride
		if d < 0 {
			d = -d
		}
		if d > worst {
			worst = d
		}
	}
	return worst
}

// erGridHeight is the row a layout's lowest box ends on.
func erGridHeight(pos map[string]erRect) int {
	bottom := 0
	for _, r := range pos {
		if r.y+r.h > bottom {
			bottom = r.y + r.h
		}
	}
	return bottom
}

// erGridOrder sorts tables so that related ones land next to each other.
// A breadth-first walk from root keeps every hop one step further out,
// and alphabetical order inside a ring keeps the result stable across
// frames; anything unreachable from root (tables with no foreign keys at
// all) trails alphabetically.
func erGridOrder(tables []erTable, links []dbpkg.ForeignKey, root string) []erTable {
	byName := map[string]erTable{}
	for _, t := range tables {
		byName[t.name] = t
	}
	adj := map[string][]string{}
	for _, l := range links {
		if l.FromTable == l.ToTable {
			continue
		}
		if _, ok := byName[l.FromTable]; !ok {
			continue
		}
		if _, ok := byName[l.ToTable]; !ok {
			continue
		}
		adj[l.FromTable] = append(adj[l.FromTable], l.ToTable)
		adj[l.ToTable] = append(adj[l.ToTable], l.FromTable)
	}
	if _, ok := byName[root]; !ok {
		names := make([]string, 0, len(byName))
		for n := range byName {
			names = append(names, n)
		}
		sort.Strings(names)
		if len(names) > 0 {
			root = names[0]
		}
	}
	out := make([]erTable, 0, len(tables))
	seen := map[string]bool{}
	for queue := []string{root}; len(queue) > 0; queue = queue[1:] {
		name := queue[0]
		if seen[name] {
			continue
		}
		seen[name] = true
		if t, ok := byName[name]; ok {
			out = append(out, t)
		}
		ring := append([]string(nil), adj[name]...)
		sort.Strings(ring)
		queue = append(queue, ring...)
	}
	rest := make([]erTable, 0, len(tables))
	for _, t := range tables {
		if !seen[t.name] {
			rest = append(rest, t)
		}
	}
	sort.Slice(rest, func(i, j int) bool { return rest[i].name < rest[j].name })
	return append(out, rest...)
}

// erCenterIn shifts a layout into the pane's spare space so a diagram
// that fits sits in the middle instead of hugging the top-left corner —
// which left the focused box stranded next to a wall of empty cells when
// it had no outgoing column. Layouts that overflow do not move, and
// panning still travels the whole canvas, because hit-testing shares the
// same rects.
func erCenterIn(pos map[string]erRect, w, h int) {
	maxX, maxY := 0, 0
	for _, r := range pos {
		if r.x+r.w > maxX {
			maxX = r.x + r.w
		}
		if r.y+r.h > maxY {
			maxY = r.y + r.h
		}
	}
	dx, dy := 0, 0
	if d := w - maxX; d > 0 {
		dx = d / 2
	}
	if d := h - maxY; d > 0 {
		dy = d / 2
	}
	if dx == 0 && dy == 0 {
		return
	}
	for name, r := range pos {
		r.x += dx
		r.y += dy
		pos[name] = r
	}
}

// erPKFieldStyle marks a primary-key row's name inside a diagram box:
// bold and bright, matching the explorer's key column. It deliberately
// avoids the detail grid's selection band, which inside a diagram would
// read as a selected row on a non-selectable surface.
var erPKFieldStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))

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
	pos := erGridLayout(tables, links, sel, innerW, innerH)
	return erRenderWithPos(tables, links, pos, erBoxLinesEx, sel, hover)
}

// erRenderWithPos draws connectors first then boxes over them for an
// explicit box layout; returns all canvas rows (unclipped). Connectors
// are orthogonal paths with corners and an arrowhead at the target edge
// (▶/◀), so 1-hop focused diagrams read like `A ──▶ B`.
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
	// Four rows of slack, not two: a self-reference loop hangs two rows
	// below its box, so a box on the last row would have its loop clipped.
	grid := make([][]rune, chh+4)
	for i := range grid {
		grid[i] = []rune(strings.Repeat(" ", cw+8))
	}
	set := func(x, y int, ch rune) {
		if y < 0 || y >= len(grid) || x < 0 || x >= len(grid[y]) {
			return
		}
		grid[y][x] = ch
	}
	byName := map[string]erTable{}
	for _, t := range tables {
		byName[t.name] = t
	}
	// Links sharing a box must not share one row: spread their anchors
	// over the box edge so two incoming keys enter on different cells
	// instead of collapsing into a single arrow.
	touches, seen := map[string]int{}, map[string]int{}
	for _, l := range links {
		if l.FromTable != l.ToTable {
			touches[l.FromTable]++
			touches[l.ToTable]++
		}
	}
	// anchor is the row a link touches a box on: the row of the column
	// it names, so a line lands on user_id rather than the middle of the
	// box. Several keys point at the same target column, so they fan out
	// one row at a time.
	anchor := func(name string, r erRect, column string) int {
		y := r.y + r.h/2
		if t, ok := byName[name]; ok {
			cols, _ := erVisibleCols(t, len(tables))
			for i, c := range cols {
				if c.Name == column {
					y = r.y + 1 + i
					break
				}
			}
		}
		i := seen[name]
		seen[name] = i + 1
		if touches[name] > 1 {
			y += i - (touches[name]-1)/2
		}
		if lo, hi := r.y+1, r.y+r.h-2; y < lo {
			y = lo
		} else if y > hi {
			y = hi
		}
		return y
	}
	blocks := erBlocks(pos, links)
	// Relationships are single orthogonal arrows: FK box ──▶ PK box.
	// No crow's-foot fans, bars, circles or cardinality labels. Each
	// link gets its own lane (via occ) so parallel lines never share
	// cells and read as one clean line each.
	occ := erOccupied{}
	for i, l := range links {
		a, okA := pos[l.FromTable]
		b, okB := pos[l.ToTable]
		if !okA || !okB {
			continue
		}
		if l.FromTable == l.ToTable {
			erSelfLinkArrow(set, blocks, occ, a)
			continue
		}
		erConnector(set, blocks, occ, a, b,
			anchor(l.FromTable, a, l.FromColumn), anchor(l.ToTable, b, l.ToColumn), i)
	}
	// Connectors are chrome around the boxes, not content: unstyled they
	// render in the terminal's default foreground and shout louder than
	// the schema itself.
	base := make([]string, len(grid))
	for i, r := range grid {
		base[i] = erStyleLine(string(r))
	}
	// Overlay boxes (box wins over connectors).
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

// erOccupied tracks connector cells already drawn so parallel links
// take separate lanes instead of sharing (and burying) one row.
type erOccupied map[[2]int]bool

func erCellBlocked(blocks []erRect, occ erOccupied, x, y int) bool {
	for _, r := range blocks {
		if y >= r.y && y < r.y+r.h && x >= r.x && x < r.x+r.w {
			return true
		}
	}
	return occ[[2]int{x, y}]
}

func erHFree(blocks []erRect, occ erOccupied, x0, x1, y int) bool {
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	for x := x0; x <= x1; x++ {
		if erCellBlocked(blocks, occ, x, y) {
			return false
		}
	}
	return true
}

func erVFree(blocks []erRect, occ erOccupied, x, y0, y1 int) bool {
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	for y := y0; y <= y1; y++ {
		if erCellBlocked(blocks, occ, x, y) {
			return false
		}
	}
	return true
}

// erConnector draws one foreign key as a continuous orthogonal arrow
// from the FK box edge to the PK box edge: ── │ with ┌┐└┘ corners and
// a single ▶/◀ head at the target. Entry is always horizontal so the
// head points into the box. Lanes (occ + lane offset) keep parallel
// links on separate cells.
func erConnector(set func(x, y int, ch rune), blocks []erRect, occ erOccupied, a, b erRect, ay, by, lane int) {
	aL, aR, bL, bR := a.x, a.x+a.w, b.x, b.x+b.w
	drawH := func(x0, x1, y int) {
		for x := min(x0, x1); x <= max(x0, x1); x++ {
			set(x, y, '─')
			occ[[2]int{x, y}] = true
		}
	}
	drawV := func(x, y0, y1 int) {
		lo, hi := min(y0, y1), max(y0, y1)
		for y := lo; y <= hi; y++ {
			set(x, y, '│')
			occ[[2]int{x, y}] = true
		}
	}
	// drawThree paints (sx,sy)->(col,sy)->(col,ey)->(ex,ey) with marked
	// corners and a horizontal arrow head at the target edge.
	drawThree := func(sx, sy, col, ex, ey int, arrow rune) {
		if sx != col {
			drawH(sx, col, sy)
		} else if sy == ey {
			drawH(sx, ex, sy)
		}
		if sy != ey {
			drawV(col, sy, ey)
		}
		if col != ex {
			drawH(col, ex, ey)
		}
		if sx != col && sy != ey {
			set(col, sy, erCorner(sgn(col-sx), 0, 0, sgn(ey-sy)))
			occ[[2]int{col, sy}] = true
		}
		if col != ex && sy != ey {
			set(col, ey, erCorner(0, sgn(ey-sy), sgn(ex-col), 0))
			occ[[2]int{col, ey}] = true
		}
		set(ex, ey, arrow)
		occ[[2]int{ex, ey}] = true
	}
	threeFree := func(sx, sy, col, ex, ey int) bool {
		if sx != col && !erHFree(blocks, occ, sx, col, sy) {
			return false
		}
		if sy != ey && !erVFree(blocks, occ, col, sy, ey) {
			return false
		}
		if col != ex && !erHFree(blocks, occ, col, ex, ey) {
			return false
		}
		if erCellBlocked(blocks, occ, ex, ey) {
			// Arrow cell may already hold another arrow on the same row
			// (two FKs to one PK row after fanning failed); still allow
			// overwrite so every link draws something.
			if !(sy != ey || sx != col) {
				return true
			}
		}
		return true
	}
	switch {
	case bL >= aR: // target to the right: leave right, enter left with ▶
		sx, ex := aR, bL-1
		mid := (sx + ex) / 2
		for _, col := range erLaneCols(mid+lane, sx, ex-1) {
			if threeFree(sx, ay, col, ex, by) {
				drawThree(sx, ay, col, ex, by, '▶')
				return
			}
		}
		// Skipping over a middle box: bridge above/below, then dive
		// into the target's gap so entry stays horizontal.
		if col, cy, ok := erBridgeLane(blocks, occ, sx, ay, ex, by, lane); ok {
			// Four segments: up to cy, across, down, in.
			drawV(sx, ay, cy)
			drawH(sx, col, cy)
			drawV(col, cy, by)
			drawH(col, ex, by)
			set(sx, cy, erCorner(0, sgn(cy-ay), sgn(col-sx), 0))
			set(col, cy, erCorner(sgn(col-sx), 0, 0, sgn(by-cy)))
			set(col, by, erCorner(0, sgn(by-cy), sgn(ex-col), 0))
			occ[[2]int{sx, cy}] = true
			occ[[2]int{col, cy}] = true
			occ[[2]int{col, by}] = true
			set(ex, by, '▶')
			occ[[2]int{ex, by}] = true
			return
		}
		drawThree(sx, ay, mid, ex, by, '▶')
	case bR <= aL: // target to the left: leave left, enter right with ◀
		sx, ex := aL-1, bR
		mid := (sx + ex) / 2
		for _, col := range erLaneCols(mid-lane, ex+1, sx) {
			if threeFree(sx, ay, col, ex, by) {
				drawThree(sx, ay, col, ex, by, '◀')
				return
			}
		}
		if col, cy, ok := erBridgeLane(blocks, occ, sx, ay, ex, by, lane); ok {
			drawV(sx, ay, cy)
			drawH(sx, col, cy)
			drawV(col, cy, by)
			drawH(col, ex, by)
			set(sx, cy, erCorner(0, sgn(cy-ay), sgn(col-sx), 0))
			set(col, cy, erCorner(sgn(col-sx), 0, 0, sgn(by-cy)))
			set(col, by, erCorner(0, sgn(by-cy), sgn(ex-col), 0))
			occ[[2]int{sx, cy}] = true
			occ[[2]int{col, cy}] = true
			occ[[2]int{col, by}] = true
			set(ex, by, '◀')
			occ[[2]int{ex, by}] = true
			return
		}
		drawThree(sx, ay, mid, ex, by, '◀')
	default: // overlapping columns: out on one side, back in on it
		for i := 0; i < 10; i++ {
			col := max(aR, bR) + 1 + lane*2 + i
			if threeFree(aR, ay, col, bR, by) {
				drawThree(aR, ay, col, bR, by, '◀')
				return
			}
		}
		for i := 0; i < 10; i++ {
			col := min(aL, bL) - 2 - lane*2 - i
			if threeFree(aL-1, ay, col, bL-1, by) {
				drawThree(aL-1, ay, col, bL-1, by, '▶')
				return
			}
		}
		// Last resort: hug the right side even if occupied.
		drawThree(aR, ay, max(aR, bR)+1, bR, by, '◀')
	}
}

// erLaneCols orders candidate vertical channels around prefer, clamped
// to [lo, hi], so parallel links spread instead of sharing one column.
func erLaneCols(prefer, lo, hi int) []int {
	if hi < lo {
		lo, hi = hi, lo
	}
	if prefer < lo {
		prefer = lo
	}
	if prefer > hi {
		prefer = hi
	}
	var out []int
	for d := 0; d <= hi-lo; d++ {
		if c := prefer + d; c <= hi {
			out = append(out, c)
		}
		if d > 0 {
			if c := prefer - d; c >= lo {
				out = append(out, c)
			}
		}
	}
	if len(out) == 0 {
		return []int{(lo + hi) / 2}
	}
	return out
}

// erBridgeLane routes a skipping link above or below every box: a top
// lane row cy plus a dive column near the target so the final entry is
// still horizontal (arrow ▶/◀ reads correctly).
func erBridgeLane(blocks []erRect, occ erOccupied, sx, ay, ex, by, lane int) (int, int, bool) {
	top, bottom := 1<<30, -1<<30
	for _, r := range blocks {
		if r.y < top {
			top = r.y
		}
		if r.y+r.h > bottom {
			bottom = r.y + r.h
		}
	}
	for _, cy := range []int{top - 1 - lane, bottom + lane, top - 2 - lane, bottom + 1 + lane} {
		for _, col := range []int{ex - 2 - lane, ex + 2 + lane, (sx + ex) / 2} {
			if !erVFree(blocks, occ, sx, ay, cy) {
				continue
			}
			if !erHFree(blocks, occ, sx, col, cy) {
				continue
			}
			if !erVFree(blocks, occ, col, cy, by) {
				continue
			}
			if !erHFree(blocks, occ, col, ex, by) {
				continue
			}
			return col, cy, true
		}
	}
	return 0, 0, false
}

// erLinkStyle dims relationship lines so the boxes stay the subject.
// Unstyled they take the terminal's default foreground and read louder
// than the schema they describe.
var erLinkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

// erStyleLine dims the connector glyphs of one canvas row, leaving the
// blank cells that carry nothing. Splitting on spaces keeps the styling
// off the padding, so box lines composed over it stay exact width.
func erStyleLine(row string) string {
	var b, run strings.Builder
	flush := func() {
		if run.Len() > 0 {
			b.WriteString(erLinkStyle.Render(run.String()))
			run.Reset()
		}
	}
	for _, r := range row {
		if r == ' ' {
			flush()
			b.WriteRune(r)
			continue
		}
		run.WriteRune(r)
	}
	flush()
	return strings.TrimRight(b.String(), " ")
}

// erBlocks returns every box rect plus the cells each self-referencing
// link reserves for its side loop. Connectors route around the lot.
func erBlocks(pos map[string]erRect, links []dbpkg.ForeignKey) []erRect {
	blocks := make([]erRect, 0, len(pos))
	for _, r := range pos {
		blocks = append(blocks, r)
	}
	for _, l := range links {
		if l.FromTable != l.ToTable {
			continue
		}
		if r, ok := pos[l.FromTable]; ok {
			blocks = append(blocks, erRect{x: r.x + r.w, y: r.y + 1, w: 3, h: 2})
		}
	}
	return blocks
}

// erRowFree reports whether no box occupies cells [x0, x1] of row y.
func erRowFree(blocks []erRect, x0, x1, y int) bool {
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	for _, r := range blocks {
		if y >= r.y && y < r.y+r.h && x1 >= r.x && x0 < r.x+r.w {
			return false
		}
	}
	return true
}

// erRectFree reports whether no box occupies any cell of the rectangle.
func erRectFree(blocks []erRect, x0, x1, y0, y1 int) bool {
	for y := y0; y <= y1; y++ {
		if !erRowFree(blocks, x0, x1, y) {
			return false
		}
	}
	return true
}

// erSelfLinkArrow draws a table's reference to itself as a small loop
// off its right edge with an arrow head re-entering the box.
func erSelfLinkArrow(set func(x, y int, ch rune), blocks []erRect, occ erOccupied, a erRect) {
	y1, y2 := a.y+1, a.y+2
	if y2 > a.y+a.h-1 {
		y2 = a.y + a.h - 1
		y1 = y2 - 1
	}
	if y1 < a.y+1 {
		y1 = a.y + 1
	}
	x0, col := a.x+a.w, a.x+a.w+2
	mark := func(x, y int, ch rune) {
		set(x, y, ch)
		occ[[2]int{x, y}] = true
	}
	for x := x0; x <= col; x++ {
		mark(x, y1, '─')
		mark(x, y2, '─')
	}
	mark(col, y1, '┐')
	mark(col, y2, '┘')
	mark(col, (y1+y2)/2, '│')
	if y2-y1 > 1 {
		for y := y1 + 1; y < y2; y++ {
			mark(col, y, '│')
		}
	}
	mark(x0, y2, '◀')
}

// erCorner returns the glyph joining two perpendicular directions. The
// turn cells are what make a path read as one line.
func erCorner(inX, inY, outX, outY int) rune {
	switch {
	case inX > 0 && outY > 0:
		return '┐'
	case inX > 0 && outY < 0:
		return '┘'
	case inX < 0 && outY > 0:
		return '┌'
	case inX < 0 && outY < 0:
		return '└'
	case inY > 0 && outX > 0:
		return '└'
	case inY > 0 && outX < 0:
		return '┘'
	case inY < 0 && outX > 0:
		return '┌'
	case inY < 0 && outX < 0:
		return '┐'
	}
	return '─'
}


// sgn is the sign of n: 1, -1 or 0.
func sgn(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	}
	return 0
}

// erRunH paints horizontal dashes from x0 to x1 inclusive at row y.
func erRunH(set func(x, y int, ch rune), x0, x1, y int) {
	for x := x0; x <= x1; x++ {
		set(x, y, '─')
	}
}

// erRunV paints vertical bars from y0 to y1 inclusive at column x.
func erRunV(set func(x, y int, ch rune), x, y0, y1 int) {
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	for y := y0; y <= y1; y++ {
		set(x, y, '│')
	}
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
	tables, links, _ := m.erLayoutFor(innerW, innerH)
	canvas := m.erCanvasFor(innerW, innerH)
	lines := erSliceViewport(canvas, m.erPanX, m.erPanY, innerW, innerH)
	if len(links) == 0 && len(tables) > 0 {
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
