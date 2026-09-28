package tui

// Session restore: db-peek writes session.json on quit and reads it back
// on the next launch. Only a saved profile *name* is recorded, so no new
// secret reaches disk; an ad-hoc connection restores nothing
// connection-wise and the picker opens as usual.

import (
	"strings"

	"db-peek/internal/saved"
)

// loadSessionFunc is a seam for tests, which must never read the
// developer's real session file.
var loadSessionFunc = saved.LoadSession

// applySession seeds the model from a previously saved session. It runs
// after refreshConns and before the first connect command.
func (m *Model) applySession(sess *saved.Session) {
	if sess == nil {
		return
	}
	if m.connStr == "" {
		m.connStr = m.restoreConn(sess)
	}
	for _, s := range pageSizes {
		if s == sess.PageSize {
			m.pageSize = s
			break
		}
	}
	m.qbufs = nil
	m.qcur = 0
	for _, text := range sess.Buffers {
		if len(m.qbufs) >= maxQueryBufs {
			break
		}
		m.qbufs = append(m.qbufs, queryBuffer{id: m.nextQbufID, editor: Editor{Lines: strings.Split(text, "\n")}})
		m.nextQbufID++
	}
	if len(m.qbufs) == 0 {
		m.qbufs = nil
	}
	m.qcur = sess.Cur
	if m.qcur < 0 || m.qcur >= len(m.qbufs) {
		m.qcur = 0
	}
	m.ensureQueryBufs()
	m.loadActiveBuf(m.qcur)
	// The explorer nodes do not exist yet: stash the selection and apply
	// it once the schemas load.
	m.restoreTable = sess.Table
	m.restoreTab = sess.Tab
	m.restoreFilter = sess.Filter
}

// restoreConn resolves the saved profile name to its connection string.
// The masked value must still match, so a profile edited since the last
// run does not silently reconnect somewhere else.
func (m *Model) restoreConn(sess *saved.Session) string {
	if sess.ConnProfile == "" || sess.ConnMasked == "" {
		return ""
	}
	p, ok := m.store.Get(sess.ConnProfile)
	if !ok {
		return ""
	}
	if saved.Mask(p.Conn) != sess.ConnMasked {
		return ""
	}
	return p.Conn
}

// saveSession records the state and writes it. Called once at quit.
func (m Model) saveSession(sess *saved.Session) {
	if sess == nil {
		return
	}
	m.fillSession(sess)
	_ = sess.Save()
}

// fillSession copies the restorable state into sess without writing it.
func (m Model) fillSession(sess *saved.Session) {
	sess.Table = m.table
	sess.Tab = m.tab
	sess.PageSize = m.pageSize
	sess.Filter = m.explorer.Filter
	sess.Cur = m.qcur
	sess.ConnMasked = saved.Mask(m.connStr)
	sess.ConnProfile = m.profileNameFor(m.connStr)
	bufs := m.qbufs
	if len(bufs) == 0 {
		bufs = []queryBuffer{{editor: m.editor}}
	}
	sess.Buffers = sess.Buffers[:0]
	for i := range bufs {
		if len(sess.Buffers) >= maxQueryBufs {
			break
		}
		sess.Buffers = append(sess.Buffers, strings.Join(bufs[i].editor.Lines, "\n"))
	}
}

// profileNameFor finds the saved profile a conn string came from, so the
// session can name it without storing the connection itself.
func (m Model) profileNameFor(connStr string) string {
	if connStr == "" || m.store == nil {
		return ""
	}
	masked := saved.Mask(connStr)
	for _, p := range m.store.List() {
		if saved.Mask(p.Conn) == masked {
			return p.Name
		}
	}
	return ""
}
