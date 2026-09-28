package tui

// Query history: every run is appended newest-first, and ctrl+p/ctrl+n
// walk back through this connection's statements.

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"db-peek/internal/saved"
)

// maskedConn is the key history is grouped by. It is the Mask()ed conn
// string, so the history file never sees a password.
func (m Model) maskedConn() string { return saved.Mask(m.connStr) }

// recordHistory appends sql to the history and returns the command that
// persists it. The write happens off the UI goroutine.
func (m *Model) recordHistory(sql string) tea.Cmd {
	if m.history == nil || strings.TrimSpace(sql) == "" {
		return nil
	}
	m.history.Add(sql, m.maskedConn())
	return saveHistoryCmd(m.history)
}

// historyPrev loads the previous entry for this connection into the
// editor. histIdx -1 means the user is editing live text, which is
// captured once so ctrl+n can restore it.
func (m *Model) historyPrev() bool {
	if m.history == nil {
		return false
	}
	entries := m.history.Recent(m.maskedConn(), saved.HistoryLimit)
	if len(entries) == 0 {
		return false
	}
	if m.histIdx < 0 {
		m.histOrig = m.editor.Text()
		m.histIdx = 0
	} else {
		if m.histIdx+1 >= len(entries) {
			return false
		}
		m.histIdx++
	}
	m.setEditorText(entries[m.histIdx].SQL)
	return true
}

// historyNext walks forward, restoring the original buffer past the
// newest entry. Returns false when there is nothing further.
func (m *Model) historyNext() bool {
	if m.history == nil || m.histIdx < 0 {
		return false
	}
	if m.histIdx == 0 {
		m.histIdx = -1
		m.setEditorText(m.histOrig)
		return true
	}
	m.histIdx--
	entries := m.history.Recent(m.maskedConn(), saved.HistoryLimit)
	if m.histIdx >= len(entries) {
		m.histIdx = -1
		m.setEditorText(m.histOrig)
		return true
	}
	m.setEditorText(entries[m.histIdx].SQL)
	return true
}

// setEditorText replaces the buffer and parks the cursor at the end.
func (m *Model) setEditorText(s string) {
	m.editor.SetText(s)
	m.editor.MoveToEnd()
	m.refreshCompletion()
}

// Any text edit leaves history-recall mode, so ctrl+p starts again from
// the newest entry instead of a stale index.
func (m *Model) resetHistoryCursor() {
	m.histIdx = -1
	m.histOrig = ""
}

// refreshCompletionAfterEdit is the single entry point for text edits: it
// rebuilds the popup and drops any history-recall cursor, so ctrl+p always
// restarts from the newest entry.
func (m *Model) refreshCompletionAfterEdit() {
	m.resetHistoryCursor()
	m.refreshCompletion()
}
