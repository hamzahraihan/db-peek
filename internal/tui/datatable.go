package tui

import (
	"sort"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// dataTable is a minimal top-anchored grid for the detail view. Unlike
// bubbles/table it decouples the cursor from the viewport: the offset moves
// only on explicit scroll actions (keyboard, wheel, paging), never as a
// side effect of selection. That makes mouse hover stable — the content
// cannot shift under a stationary cursor. Hover is also separate state
// from selection, so the mouse never yanks keyboard context (e.g. the
// DDL echo line) around.
//
// Method names mirror the bubbles/table callsites they replace.
type dataTable struct {
	cols   []string
	widths []int
	rows   [][]string
	cursor int
	col    int // column cursor (cell editing)
	offset int
	hover  int // hovered absolute row, -1 when none
	width  int
	height int // visible data rows, excluding the header
	// Semantic styling: optional per-column roles plus a per-cell hook.
	// All styling runs on plain text after fitText/padCell, so widths and
	// mouse hit-testing (RowAt, dataHeaderH) are unchanged. Selection and
	// hover always win; zebra (bg-only) composes with cell fg colors.
	headerStyle    lipgloss.Style
	dimHeaderStyle lipgloss.Style
	colStyles      []lipgloss.Style
	dimColStyles   []lipgloss.Style
	// styleCell, when non-nil, returns the semantic style for one body cell.
	// dim reports whether the pane is unfocused (use muted roles).
	styleCell func(col int, val string, dim bool) (lipgloss.Style, bool)
	hasHeader bool // true once SetHeaderStyles has run; else globals apply
	// Sorting: local view-only sort. sortCol -1 means unsorted.
	sortCol int
	sortDir int // 1 = asc, -1 = desc
	// Filtering: local view-only find on loaded rows. baseRows is the
	// full data from setData; rows is the visible filtered+sorted view.
	baseRows [][]string
	filter   string
}

// SetHeaderStyles overrides the header role for this table (e.g. field-name
// headers on the Rows tab). Pass dim variant for unfocused rendering.
func (t *dataTable) SetHeaderStyles(normal, dimmed lipgloss.Style) {
	t.headerStyle, t.dimHeaderStyle, t.hasHeader = normal, dimmed, true
}

// SetColStyles assigns per-column body roles (field vs type vs meta).
// Length may be shorter than the column count; missing entries are plain.
func (t *dataTable) SetColStyles(normal, dimmed []lipgloss.Style) {
	t.colStyles, t.dimColStyles = normal, dimmed
}

// SetCellHook installs per-cell semantic overrides (PK gold, unique green).
// It runs after colStyles; returning false falls back to the column role.
func (t *dataTable) SetCellHook(hook func(col int, val string, dim bool) (lipgloss.Style, bool)) {
	t.styleCell = hook
}

// dataHeaderH is the rendered header height both here and in hit-testing:
// one title row plus the bottom border.
const dataHeaderH = 2

func (t *dataTable) setData(cols []string, rows [][]string) {
	t.cols = cols
	t.rows = rows
	t.baseRows = cloneRows(rows)
	t.filter = ""
	t.widths = distributeWidths(t.width, len(cols))
	t.sortCol = -1
	t.sortDir = 0
	if t.cursor > len(rows)-1 {
		t.cursor = len(rows) - 1
	}
	if t.cursor < 0 {
		t.cursor = 0
	}
	t.SetCol(t.col)
	t.hover = -1
	t.clampOffset()
}

func (t *dataTable) Resize(w, totalH int) {
	t.width = w
	t.widths = distributeWidths(w, len(t.cols))
	t.height = totalH - dataHeaderH
	if t.height < 1 {
		t.height = 1
	}
	t.clampOffset()
}

// SortState reports the current sort for tests/status: "" when unsorted,
// else "col:asc" or "col:desc".
func (t *dataTable) SortState() string {
	if t.sortCol < 0 || t.sortDir == 0 {
		return ""
	}
	dir := "asc"
	if t.sortDir < 0 {
		dir = "desc"
	}
	return strconv.Itoa(t.sortCol) + ":" + dir
}

// SortedCol reports the sorted column, or -1 when unsorted.
func (t *dataTable) SortedCol() int { return t.sortCol }

// SortBy cycles the sort for col: unsorted -> asc -> desc -> unsorted.
// Sorting is view-only and stable; clearing recomputes from baseRows so
// the pre-sort (and filtered) order returns.
func (t *dataTable) SortBy(col int) {
	if col < 0 || col >= len(t.cols) || len(t.baseRows) == 0 {
		return
	}
	if t.isPlaceholderBase() {
		return
	}
	if t.sortCol != col {
		t.sortCol, t.sortDir = col, 1
		t.rebuildVisible()
		return
	}
	if t.sortDir > 0 {
		t.sortDir = -1
		t.rebuildVisible()
		return
	}
	// Third click clears.
	t.sortCol, t.sortDir = -1, 0
	t.rebuildVisible()
}

// ClearSort drops any sort and recomputes the visible rows.
func (t *dataTable) ClearSort() {
	t.sortCol, t.sortDir = -1, 0
	t.rebuildVisible()
}

// SetFilter applies a case-insensitive substring find across all columns
// of the loaded rows. NULL never matches. Empty text clears.
func (t *dataTable) SetFilter(s string) {
	t.filter = s
	t.rebuildVisible()
}

// ClearFilter drops the find text and recomputes (keeping any sort).
func (t *dataTable) ClearFilter() {
	t.filter = ""
	t.rebuildVisible()
}

// FilterText reports the active find text, "" when none.
func (t *dataTable) FilterText() string { return t.filter }

// rebuildVisible derives rows from baseRows: filter first, then stable
// sort. Placeholder grids ("(no rows)") pass through untouched.
func (t *dataTable) rebuildVisible() {
	if t.isPlaceholderBase() {
		t.rows = cloneRows(t.baseRows)
		t.cursor = 0
		t.offset = 0
		t.hover = -1
		return
	}
	needle := strings.ToLower(t.filter)
	var out [][]string
	if needle == "" {
		out = cloneRows(t.baseRows)
	} else {
		for _, r := range t.baseRows {
			matched := false
			for _, c := range r {
				if isNullCell(c) {
					continue
				}
				if strings.Contains(strings.ToLower(c), needle) {
					matched = true
					break
				}
			}
			if matched {
				out = append(out, append([]string(nil), r...))
			}
		}
		if len(out) == 0 {
			out = [][]string{{"(no match)"}}
		}
	}
	if t.sortCol >= 0 && t.sortDir != 0 && len(out) > 0 && !isPlaceholderRow(out) {
		col, dir := t.sortCol, t.sortDir
		sort.SliceStable(out, func(i, j int) bool {
			a, b := cellAt(out[i], col), cellAt(out[j], col)
			aNull, bNull := isNullCell(a), isNullCell(b)
			if aNull && bNull {
				return false
			}
			if aNull {
				return false // nulls last in both directions
			}
			if bNull {
				return true
			}
			c := compareValues(a, b)
			if dir < 0 {
				c = -c
			}
			return c < 0
		})
	}
	t.rows = out
	t.cursor = 0
	t.offset = 0
	t.hover = -1
	t.clampOffset()
}

func (t *dataTable) isPlaceholderBase() bool { return isPlaceholderRow(t.baseRows) }

func isPlaceholderRow(rows [][]string) bool {
	if len(rows) != 1 || len(rows[0]) != 1 {
		return false
	}
	return rows[0][0] == "(no rows)" || rows[0][0] == "(no match)"
}

func cellAt(row []string, col int) string {
	if col < 0 || col >= len(row) {
		return ""
	}
	return row[col]
}

func cloneRows(in [][]string) [][]string {
	if in == nil {
		return nil
	}
	out := make([][]string, len(in))
	for i, r := range in {
		out[i] = append([]string(nil), r...)
	}
	return out
}

func isNullCell(s string) bool {
	return s == "" || s == "NULL" || s == "(no rows)" || s == "(no match)"
}

func compareCell(a, b string) int {
	aNull, bNull := isNullCell(a), isNullCell(b)
	if aNull && bNull {
		return 0
	}
	if aNull {
		return 1 // nulls last in asc order
	}
	if bNull {
		return -1
	}
	return compareValues(a, b)
}

func compareValues(a, b string) int {
	if af, err1 := strconv.ParseFloat(strings.TrimSpace(a), 64); err1 == nil {
		if bf, err2 := strconv.ParseFloat(strings.TrimSpace(b), 64); err2 == nil {
			switch {
			case af < bf:
				return -1
			case af > bf:
				return 1
			default:
				return 0
			}
		}
	}
	// Case-insensitive text compare; tie-break case-sensitive for stability.
	al, bl := strings.ToLower(a), strings.ToLower(b)
	if al < bl {
		return -1
	}
	if al > bl {
		return 1
	}
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// distributeWidths mirrors the old sizeTables policy: even split, last
// column takes the slack, with minimums so narrow terminals stay usable.
func distributeWidths(w, n int) []int {
	if n <= 0 {
		return nil
	}
	out := make([]int, n)
	per := (w - 4) / n
	if per < 8 {
		per = 8
	}
	for i := range out {
		out[i] = per
	}
	out[n-1] = w - 4 - per*(n-1)
	if out[n-1] < 10 {
		out[n-1] = 10
	}
	return out
}

func (t *dataTable) Cursor() int { return t.cursor }

func (t *dataTable) SetCursor(n int) {
	t.cursor = clamp(n, 0, len(t.rows)-1)
}

// Col is the column cursor. The grid has one for cell editing; nothing
// else depends on it, so it starts at 0 and is clamped with the data.
func (t *dataTable) Col() int { return t.col }

func (t *dataTable) SetCol(n int) {
	if n < 0 {
		n = 0
	}
	if max := len(t.cols) - 1; n > max {
		n = max
	}
	t.col = n
}

func (t *dataTable) MoveColLeft()  { t.SetCol(t.col - 1) }
func (t *dataTable) MoveColRight() { t.SetCol(t.col + 1) }

// ColAt maps a content x-offset to a column index, or -1 past the last
// column. It mirrors RowAt so a click can position both cursors.
func (t *dataTable) ColAt(x int) int {
	col := 0
	for i, w := range t.widths {
		if x < col+w {
			return i
		}
		col += w
	}
	return -1
}

func (t *dataTable) MoveUp(n int) {
	t.cursor = clamp(t.cursor-n, 0, len(t.rows)-1)
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
}

func (t *dataTable) MoveDown(n int) {
	t.cursor = clamp(t.cursor+n, 0, len(t.rows)-1)
	if t.cursor > t.offset+t.height-1 {
		t.offset = t.cursor - t.height + 1
	}
}

func (t *dataTable) GotoTop() {
	t.cursor = 0
	t.offset = 0
}

func (t *dataTable) GotoBottom() {
	t.cursor = len(t.rows) - 1
	if t.cursor < 0 {
		t.cursor = 0
	}
	t.offset = t.cursor - t.height + 1
	if t.offset < 0 {
		t.offset = 0
	}
}

func (t *dataTable) Rows() [][]string { return t.rows }
func (t *dataTable) Height() int      { return t.height }

func (t *dataTable) SetHover(n int) {
	if n < 0 || n >= len(t.rows) {
		t.hover = -1
		t.clampOffset()
		return
	}
	t.hover = n
}

func (t *dataTable) RowAt(rel int) (int, bool) {
	r := rel - dataHeaderH
	if r < 0 || r >= t.height {
		return 0, false
	}
	abs := t.offset + r
	if abs < 0 || abs >= len(t.rows) {
		return 0, false
	}
	return abs, true
}

func (t *dataTable) clampOffset() {
	max := len(t.rows) - t.height
	if max < 0 {
		max = 0
	}
	t.offset = clamp(t.offset, 0, max)
}

func (t *dataTable) View() string { return t.view(false) }

// ViewDimmed renders the grid in the muted palette used when focus sits
// on the sidebar. Layout is identical; only colors change.
func (t *dataTable) ViewDimmed() string { return t.view(true) }

func (t *dataTable) view(dim bool) string {
	headerStyle, selectedStyle := dataHeaderStyle, dataSelectedStyle
	hoverStyle := dataHoverStyle
	if dim {
		headerStyle, selectedStyle = dimDataHeaderStyle, dimDataSelectedStyle
		hoverStyle = lipgloss.NewStyle()
	}
	if t.hasHeader {
		if dim {
			headerStyle = t.dimHeaderStyle
		} else {
			headerStyle = t.headerStyle
		}
	}
	colStyles := t.colStyles
	if dim {
		colStyles = t.dimColStyles
	}
	var b strings.Builder
	cells := make([]string, len(t.cols))
	for i, c := range t.cols {
		label := c
		if i == t.sortCol && t.sortDir != 0 {
			if t.sortDir > 0 {
				label += " ▲"
			} else {
				label += " ▼"
			}
		}
		padded := padCell(label, t.widths[i])
		if i < len(colStyles) {
			cells[i] = colStyles[i].Render(padded)
		} else {
			cells[i] = padded
		}
	}
	if len(colStyles) == 0 {
		b.WriteString(headerStyle.Render(strings.Join(cells, "")) + "\n")
	} else {
		b.WriteString(strings.Join(cells, "") + "\n")
	}
	total := 0
	for _, w := range t.widths {
		total += w
	}
	b.WriteString(dataHeaderBorder.Render(strings.Repeat("─", total)) + "\n")
	end := t.offset + t.height
	if end > len(t.rows) {
		end = len(t.rows)
	}
	for i := t.offset; i < end; i++ {
		cells = cells[:0]
		selected := i == t.cursor && t.cursor >= 0
		hovered := i == t.hover
		plain := selected || hovered
		for j := range t.cols {
			v := ""
			if j < len(t.rows[i]) {
				v = t.rows[i][j]
			}
			padded := padCell(v, t.widths[j])
			if !plain {
				if t.styleCell != nil {
					if st, ok := t.styleCell(j, v, dim); ok {
						padded = st.Render(padded)
						cells = append(cells, padded)
						continue
					}
				}
				if j < len(colStyles) {
					padded = colStyles[j].Render(padded)
				}
			}
			cells = append(cells, padded)
		}
		line := strings.Join(cells, "")
		switch {
		case selected:
			line = selectedStyle.Render(line)
		case hovered:
			line = hoverStyle.Render(line)
		case i%2 == 1:
			if dim {
				line = zebraDimStyle.Render(line)
			} else {
				line = zebraStyle.Render(line)
			}
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func padCell(s string, w int) string {
	s = fitText(s, w)
	if pad := w - lipgloss.Width(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

func clamp(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}
