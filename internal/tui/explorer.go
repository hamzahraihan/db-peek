package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
}

func NewExplorer(connName string, schemas []string) Explorer {
	e := Explorer{ConnName: connName}
	for i, s := range schemas {
		e.Schemas = append(e.Schemas, SchemaNode{Name: s, Expanded: i == 0})
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

func (e *Explorer) Render(sidebarW, height int) string {
	rows := e.VisibleRows()
	var b strings.Builder
	b.WriteString(explorerTitle.Render("explorer") + "\n")
	conn := "● " + e.ConnName
	b.WriteString(explorerConn.Render(fitText(conn, sidebarW-2)) + "\n")
	var lines []string
	for i, r := range rows {
		var left, right string
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
			left = disc + " 🗄 " + r.Schema
			right = explorerCount.Render(fmt.Sprintf("%d", n))
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
		left = "  " + disc + " " + icon + " " + r.Table
		if tb.CountErr {
			right = explorerCount.Render("?")
		} else if tb.CountOK {
			right = explorerCount.Render(humanizeCount(tb.Count))
		} else {
			right = explorerCount.Render("…")
		}
		case RowColumn:
			c, _ := e.columnByName(r.Schema, r.Table, r.Column)
			icon := "◇"
			if c.IsPK {
				icon = "🔑"
			} else if c.IsFK {
				icon = "➤"
			}
			left = "    " + icon + " " + r.Column
			right = explorerType.Render(c.DataType)
		}
		gap := sidebarW - lipgloss.Width(left) - lipgloss.Width(right) - 1
		if gap < 1 {
			gap = 1
		}
		line := left + strings.Repeat(" ", gap) + right
		line = fitText(line, sidebarW)
		if i == e.Cursor {
			line = explorerSel.Render(line)
		}
		lines = append(lines, line)
	}
	if height > 0 && len(lines) > height {
		lines = lines[:height]
	}
	b.WriteString(strings.Join(lines, "\n"))
	return b.String()
}
