package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

func (m *Model) openCellModal() bool {
	var (
		cols []string
		raw  [][]any
		row  int
		col  int
	)

	switch {
	case m.tab == 2: // Rows tab
		if m.sample == nil {
			return false
		}
		cols = m.sample.Columns
		raw = m.sample.Raw
		row = m.rowTable.Cursor()
		col = m.rowTable.Col()
	case m.tab == 3 && m.queryFocus == 1: // Query results
		if m.querySample == nil {
			return false
		}
		cols = m.querySample.Columns
		raw = m.querySample.Raw
		row = m.queryTable.Cursor()
		col = m.queryTable.Col()
	default:
		return false
	}

	if row < 0 || row >= len(raw) || col < 0 || col >= len(cols) || col >= len(raw[row]) {
		return false
	}

	m.cellModal = true
	m.cellModalCol = cols[col]
	m.cellModalVal = formatCellRaw(raw[row][col])
	m.cellModalOffset = 0
	return true
}

func formatCellRaw(v any) string {
	if v == nil {
		return "(NULL)"
	}
	switch val := v.(type) {
	case string:
		trimmed := strings.TrimSpace(val)
		if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
			(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
			var out bytes.Buffer
			if err := json.Indent(&out, []byte(trimmed), "", "  "); err == nil {
				return out.String()
			}
		}
		return val
	case []byte:
		trimmed := bytes.TrimSpace(val)
		if (bytes.HasPrefix(trimmed, []byte("{")) && bytes.HasSuffix(trimmed, []byte("}"))) ||
			(bytes.HasPrefix(trimmed, []byte("[")) && bytes.HasSuffix(trimmed, []byte("]"))) {
			var out bytes.Buffer
			if err := json.Indent(&out, trimmed, "", "  "); err == nil {
				return out.String()
			}
		}
		return string(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func (m Model) cellModalView() string {
	var base string
	switch m.screen {
	case screenConns:
		base = m.connsView()
	case screenForm:
		base = m.formView()
	default:
		base = m.browseView()
	}

	termW := m.width
	if termW < 40 {
		termW = 40
	}
	termH := m.height
	if termH < 12 {
		termH = 12
	}

	boxW := termW - 8
	if boxW > 90 {
		boxW = 90
	}
	if boxW < 30 {
		boxW = 30
	}

	boxH := termH - 6
	if boxH > 25 {
		boxH = 25
	}
	if boxH < 8 {
		boxH = 8
	}

	innerW := boxW - 2 // border
	innerH := boxH - 2

	header := titleStyle.Render(fmt.Sprintf(" Value: %s ", m.cellModalCol))
	footer := dimStyle.Render(" j/k: scroll • c: copy • esc/q/enter: close ")

	valLines := strings.Split(m.cellModalVal, "\n")
	bodyBudget := innerH - 2 // header row + footer row
	if bodyBudget < 1 {
		bodyBudget = 1
	}

	maxOffset := len(valLines) - bodyBudget
	if maxOffset < 0 {
		maxOffset = 0
	}
	offset := m.cellModalOffset
	if offset > maxOffset {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}

	end := offset + bodyBudget
	if end > len(valLines) {
		end = len(valLines)
	}

	contentRows := valLines[offset:end]
	var body strings.Builder
	body.WriteString(fitText(header, innerW))
	body.WriteString("\n")

	for i := range bodyBudget {
		if i < len(contentRows) {
			body.WriteString(fitText(contentRows[i], innerW))
		}
		body.WriteString("\n")
	}
	body.WriteString(fitText(footer, innerW))

	box := paneBorder(true).Render(body.String())
	boxLines := strings.Split(box, "\n")
	baseLines := strings.Split(base, "\n")

	actualH := len(boxLines)
	actualW := 0
	for _, l := range boxLines {
		if w := lipgloss.Width(l); w > actualW {
			actualW = w
		}
	}

	x := (termW - actualW) / 2
	if x < 0 {
		x = 0
	}
	y := (termH - actualH) / 2
	if y < 0 {
		y = 0
	}

	for i := range actualH {
		lineIdx := y + i
		if lineIdx >= len(baseLines) {
			break
		}
		baseLines[lineIdx] = spliceCells(baseLines[lineIdx], boxLines[i], x, actualW, termW)
	}

	return strings.Join(baseLines, "\n")
}
