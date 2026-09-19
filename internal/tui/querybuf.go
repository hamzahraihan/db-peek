package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	dbpkg "db-peek/internal/db"
)

// Query buffers: one editor + results per buffer so a new query never
// requires erasing the current one. The Model keeps active working-copy
// fields (editor/querySample/.../queryTable); qbufs holds all buffers
// with deep-copied lines/rows so switching never aliases state.

const maxQueryBufs = 5

type queryBuffer struct {
	id           int
	editor       Editor
	sample       *dbpkg.Sample
	affected     int64
	previewTable string
	ms           int64
	table        dataTable
	seq          int
	errStr       string
}

func copyLines(in []string) []string {
	if in == nil {
		return []string{""}
	}
	out := make([]string, len(in))
	copy(out, in)
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

func copyTable(in dataTable) dataTable {
	out := in
	out.cols = append([]string(nil), in.cols...)
	out.widths = append([]int(nil), in.widths...)
	out.rows = make([][]string, len(in.rows))
	for i, r := range in.rows {
		out.rows[i] = append([]string(nil), r...)
	}
	out.colStyles = append([]lipgloss.Style(nil), in.colStyles...)
	out.dimColStyles = append([]lipgloss.Style(nil), in.dimColStyles...)
	return out
}

// ensureQueryBufs initializes the buffer list from the active working
// copy when empty (fresh Model or older state).
func (m *Model) ensureQueryBufs() {
	if len(m.qbufs) > 0 {
		if m.qcur < 0 {
			m.qcur = 0
		}
		if m.qcur >= len(m.qbufs) {
			m.qcur = len(m.qbufs) - 1
		}
		return
	}
	m.qbufs = []queryBuffer{{
		id:           1,
		editor:       Editor{Lines: copyLines(m.editor.Lines), CurLine: m.editor.CurLine, CurCol: m.editor.CurCol, OffY: m.editor.OffY},
		sample:       m.querySample,
		affected:     m.queryAffected,
		previewTable: m.queryPreviewTable,
		ms:           m.queryMs,
		table:        copyTable(m.queryTable),
		seq:          m.querySeq,
		errStr:       m.err,
	}}
	m.qcur = 0
	m.nextQbufID = 2
	if m.qbufs[0].editor.Lines == nil {
		m.qbufs[0].editor.Lines = []string{""}
	}
}

// saveActiveBuf copies the working copy into the current slot.
func (m *Model) saveActiveBuf() {
	m.ensureQueryBufs()
	b := &m.qbufs[m.qcur]
	b.editor.Lines = copyLines(m.editor.Lines)
	b.editor.CurLine, b.editor.CurCol, b.editor.OffY = m.editor.CurLine, m.editor.CurCol, m.editor.OffY
	b.sample = m.querySample
	b.affected = m.queryAffected
	b.previewTable = m.queryPreviewTable
	b.ms = m.queryMs
	b.table = copyTable(m.queryTable)
	b.errStr = m.err
}

// querySeqForBuf returns the expected seq for a buffer slot.
func (m *Model) querySeqForBuf(idx int) int {
	if idx < 0 || idx >= len(m.qbufs) {
		return m.querySeq
	}
	return m.qbufs[idx].seq
}

// loadActiveBuf copies slot idx into the working copy and refits.
func (m *Model) loadActiveBuf(idx int) {
	m.ensureQueryBufs()
	if idx < 0 {
		idx = 0
	}
	if idx >= len(m.qbufs) {
		idx = len(m.qbufs) - 1
	}
	m.qcur = idx
	b := m.qbufs[idx]
	m.editor.Lines = copyLines(b.editor.Lines)
	m.editor.CurLine, m.editor.CurCol, m.editor.OffY = b.editor.CurLine, b.editor.CurCol, b.editor.OffY
	m.editor.ClearSelection()
	m.querySample = b.sample
	m.queryAffected = b.affected
	m.queryPreviewTable = b.previewTable
	m.queryMs = b.ms
	m.queryTable = copyTable(b.table)
	m.err = b.errStr
	m.showComplete = false
	m.completeItems = nil
	m.completeIdx = 0
	m.clampEditorScroll()
	m.sizeTables()
}

// newQueryBuf appends a fresh empty buffer (cap maxQueryBufs) and focuses
// its editor, preserving the current buffer first.
func (m *Model) newQueryBuf() {
	m.ensureQueryBufs()
	if len(m.qbufs) >= maxQueryBufs {
		m.status = fmt.Sprintf("max %d query buffers", maxQueryBufs)
		return
	}
	m.saveActiveBuf()
	m.qbufs = append(m.qbufs, queryBuffer{
		id:       m.nextQbufID,
		editor:   NewEditor(),
		affected: -1,
	})
	m.nextQbufID++
	m.loadActiveBuf(len(m.qbufs) - 1)
	m.queryFocus = 0
	m.status = fmt.Sprintf("query %d/%d", m.qcur+1, len(m.qbufs))
}

// closeQueryBuf removes the current buffer; the last one clears instead
// of deleting so there is always an editor to type into.
func (m *Model) closeQueryBuf() {
	m.ensureQueryBufs()
	if len(m.qbufs) == 1 {
		m.qbufs[0].editor = NewEditor()
		m.qbufs[0].sample = nil
		m.qbufs[0].affected = -1
		m.qbufs[0].previewTable = ""
		m.qbufs[0].ms = 0
		m.qbufs[0].table = dataTable{}
		m.qbufs[0].errStr = ""
		m.qbufs[0].seq++
		m.loadActiveBuf(0)
		m.queryFocus = 0
		m.status = "query cleared"
		return
	}
	m.qbufs = append(m.qbufs[:m.qcur], m.qbufs[m.qcur+1:]...)
	if m.qcur >= len(m.qbufs) {
		m.qcur = len(m.qbufs) - 1
	}
	m.loadActiveBuf(m.qcur)
	m.queryFocus = 1
	m.status = fmt.Sprintf("closed • query %d/%d", m.qcur+1, len(m.qbufs))
}

// switchQueryBuf moves focus by delta, wrapping around.
func (m *Model) switchQueryBuf(delta int) {
	m.ensureQueryBufs()
	if len(m.qbufs) < 2 {
		return
	}
	m.saveActiveBuf()
	n := len(m.qbufs)
	next := (m.qcur + delta) % n
	if next < 0 {
		next += n
	}
	m.loadActiveBuf(next)
	// Stay in results (normal mode): the caller came from H/L.
	m.queryFocus = 1
	m.status = fmt.Sprintf("query %d/%d", m.qcur+1, len(m.qbufs))
}

// startQueryRun bumps the global + buffer seq for a new run on the
// active buffer. The run captures buffer id + seq so late replies for
// other buffers never paint the wrong results.
func (m *Model) startQueryRun() {
	m.ensureQueryBufs()
	m.saveActiveBuf()
	m.querySeq++
	m.qbufs[m.qcur].seq = m.querySeq
	m.qbufs[m.qcur].errStr = ""
	m.loading = true
	m.err = ""
}

// queryBufTitle derives a short label from the first non-empty line.
func queryBufTitle(b queryBuffer, idx int) string {
	for _, ln := range b.editor.Lines {
		t := strings.TrimSpace(ln)
		if t == "" || strings.HasPrefix(t, "--") {
			continue
		}
		t = strings.Join(strings.Fields(t), " ")
		if len([]rune(t)) > 14 {
			t = string([]rune(t)[:13]) + "…"
		}
		return fmt.Sprintf("%d %s", idx+1, t)
	}
	return fmt.Sprintf("%d query", idx+1)
}

// queryStripView renders the buffer strip; it replaces the blank row
// after the tab strip for tab==3 so editor Y math never shifts.
// Labels fall back to compact numbers when the full titles overflow,
// so ANSI styles are never cut mid-sequence by fitText.
func (m Model) queryStripView(w int) string {
	render := func(compact bool) (string, int) {
		parts := make([]string, 0, len(m.qbufs)+1)
		width := 0
		for i, b := range m.qbufs {
			title := queryBufTitle(b, i)
			if compact {
				title = fmt.Sprintf("%d", i+1)
			}
			label := "[" + title + "]"
			width += len([]rune(label)) + 1
			if i == m.qcur {
				parts = append(parts, activeTab.Render(label))
			} else {
				parts = append(parts, inactiveTab.Render(label))
			}
		}
		if len(m.qbufs) < maxQueryBufs {
			width += 4
			parts = append(parts, dimStyle.Render("[+]"))
		}
		return strings.Join(parts, " "), width
	}
	s, width := render(false)
	if w > 0 && width > w {
		s, _ = render(true)
	}
	return s
}
