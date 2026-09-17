package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type RowKind int

const (
	RowSchema RowKind = iota
	RowTable
	RowColumn
)

type ColumnNode struct {
	Name     string
	DataType string
	IsPK     bool
	IsFK     bool
}

type TableNode struct {
	Schema   string
	Name     string
	IsView   bool
	Expanded bool
	Count    int64
	CountOK  bool
	CountErr bool // latched on count failure: Render shows "?", loader skips retry
	Columns  []ColumnNode
}

type SchemaNode struct {
	Name     string
	Expanded bool
	Tables   []TableNode
}

type Row struct {
	Kind   RowKind
	Schema string
	Table  string
	Column string
	Depth  int
}

type Explorer struct {
	ConnName string
	Schemas  []SchemaNode
	Cursor   int
	Filter   string
	Offset   int // first visible tree-row index into VisibleRows
}

func NewExplorer(connName string, schemas []string) Explorer {
	e := Explorer{ConnName: connName}
	// TablePlus/DBeaver UX: public is the default working schema.
	// Expand public when present, otherwise expand the first schema so
	// the tree is never fully collapsed on connect.
	expandIdx := 0
	for i, s := range schemas {
		if s == "public" {
			expandIdx = i
			break
		}
	}
	for i, s := range schemas {
		e.Schemas = append(e.Schemas, SchemaNode{Name: s, Expanded: i == expandIdx})
	}
	return e
}

func (e *Explorer) VisibleRows() []Row {
	var out []Row
	f := strings.ToLower(strings.TrimSpace(e.Filter))
	match := func(s string) bool {
		if f == "" {
			return true
		}
		return strings.Contains(strings.ToLower(s), f)
	}
	for _, s := range e.Schemas {
		var kept []TableNode
		for _, tb := range s.Tables {
			if f != "" && !match(tb.Name) {
				var cols []ColumnNode
				for _, c := range tb.Columns {
					if match(c.Name) {
						cols = append(cols, c)
					}
				}
				if len(cols) == 0 {
					continue
				}
				cp := tb
				cp.Columns = cols
				kept = append(kept, cp)
				continue
			}
			kept = append(kept, tb)
		}
		if f != "" && len(kept) == 0 && !match(s.Name) {
			continue
		}
		out = append(out, Row{Kind: RowSchema, Schema: s.Name, Depth: 0})
		if !s.Expanded && f == "" {
			continue
		}
		for _, tb := range kept {
			out = append(out, Row{Kind: RowTable, Schema: s.Name, Table: tb.Name, Depth: 1})
			if !tb.Expanded && f == "" {
				continue
			}
			for _, c := range tb.Columns {
				if f != "" && !match(c.Name) && !match(tb.Name) {
					continue
				}
				out = append(out, Row{Kind: RowColumn, Schema: s.Name, Table: tb.Name, Column: c.Name, Depth: 2})
			}
		}
	}
	return out
}

func (e *Explorer) RowAt(i int) (Row, bool) {
	rows := e.VisibleRows()
	if i < 0 || i >= len(rows) {
		return Row{}, false
	}
	return rows[i], true
}

func (e *Explorer) MoveUp() {
	if e.Cursor > 0 {
		e.Cursor--
	}
}

func (e *Explorer) MoveDown() {
	if e.Cursor < len(e.VisibleRows())-1 {
		e.Cursor++
	}
}

func (e *Explorer) Toggle() {
	r, ok := e.RowAt(e.Cursor)
	if !ok {
		return
	}
	switch r.Kind {
	case RowSchema:
		for i := range e.Schemas {
			if e.Schemas[i].Name == r.Schema {
				e.Schemas[i].Expanded = !e.Schemas[i].Expanded
			}
		}
	case RowTable:
		for si := range e.Schemas {
			if e.Schemas[si].Name != r.Schema {
				continue
			}
			for ti := range e.Schemas[si].Tables {
				if e.Schemas[si].Tables[ti].Name == r.Table {
					e.Schemas[si].Tables[ti].Expanded = !e.Schemas[si].Tables[ti].Expanded
				}
			}
		}
	}
	if e.Cursor >= len(e.VisibleRows()) {
		e.Cursor = len(e.VisibleRows()) - 1
	}
	if e.Cursor < 0 {
		e.Cursor = 0
	}
}

