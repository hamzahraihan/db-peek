package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	t.widths = distributeWidths(t.width, len(cols))
	if t.cursor > len(rows)-1 {
		t.cursor = len(rows) - 1
	}
	if t.cursor < 0 {
		t.cursor = 0
	}
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
		padded := padCell(c, t.widths[i])
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
