package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"

	"db-peek/internal/saved"
)

func (m *Model) openHistoryModal() bool {
	if m.history == nil {
		m.status = "no history available"
		return false
	}
	entries := m.history.Recent(m.maskedConn(), saved.HistoryLimit)
	if len(entries) == 0 {
		entries = m.history.All(saved.HistoryLimit)
	}
	if len(entries) == 0 {
		m.status = "history is empty"
		return false
	}

	m.historyModal = true
	m.historyEntries = entries
	m.historyFilterInput = textinput.New()
	m.historyFilterInput.Prompt = "/ "
	m.historyFilterInput.Focus()
	m.rebuildHistoryFiltered()
	m.historyCursor = 0
	m.historyOffset = 0
	return true
}

func (m *Model) rebuildHistoryFiltered() {
	query := strings.ToLower(strings.TrimSpace(m.historyFilterInput.Value()))
	m.historyFiltered = nil
	for i, e := range m.historyEntries {
		if query == "" || strings.Contains(strings.ToLower(e.SQL), query) {
			m.historyFiltered = append(m.historyFiltered, i)
		}
	}
	if m.historyCursor >= len(m.historyFiltered) {
		m.historyCursor = len(m.historyFiltered) - 1
	}
	if m.historyCursor < 0 {
		m.historyCursor = 0
	}
	m.clampHistoryOffset()
}

func (m *Model) clampHistoryOffset() {
	budget := m.historyBodyBudget()
	if m.historyCursor < m.historyOffset {
		m.historyOffset = m.historyCursor
	}
	if m.historyCursor >= m.historyOffset+budget {
		m.historyOffset = m.historyCursor - budget + 1
	}
	if m.historyOffset < 0 {
		m.historyOffset = 0
	}
}

func (m Model) historyBodyBudget() int {
	h := m.height - 10
	if h < 4 {
		return 4
	}
	if h > 14 {
		return 14
	}
	return h
}

func (m *Model) selectHistoryEntry() bool {
	if len(m.historyFiltered) == 0 || m.historyCursor < 0 || m.historyCursor >= len(m.historyFiltered) {
		return false
	}
	idx := m.historyFiltered[m.historyCursor]
	entry := m.historyEntries[idx]
	m.setEditorText(entry.SQL)
	m.historyModal = false
	m.status = "loaded query from history"
	return true
}

func (m *Model) copySelectedHistory() string {
	if len(m.historyFiltered) == 0 || m.historyCursor < 0 || m.historyCursor >= len(m.historyFiltered) {
		return "no query selected"
	}
	idx := m.historyFiltered[m.historyCursor]
	entry := m.historyEntries[idx]
	return m.copySelectionText(entry.SQL)
}

func (m Model) historyModalView() string {
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
	if termW < 50 {
		termW = 50
	}
	termH := m.height
	if termH < 14 {
		termH = 14
	}

	boxW := termW - 8
	if boxW > 96 {
		boxW = 96
	}
	if boxW < 40 {
		boxW = 40
	}

	innerW := boxW - 2
	budget := m.historyBodyBudget()

	var b strings.Builder
	title := fmt.Sprintf(" Query History (%d/%d) ", len(m.historyFiltered), len(m.historyEntries))
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n")
	b.WriteString(m.historyFilterInput.View())
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n")

	start := m.historyOffset
	end := start + budget
	if end > len(m.historyFiltered) {
		end = len(m.historyFiltered)
	}

	for i := range budget {
		idx := start + i
		if idx < end {
			entryIdx := m.historyFiltered[idx]
			entry := m.historyEntries[entryIdx]
			timeStr := entry.At.Format("01/02 15:04")
			sqlSnippet := strings.ReplaceAll(strings.ReplaceAll(entry.SQL, "\n", " "), "\t", " ")

			prefix := "  "
			isCursor := idx == m.historyCursor
			if isCursor {
				prefix = "> "
			}

			timeTag := dimStyle.Render(timeStr)
			availW := innerW - lipgloss.Width(prefix) - lipgloss.Width(timeStr) - 3
			if availW < 10 {
				availW = 10
			}
			sqlText := fitText(sqlSnippet, availW)

			line := fmt.Sprintf("%s%s  %s", prefix, timeTag, sqlText)
			if isCursor {
				b.WriteString(dataSelectedStyle.Render(fitText(line, innerW)))
			} else {
				b.WriteString(fitText(line, innerW))
			}
		}
		b.WriteString("\n")
	}

	b.WriteString(dimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(" ↑/↓ / ctrl+p/n: move • enter: load • y: copy • esc: close "))

	box := paneBorder(true).Render(b.String())
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