func (e *Explorer) SetFilter(f string) {
	e.Filter = f
	e.Cursor = 0
	e.Offset = 0
}

// ensureVisible keeps Cursor inside [Offset, Offset+viewH) and Offset
// inside its valid range. Call after every cursor/row mutation.
func (e *Explorer) ensureVisible(viewH int) {
	if viewH < 1 {
		viewH = 1
	}
	n := len(e.VisibleRows())
	if n == 0 {
		e.Cursor, e.Offset = 0, 0
		return
	}
	if e.Cursor < 0 {
		e.Cursor = 0
	}
	if e.Cursor >= n {
		e.Cursor = n - 1
	}
	maxOff := n - viewH
	if maxOff < 0 {
		maxOff = 0
	}
	if e.Offset < 0 {
		e.Offset = 0
	}
	if e.Offset > maxOff {
		e.Offset = maxOff
	}
	if e.Cursor < e.Offset {
		e.Offset = e.Cursor
	}
	if e.Cursor >= e.Offset+viewH {
		e.Offset = e.Cursor - viewH + 1
	}
}

func humanizeCount(n int64) string {
	switch {
	case n >= 1000000:
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func (e *Explorer) tableByName(schema, table string) (TableNode, bool) {
	for _, s := range e.Schemas {
		if s.Name != schema {
			continue
		}
		for _, tb := range s.Tables {
			if tb.Name == table {
				return tb, true
			}
		}
	}
	return TableNode{}, false
}

func (e *Explorer) columnByName(schema, table, col string) (ColumnNode, bool) {
	tb, ok := e.tableByName(schema, table)
	if !ok {
		return ColumnNode{}, false
	}
	for _, c := range tb.Columns {
		if c.Name == col {
			return c, true
		}
	}
	return ColumnNode{}, false
}

// explorerLine joins a plain left and plain right with gap spaces to
// exactly fit w cells, then applies styleRight. Truncation runs on
// plain text only, so styled output is never sliced mid-escape.
func explorerLine(left, right string, styleRight func(string) string, w int) string {
	return explorerLineStyled(left, right, nil, styleRight, w)
}

// explorerLineStyled is explorerLine with an optional left style (field name
// vs type). styleLeft may be nil for plain left.
func explorerLineStyled(left, right string, styleLeft, styleRight func(string) string, w int) string {
	if w < 4 {
		w = 4
	}
	if lipgloss.Width(right) > w-2 {
		right = fitText(right, w-2)
	}
	maxLeft := w - lipgloss.Width(right) - 1
	if maxLeft < 1 {
		maxLeft = 1
	}
	left = fitText(left, maxLeft)
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	if styleLeft != nil {
		left = styleLeft(left)
	}
	if styleRight != nil {
		return left + strings.Repeat(" ", gap) + styleRight(right)
	}
	return left + strings.Repeat(" ", gap) + right
}

// setLastCell overlays ch on the final cell of an ANSI-styled line of
// width w, padding short lines with spaces first.
func setLastCell(line, ch string, w int) string {
	if w < 1 {
		return line
	}
	if vw := lipgloss.Width(line); vw < w-1 {
		line += strings.Repeat(" ", w-1-vw)
	} else if vw >= w {
		line = ansi.Truncate(line, w-1, "")
	}
	return line + ch
}

// visibleStart is the single source of truth for the first visible
// tree-row index: Offset clamped to its valid range for viewH. Render
// stays pure (never mutates Offset), so mouse hit-testing must derive
// its index from here too — otherwise a stale Offset (rows shrank while
// scrolled, e.g. collapsing a schema) shows one window while clicks map
// into another.
func (e *Explorer) visibleStart(viewH int) int {
	if viewH < 1 {
		viewH = 1
	}
	maxStart := len(e.VisibleRows()) - viewH
	if maxStart < 0 {
		maxStart = 0
	}
	s := e.Offset
	if s < 0 {
		s = 0
	}
	if s > maxStart {
		s = maxStart
	}
	return s
}

func (e *Explorer) Render(sidebarW, height int) string {
	rows := e.VisibleRows()
	if height < 1 {
		height = 1
	}
	start := e.visibleStart(height)
	end := start + height
	if end > len(rows) {
		end = len(rows)
	}
	vis := rows[start:end]
	var b strings.Builder
	b.WriteString(explorerTitle.Render("explorer") + "\n")
	connLeft := fitText("● "+e.ConnName, sidebarW-2)
	gap := sidebarW - lipgloss.Width(connLeft) - 1
	if gap < 1 {
		gap = 1
	}
	connLine := explorerConn.Render(connLeft) + strings.Repeat(" ", gap) + dimStyle.Render("×")
	b.WriteString(connLine + "\n")
	var lines []string
	for i, r := range vis {
		var line string
		switch r.Kind {
		case RowSchema:
			disc := "▸"
			for _, s := range e.Schemas {
				if s.Name == r.Schema && s.Expanded {
					disc = "▾"
				}
			}
			n := 0
			for _, s := range e.Schemas {
				if s.Name == r.Schema {
					n = len(s.Tables)
				}
			}
			left := disc + " 🗄 " + r.Schema
			right := fmt.Sprintf("%d", n)
			line = explorerLine(left, right, func(s string) string { return explorerCount.Render(s) }, sidebarW)
		case RowTable:
			tb, _ := e.tableByName(r.Schema, r.Table)
			disc := "▸"
			if tb.Expanded {
				disc = "▾"
			}
			icon := "▦"
			if tb.IsView {
				icon = "👁"
			}
			left := "  " + disc + " " + icon + " " + r.Table
			var right string
			if tb.CountErr {
				right = "?"
			} else if tb.CountOK {
				right = humanizeCount(tb.Count)
			} else {
				right = "…"
			}
			line = explorerLine(left, right, func(s string) string { return explorerCount.Render(s) }, sidebarW)
		case RowColumn:
			c, _ := e.columnByName(r.Schema, r.Table, r.Column)
			icon := "◇"
			styleLeft := func(s string) string { return explorerCol.Render(s) }
			switch {
			case c.IsPK:
				icon = "🔑"
				styleLeft = func(s string) string { return explorerPK.Render(s) }
			case c.IsFK:
				icon = "➤"
				styleLeft = func(s string) string { return explorerFK.Render(s) }
			}
			left := "    " + icon + " " + r.Column
			line = explorerLineStyled(left, c.DataType, styleLeft, func(s string) string { return explorerType.Render(s) }, sidebarW)
		}
		if i+start == e.Cursor {
			line = explorerSel.Render(line)
		}
		lines = append(lines, line)
	}
	if height > 0 && len(lines) > height {
		lines = lines[:height]
	}
	if len(rows) > height {
		thumbH := height * height / len(rows)
		if thumbH < 1 {
			thumbH = 1
		}
		thumbStart := 0
		if span := len(rows) - height; span > 0 {
			thumbStart = start * (height - thumbH) / span
		}
		for i := range lines {
			if i >= height {
				break
			}
			ch, st := "│", scrollTrackStyle
			if i >= thumbStart && i < thumbStart+thumbH {
				ch, st = "█", scrollThumbStyle
			}
			lines[i] = setLastCell(lines[i], st.Render(ch), sidebarW)
		}
	}
	b.WriteString(strings.Join(lines, "\n"))
	return b.String()
}
