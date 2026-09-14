package tui

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	cols, more := erVisibleCols(t, total)
	w := erBoxWidth(t)
	head := fitText("▦ "+t.name, w)
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#005FD7")).Width(w)
	if selected {
		style = style.BorderForeground(lipgloss.Color("#CA8A04"))
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
		ws[i] = erBoxWidth(t)
		hs[i] = erBoxHeight(t, n)
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
