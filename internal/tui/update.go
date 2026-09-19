package tui

// Update is the Elm update function: (Model, Msg) -> (Model, Cmd).
// Message arrivals mutate state and kick off commands; key and mouse
// input delegate to per-screen handlers.

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	dbpkg "db-peek/internal/db"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeBrowse()
		m.conns.SetSize(msg.Width-4, m.contentH())
		return m, nil

	case connectMsg:
		m.loading = false
		if msg.err != nil {
			if m.db != nil {
				_ = m.db.Close()
				m.db = nil
			}
			m.err = msg.err.Error()
			m.screen = screenConns
			m.refreshConns()
			return m, nil
		}
		m.db = msg.db
		m.screen = screenBrowse
		m.clearConnState()
		m.focusDetail = false
		m.err = ""
		m.loading = true
		m.explorer = NewExplorer(m.db.Display, nil)
		return m, m.loadSchemas()

	case detailLoadedMsg:
		if msg.conn != m.connSeq {
			return m, nil // superseded by a connection switch
		}
		if msg.seq != m.detailSeq {
			return m, nil // superseded by a newer selection
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.table = msg.table
		m.cols = msg.cols
		m.indexes = msg.indexes
		m.sample = msg.sample
		m.count = msg.count
		m.page = 0
		m.err = ""
		m.buildTables()
		m.sizeTables()
		return m, nil

	case schemasLoadedMsg:
		if msg.conn != m.connSeq {
			return m, nil // superseded by a connection switch
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.explorer = NewExplorer(m.db.Display, msg.schemas)
		total := 0
		for si := range m.explorer.Schemas {
			refs := msg.tables[m.explorer.Schemas[si].Name]
			tbs := make([]TableNode, len(refs))
			for i, r := range refs {
				tbs[i] = TableNode{
					Schema: m.explorer.Schemas[si].Name,
					Name:   r.Name,
					IsView: r.IsView,
					Count:  -1,
				}
			}
			m.explorer.Schemas[si].Tables = tbs
			total += len(tbs)
		}
		m.status = fmt.Sprintf("%d tables • %s (%s)", total, m.db.Display, m.db.Driver)
		for _, s := range m.explorer.Schemas {
			for _, tb := range s.Tables {
				m2, inspectCmd := m.inspectTable(tb.Name)
				m = m2
				return m, tea.Batch(m.loadOneCount(s.Name, tb.Name), inspectCmd)
			}
		}
		return m, nil

	case tableCountMsg:
		if msg.conn != m.connSeq {
			return m, nil // superseded by a connection switch
		}
		for si := range m.explorer.Schemas {
			if m.explorer.Schemas[si].Name != msg.schema {
				continue
			}
			for ti := range m.explorer.Schemas[si].Tables {
				if m.explorer.Schemas[si].Tables[ti].Name != msg.table {
					continue
				}
				if msg.err != nil {
					m.explorer.Schemas[si].Tables[ti].CountErr = true
				} else {
					m.explorer.Schemas[si].Tables[ti].Count = msg.count
					m.explorer.Schemas[si].Tables[ti].CountOK = true
					m.explorer.Schemas[si].Tables[ti].CountErr = false
				}
			}
		}
		for _, s := range m.explorer.Schemas {
			for _, tb := range s.Tables {
				if !tb.CountOK && !tb.CountErr {
					return m, m.loadOneCount(s.Name, tb.Name)
				}
			}
		}
		return m, nil

	case columnsLoadedMsg:
		if msg.conn != m.connSeq {
			return m, nil // superseded by a connection switch
		}
		if msg.err != nil {
			m.err = msg.err.Error()
			for si := range m.explorer.Schemas {
				if m.explorer.Schemas[si].Name != msg.schema {
					continue
				}
				for ti := range m.explorer.Schemas[si].Tables {
					if m.explorer.Schemas[si].Tables[ti].Name == msg.table {
						m.explorer.Schemas[si].Tables[ti].Expanded = false
					}
				}
			}
			m.explorer.ensureVisible(m.sidebarTreeH())
			return m, nil
		}
		for si := range m.explorer.Schemas {
			if m.explorer.Schemas[si].Name != msg.schema {
				continue
			}
			for ti := range m.explorer.Schemas[si].Tables {
				if m.explorer.Schemas[si].Tables[ti].Name != msg.table {
					continue
				}
				cols := make([]ColumnNode, len(msg.columns))
				for i, c := range msg.columns {
					cols[i] = ColumnNode{
						Name:     c.Name,
						DataType: c.Type,
						IsPK:     strings.HasPrefix(c.Extra, "PK"),
						IsFK:     strings.HasSuffix(c.Name, "_id"),
					}
				}
				m.explorer.Schemas[si].Tables[ti].Columns = cols
			}
		}
		m.explorer.ensureVisible(m.sidebarTreeH())
		return m, nil

	case rowsPageMsg:
		if msg.conn != m.connSeq {
			return m, nil // superseded by a connection switch
		}
		if msg.seq != m.detailSeq {
			return m, nil // superseded by a newer selection
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.sample = msg.sample
		m.page = msg.page
		m.err = ""
		m.buildRowTable()
		m.sizeTables()
		return m, nil

	case queryDoneMsg:
		if msg.conn != m.connSeq {
			return m, nil // superseded by a connection switch
		}
		m.ensureQueryBufs()
		bufIdx := -1
		if msg.qbufID == 0 {
			// Legacy/test path without buffer id: route to active buffer
			// when the global seq matches.
			if msg.seq != m.querySeq {
				return m, nil // superseded by a newer run
			}
			bufIdx = m.qcur
			m.qbufs[bufIdx].seq = msg.seq
		} else {
			for i, b := range m.qbufs {
				if b.id == msg.qbufID {
					bufIdx = i
					break
				}
			}
			if bufIdx < 0 {
				return m, nil // buffer closed since the run
			}
			if msg.seq != m.qbufs[bufIdx].seq {
				return m, nil // superseded by a newer run on that buffer
			}
		}
		m.loading = false
		if msg.err != nil {
			errStr := msg.err.Error() + dbpkg.HintForError(m.connStr, msg.err)
			m.qbufs[bufIdx].errStr = errStr
			if bufIdx == m.qcur {
				m.err = errStr
			}
			return m, nil // keep editor text + old results
		}
		applyBuf := &m.qbufs[bufIdx]
		applyBuf.sample = msg.sample
		applyBuf.affected = msg.affected
		applyBuf.previewTable = ""
		applyBuf.ms = msg.ms
		applyBuf.errStr = ""
		if msg.affected >= 0 {
			if msg.isDDL {
				// DDL: no grid; refresh the schema tree so the new/dropped
				// table shows up. Keep the affected count for the footer.
				applyBuf.sample = nil
				applyBuf.previewTable = ""
				applyBuf.affected = msg.affected
				applyBuf.ms = msg.ms
				applyBuf.table.setData([]string{"rows"}, [][]string{{"(no rows)"}})
				if bufIdx == m.qcur {
					m.querySample = nil
					m.queryPreviewTable = ""
					m.queryAffected = msg.affected
					m.queryMs = msg.ms
					m.err = ""
					m.queryTable.setData([]string{"rows"}, [][]string{{"(no rows)"}})
					m.sizeTables()
				}
				m.loading = true
				return m, m.loadSchemas()
			}
			if msg.previewTable != "" && msg.sample != nil {
				applyBuf.sample = msg.sample
				applyBuf.previewTable = msg.previewTable
				applyBuf.affected = msg.affected
				applyBuf.ms = msg.ms
				applyBuf.errStr = ""
				var qcols []string
				var qrows [][]string
				qcols = append([]string(nil), msg.sample.Columns...)
				for _, r := range msg.sample.Rows {
					qrows = append(qrows, append([]string(nil), r...))
				}
				if len(qcols) == 0 {
					qcols = []string{"rows"}
					qrows = [][]string{{"(no rows)"}}
				}
				applyBuf.table.setData(qcols, qrows)
				applyBuf.table.SetHeaderStyles(dataFieldHeaderStyle, dimFieldHeaderStyle)
				if bufIdx == m.qcur {
					m.querySample = msg.sample
					m.queryPreviewTable = msg.previewTable
					m.queryAffected = msg.affected
					m.queryMs = msg.ms
					m.err = ""
					m.queryTable.setData(qcols, qrows)
					m.queryTable.SetHeaderStyles(dataFieldHeaderStyle, dimFieldHeaderStyle)
					m.sizeTables()
				}
				// Best-effort count refresh for the previewed table.
				for _, s := range m.explorer.Schemas {
					for _, tb := range s.Tables {
						if tb.Name == msg.previewTable {
							return m, m.loadOneCount(s.Name, tb.Name)
						}
					}
				}
				return m, nil
			}
			// Write path without preview: no grid, the view shows rows-affected instead.
			applyBuf.sample = nil
			applyBuf.previewTable = ""
			applyBuf.affected = msg.affected
			applyBuf.ms = msg.ms
			applyBuf.table.setData([]string{"rows"}, [][]string{{"(no rows)"}})
			if bufIdx == m.qcur {
				m.querySample = nil
				m.queryPreviewTable = ""
				m.queryAffected = msg.affected
				m.queryMs = msg.ms
				m.err = ""
				m.queryTable.setData([]string{"rows"}, [][]string{{"(no rows)"}})
				m.sizeTables()
			}
			return m, nil
		}
		var qcols []string
		var qrows [][]string
		if msg.sample != nil {
			qcols = append([]string(nil), msg.sample.Columns...)
			for _, r := range msg.sample.Rows {
				qrows = append(qrows, append([]string(nil), r...))
			}
		}
		if len(qcols) == 0 {
			qcols = []string{"rows"}
			qrows = [][]string{{"(no rows)"}}
		}
		applyBuf.table.setData(qcols, qrows)
		applyBuf.table.SetHeaderStyles(dataFieldHeaderStyle, dimFieldHeaderStyle)
		if bufIdx == m.qcur {
			m.querySample = msg.sample
			m.queryAffected = msg.affected
			m.queryPreviewTable = ""
			m.queryMs = msg.ms
			m.err = ""
			m.queryTable.setData(qcols, qrows)
			m.queryTable.SetHeaderStyles(dataFieldHeaderStyle, dimFieldHeaderStyle)
			m.sizeTables()
		}
		return m, nil

	case erLoadedMsg:
		if msg.conn != m.connSeq {
			return m, nil // superseded by a connection switch
		}
		if msg.seq != m.erSeq {
			return m, nil // superseded by a newer tab enter
		}
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		if m.erCache == nil {
			m.erCache = map[string][]dbpkg.ForeignKey{}
		}
		m.erCache[msg.table] = msg.links
		if msg.table == m.table {
			m.erLinks = msg.links
			m.erOffset = 0
			m.err = ""
		}
		return m, nil

	case erSchemaLoadedMsg:
		if msg.conn != m.connSeq {
			return m, nil // superseded by a connection switch
		}
		if msg.seq != m.erSeq {
			return m, nil // superseded
		}
		if msg.err != nil {
			if len(msg.tables) == 0 && len(msg.links) == 0 {
				// Total failure: stay on the legacy single-table
				// view, error on the footer.
				m.err = msg.err.Error()
				m.erSchema.err = msg.err.Error()
				return m, nil
			}
			// Partial failure (design §5): keep the usable boxes
			// + links with loaded:true and record the error in
			// erSchema.err, surfaced via the m.err footer line.
			m.erSchema = erSchemaState{tables: msg.tables, links: msg.links, loaded: true, err: msg.err.Error()}
			if m.erCenter == "" {
				m.erCenter = m.table
			}
			m.erSel = m.table
			if m.erSel == "" {
				m.erSel = m.erCenter
			}
			m.erPanX, m.erPanY = 0, 0
			m.err = msg.err.Error()
			return m, nil
		}
		m.erSchema = erSchemaState{tables: msg.tables, links: msg.links, loaded: true}
		if m.erCenter == "" {
			m.erCenter = m.table
		}
		m.erSel = m.table
		if m.erSel == "" {
			m.erSel = m.erCenter
		}
		m.erPanX, m.erPanY = 0, 0
		m.err = ""
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case tea.MouseClickMsg:
		return m.handleMouseClick(msg)
	case tea.MouseWheelMsg:
		// v2 reports wheel direction in Button; release events arrive
		// as MouseReleaseMsg and are ignored (as in v1).
		if msg.Button == tea.MouseWheelUp {
			return m.wheel(-3)
		}
		return m.wheel(3)
	case tea.MouseMotionMsg:
		return m.hover(msg.X, msg.Y)
	}
	// Route updates to the focused component.
	switch m.screen {
	case screenConns:
		var cmd tea.Cmd
		m.conns, cmd = m.conns.Update(msg)
		return m, cmd
	case screenForm:
		var cmd tea.Cmd
		if m.formFocus == 0 {
			m.nameInput, cmd = m.nameInput.Update(msg)
		} else {
			m.connInput, cmd = m.connInput.Update(msg)
		}
		return m, cmd
	}
	return m, nil
}
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		if m.db != nil {
			_ = m.db.Close()
		}
		return m, tea.Quit
	}

	// Which-key overlay is modal: while open every key except ?/esc
	// (which close it) is swallowed before screen dispatch.
	if m.showHelp {
		if key == "?" || key == "esc" {
			m.showHelp = false
		}
		return m, nil
	}
	// ? opens the overlay, except where ? is text: conns filter input,
	// form inputs, or the query editor (tab==3, editor focused) where
	// Postgres JSON operators need the rune.
	if key == "?" && m.helpToggleAllowed() {
		m.showHelp = true
		return m, nil
	}

	switch m.screen {
	case screenConns:
		return m.connsKey(msg, key)
	case screenForm:
		return m.formKey(msg, key)
	case screenBrowse:
		// q quits unless the sidebar filter is being typed into or the
		// query editor has focus (there it inserts the rune).
		if key == "q" && !m.filtering && m.explorer.Filter == "" && !(m.focusDetail && m.tab == 3 && m.queryFocus == 0) {
			if m.db != nil {
				_ = m.db.Close()
			}
			return m, tea.Quit
		}
		// tab jumps between sidebar and detail — unless the query editor
		// owns it: while typing a query, tab accepts a completion (or
		// indents) and shift+tab stays the pane-switch escape hatch.
		// Leaving the sidebar blurs the filter input (text stays applied).
		if key == "tab" || key == "shift+tab" {
			if key == "tab" && m.focusDetail && m.tab == 3 && m.queryFocus == 0 {
				return m.detailKey(msg, key)
			}
			if !m.focusDetail {
				m.filtering = false
				m.filterInput.Blur()
			}
			m.focusDetail = !m.focusDetail
			return m, nil
		}
		if !m.focusDetail {
			return m.sidebarKeys(msg, key)
		}
		return m.detailKey(msg, key)
	}
	return m, nil
}

// helpToggleAllowed reports whether ? may open the help overlay on the
// current screen. Typing contexts own the rune instead: the conns filter
// input, either form input, the sidebar filter input, and the focused
// query editor.
func (m Model) helpToggleAllowed() bool {
	switch m.screen {
	case screenConns:
		return !m.conns.SettingFilter()
	case screenForm:
		return !m.nameInput.Focused() && !m.connInput.Focused()
	case screenBrowse:
		return !(m.focusDetail && m.tab == 3 && m.queryFocus == 0) && !m.filtering
	}
	return true
}

// clearConnState drops every piece of per-connection UI state so the
// next connection never shows the previous database's tables, counts,
// columns, query results or ER diagram. Seq counters bump alongside so
// in-flight replies carrying the old detail/er/query seq are dropped
// even before their conn generation is checked.
func (m *Model) clearConnState() {
	m.table = ""
	m.cols = nil
	m.indexes = nil
	m.sample = nil
	m.count = -1
	m.page = 0
	m.detailSeq++
	m.querySeq++
	m.qbufs = nil
	m.qcur = 0
	m.ensureQueryBufs()
	m.qbufs[0].editor = NewEditor()
	m.qbufs[0].sample = nil
	m.qbufs[0].affected = -1
	m.qbufs[0].previewTable = ""
	m.qbufs[0].ms = 0
	m.qbufs[0].table = dataTable{}
	m.qbufs[0].errStr = ""
	m.qbufs[0].seq = m.querySeq
	m.loadActiveBuf(0)
	m.erSeq++
	m.explorer = NewExplorer("", nil)
	m.erSchema = erSchemaState{}
	m.erCache = nil
	m.erLinks = nil
	m.erOffset = 0
	m.erSel = ""
	m.erCenter = ""
	m.erPanX, m.erPanY = 0, 0
	m.hoverER = ""
	m.querySample = nil
	m.queryAffected = -1
	m.queryPreviewTable = ""
	m.queryMs = 0
	m.queryFocus = 0
	m.tab = 0
	m.hoverTab = -1
	m.status = ""
	m.err = ""
	m.filtering = false
	m.filterInput.Blur()
	m.filterInput.SetValue("")
}

// disconnect closes the database, drops its UI state, and returns to
// the picker. The conn generation bumps so late replies from the old
// connection are ignored.
func (m *Model) disconnect() {
	if m.db != nil {
		_ = m.db.Close()
		m.db = nil
	}
	m.connSeq++
	m.clearConnState()
	m.screen = screenConns
	m.focusDetail = false
	m.refreshConns()
}

// sidebarKeys handles keys on the browse sidebar (explorer is the source
// of truth). While filtering, every keystroke belongs to the filter input.
func (m Model) sidebarKeys(msg tea.KeyPressMsg, key string) (tea.Model, tea.Cmd) {
	if m.filtering {
		return m.filterKeys(msg, key)
	}
	switch key {
	case "up", "k":
		m.explorer.MoveUp()
		m.explorer.ensureVisible(m.sidebarTreeH())
		return m, nil
	case "down", "j":
		m.explorer.MoveDown()
		m.explorer.ensureVisible(m.sidebarTreeH())
		return m, nil
	case "left", "right":
		m.explorer.Toggle()
		m.explorer.ensureVisible(m.sidebarTreeH())
		return m, nil
	case "enter":
		row, ok := m.explorer.RowAt(m.explorer.Cursor)
		m.explorer.Toggle()
		if !ok {
			return m, nil
		}
		if row.Kind == RowColumn {
			for si := range m.explorer.Schemas {
				if m.explorer.Schemas[si].Name != row.Schema {
					continue
				}
				for ti := range m.explorer.Schemas[si].Tables {
					if m.explorer.Schemas[si].Tables[ti].Name == row.Table {
						m.explorer.Schemas[si].Tables[ti].Expanded = true
					}
				}
			}
			colCmd := m.loadColumns(row.Schema, row.Table)
			m2, inspectCmd := m.inspectTable(row.Table)
			m = m2
			return m, tea.Batch(colCmd, inspectCmd)
		}
		if row.Kind != RowTable {
			return m, nil
		}
		colCmd := m.loadColumns(row.Schema, row.Table)
		m2, inspectCmd := m.inspectTable(row.Table)
		m = m2
		return m, tea.Batch(colCmd, inspectCmd)
	case "/":
		// Open the filter input, keeping any existing text for refinement.
		m.filtering = true
		m.filterInput.SetValue(m.explorer.Filter)
		return m, m.filterInput.Focus()
	case "r":
		m.loading = true
		return m, m.loadSchemas()
	case "c":
		m.disconnect()
		return m, nil
	case "esc":
		m.disconnect()
		return m, nil
	}
	return m, nil
}

// filterKeys handles keys while the sidebar filter input is focused.
// Every keystroke belongs to the filter: single-letter sidebar actions
// (r/c/...) must not hijack typing. esc exits and clears, enter applies
// the text and exits, up/down navigate without exiting.
func (m Model) filterKeys(msg tea.KeyPressMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.filtering = false
		m.filterInput.Blur()
		m.filterInput.SetValue("")
		m.explorer.SetFilter("")
		return m, nil
	case "enter":
		m.filtering = false
		m.filterInput.Blur()
		return m, nil
	case "up":
		m.explorer.MoveUp()
		m.explorer.ensureVisible(m.sidebarTreeH())
		return m, nil
	case "down":
		m.explorer.MoveDown()
		m.explorer.ensureVisible(m.sidebarTreeH())
		return m, nil
	}
	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	m.explorer.SetFilter(m.filterInput.Value())
	return m, cmd
}

// detailKey handles keys on the schema/indexes/rows/query/er tabs.
// Tabs 3 (query) and 4 (er) route to their own handlers first so editing
// runes never trigger the schema/indexes/rows bindings below.
func (m Model) detailKey(msg tea.KeyPressMsg, key string) (tea.Model, tea.Cmd) {
	if m.tab == 3 {
		return m.queryKeys(msg, key)
	}
	if m.tab == 4 {
		return m.erKeys(msg, key)
	}
	switch key {
	case "esc", "backspace":
		m.focusDetail = false
		m.err = ""
		m.hoverTab = -1
		return m, nil
	case "tab", "right", "l":
		cmd := m.setTab((m.tab + 1) % 5)
		return m, cmd
	case "shift+tab", "left", "h":
		cmd := m.setTab((m.tab + 4) % 5)
		return m, cmd
	case "1":
		cmd := m.setTab(0)
		return m, cmd
	case "2":
		cmd := m.setTab(1)
		return m, cmd
	case "3":
		cmd := m.setTab(2)
		return m, cmd
	case "4":
		cmd := m.setTab(3)
		return m, cmd
	case "5":
		cmd := m.setTab(4)
		return m, cmd
	case "r":
		m.loading = true
		return m, m.loadDetail(m.table)
	case "n":
		if m.tab == 2 && m.hasNextPage() {
			m.page++
			m.loading = true
			return m, m.loadRowsPage()
		}
		return m, nil
	case "p":
		if m.tab == 2 && m.page > 0 {
			m.page--
			m.loading = true
			return m, m.loadRowsPage()
		}
		return m, nil
	case "s":
		if m.tab == 2 {
			m.pageSize = nextPageSize(m.pageSize)
			m.page = 0
			m.loading = true
			return m, m.loadRowsPage()
		}
		return m, nil
	}
	// Grid movement only applies to tabs 0-2 (tabs 3/4 route above).
	if m.tab < 3 {
		g := m.activeGrid()
		switch key {
		case "up", "k":
			g.MoveUp(1)
		case "down", "j":
			g.MoveDown(1)
		case "pgup":
			g.MoveUp(g.Height())
		case "pgdown":
			g.MoveDown(g.Height())
		case "ctrl+u":
			g.MoveUp(g.Height() / 2)
		case "ctrl+d":
			g.MoveDown(g.Height() / 2)
		case "home", "g":
			g.GotoTop()
		case "end", "G":
			g.GotoBottom()
		}
	}
	return m, nil
}

// queryKeys handles keys on the query tab (tab 3). With the editor
// focused every typed rune inserts; esc steps focus editor → results →
// sidebar. With results focused the grid moves and 1-5/r switch/rerun.
func (m Model) queryKeys(msg tea.KeyPressMsg, key string) (tea.Model, tea.Cmd) {
	if m.queryFocus == 0 {
		// Autocomplete popup takes priority: Tab/Enter accept, Esc
		// dismisses (a second Esc then drops to results), Up/Down move
		// the selection instead of the editor cursor.
		if m.showComplete {
			switch key {
			case "shift+up", "shift+down":
				m.showComplete = false
				m.completeItems = nil
				m.completeIdx = 0
				if key == "shift+up" {
					m.editor.ExtendSelectionTo(m.editor.CurLine - 1)
				} else {
					m.editor.ExtendSelectionTo(m.editor.CurLine + 1)
				}
				m.clampEditorScroll()
				return m, nil
			case "esc":
				m.showComplete = false
				m.completeItems = nil
				m.completeIdx = 0
				return m, nil
			case "tab", "enter":
				if m.completeIdx >= 0 && m.completeIdx < len(m.completeItems) {
					it := m.completeItems[m.completeIdx]
					if ln := m.editor.CurLine; ln >= 0 && ln < len(m.editor.Lines) {
						newLine, newCol := applyCompletion(m.editor.Lines[ln], m.editor.CurCol, it)
						m.editor.Lines[ln] = newLine
						m.editor.CurCol = newCol
					}
				}
				m.showComplete = false
				m.completeItems = nil
				m.completeIdx = 0
				m.clampEditorScroll()
				return m, nil
			case "up":
				if m.completeIdx > 0 {
					m.completeIdx--
				}
				return m, nil
			case "down":
				if m.completeIdx < len(m.completeItems)-1 {
					m.completeIdx++
				}
				return m, nil
			case "ctrl+r", "f5":
				m.showComplete = false
				m.completeItems = nil
				m.completeIdx = 0
				m.startQueryRun()
				return m, m.runQuery()
			case "ctrl+t":
				m.showComplete = false
				m.completeItems = nil
				m.completeIdx = 0
				m.saveActiveBuf()
				m.newQueryBuf()
				m.refreshCompletion()
				return m, nil
			case "ctrl+w":
				m.showComplete = false
				m.completeItems = nil
				m.completeIdx = 0
				m.closeQueryBuf()
				m.refreshCompletion()
				return m, nil
			}
		}
		switch key {
		case "esc":
			if _, _, active := m.editor.SelectedRange(); active {
				m.editor.ClearSelection()
				return m, nil
			}
			m.queryFocus = 1
			return m, nil
		case "ctrl+s":
			if txt := m.editor.SelectionText(); txt != "" {
				m.status = m.copySelectionText(txt)
				return m, nil
			}
			m.status = m.copyQueryDump()
			return m, nil
		case "tab":
			// No popup open (the open case returns above): indent with
			// two spaces. SQL ignores the extra whitespace.
			m.editor.ClearSelection()
			m.editor.Insert(' ')
			m.editor.Insert(' ')
			m.clampEditorScroll()
			m.refreshCompletion()
			return m, nil
		case "shift+up":
			m.editor.ExtendSelectionTo(m.editor.CurLine - 1)
			m.clampEditorScroll()
			m.refreshCompletion()
			return m, nil
		case "shift+down":
			m.editor.ExtendSelectionTo(m.editor.CurLine + 1)
			m.clampEditorScroll()
			m.refreshCompletion()
			return m, nil
		case "ctrl+d":
			if _, _, active := m.editor.SelectedRange(); active {
				m.editor.DeleteRange()
			} else {
				// Delete current line without clipboard (spec: not a cut).
				m.editor.DeleteCurrentLine()
			}
			m.clampEditorScroll()
			m.refreshCompletion()
			return m, nil
		case "ctrl+/", "ctrl+_":
			m.editor.ToggleCommentRange()
			m.clampEditorScroll()
			m.refreshCompletion()
			return m, nil
		case "ctrl+r", "f5":
			m.startQueryRun()
			return m, m.runQuery()
		case "ctrl+t":
			m.saveActiveBuf()
			m.newQueryBuf()
			m.refreshCompletion()
			return m, nil
		case "ctrl+w":
			m.closeQueryBuf()
			m.refreshCompletion()
			return m, nil
		case "enter":
			m.editor.ClearSelection()
			m.editor.Newline()
			m.clampEditorScroll()
			m.refreshCompletion()
			return m, nil
		case "backspace":
			m.editor.ClearSelection()
			m.editor.Backspace()
			m.clampEditorScroll()
			m.refreshCompletion()
			return m, nil
		case "delete":
			m.editor.ClearSelection()
			m.editor.Delete()
			m.refreshCompletion()
			return m, nil
		case "up":
			m.editor.ClearSelection()
			m.editor.MoveUp()
			m.clampEditorScroll()
			m.refreshCompletion()
			return m, nil
		case "down":
			m.editor.ClearSelection()
			m.editor.MoveDown()
			m.clampEditorScroll()
			m.refreshCompletion()
			return m, nil
		case "left":
			m.editor.ClearSelection()
			m.editor.MoveLeft()
			m.clampEditorScroll()
			m.refreshCompletion()
			return m, nil
		case "right":
			m.editor.ClearSelection()
			m.editor.MoveRight()
			m.clampEditorScroll()
			m.refreshCompletion()
			return m, nil
		case "home":
			m.editor.ClearSelection()
			m.editor.Home()
			m.refreshCompletion()
			return m, nil
		case "end":
			m.editor.ClearSelection()
			m.editor.End()
			m.refreshCompletion()
			return m, nil
		}
		// Every rune inserts while editing, including "?" (Postgres JSON
		// operators need it) and " " (v2 reports it as "space" via
		// String(), but Text still carries the printable characters).
		// Task 7's "?"-overlay must intercept "?" before detailKey
		// routing except when the editor is focused.
		if msg.Text != "" {
			m.editor.ClearSelection()
			for _, r := range msg.Text {
				m.editor.Insert(r)
			}
			m.clampEditorScroll()
			m.refreshCompletion()
			return m, nil
		}
		return m, nil
	}
	switch key {
	case "esc", "backspace":
		m.focusDetail = false
		m.err = ""
		m.hoverTab = -1
		return m, nil
	case "tab", "right", "l":
		cmd := m.setTab((m.tab + 1) % 5)
		return m, cmd
	case "shift+tab", "left", "h":
		cmd := m.setTab((m.tab + 4) % 5)
		return m, cmd
	case "1":
		cmd := m.setTab(0)
		return m, cmd
	case "2":
		cmd := m.setTab(1)
		return m, cmd
	case "3":
		cmd := m.setTab(2)
		return m, cmd
	case "4":
		cmd := m.setTab(3)
		return m, cmd
	case "5":
		cmd := m.setTab(4)
		return m, cmd
	case "ctrl+r", "f5", "r":
		m.startQueryRun()
		return m, m.runQuery()
	case "ctrl+t", "t":
		m.saveActiveBuf()
		m.newQueryBuf()
		return m, nil
	case "ctrl+w", "X":
		m.closeQueryBuf()
		return m, nil
	case "H":
		m.switchQueryBuf(-1)
		return m, nil
	case "L":
		m.switchQueryBuf(1)
		return m, nil
	case "ctrl+s":
		if txt := m.editor.SelectionText(); txt != "" {
			m.status = m.copySelectionText(txt)
			return m, nil
		}
		m.status = m.copyQueryDump()
		return m, nil
	case "e":
		m.queryFocus = 0
		m.clampEditorScroll()
		return m, nil
	}
	g := &m.queryTable
	switch key {
	case "up", "k":
		g.MoveUp(1)
	case "down", "j":
		g.MoveDown(1)
	case "pgup":
		g.MoveUp(g.Height())
	case "pgdown":
		g.MoveDown(g.Height())
	case "ctrl+u":
		g.MoveUp(g.Height() / 2)
	case "ctrl+d":
		g.MoveDown(g.Height() / 2)
	case "home", "g":
		g.GotoTop()
	case "end", "G":
		g.GotoBottom()
	}
	return m, nil
}

// erKeys handles keys on the er tab (tab 4): focused 1-hop diagram by
// default (f toggles the full schema), pan via arrows/WASD, n/p to cycle
// visible boxes, enter to recenter on the selection (stays on ER), r to
// reload the schema.
func (m Model) erKeys(_ tea.KeyPressMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "backspace":
		m.focusDetail = false
		m.err = ""
		m.hoverTab = -1
		return m, nil
	case "tab", "right", "l":
		cmd := m.setTab((m.tab + 1) % 5)
		return m, cmd
	case "shift+tab", "left", "h":
		cmd := m.setTab((m.tab + 4) % 5)
		return m, cmd
	case "1":
		cmd := m.setTab(0)
		return m, cmd
	case "2":
		cmd := m.setTab(1)
		return m, cmd
	case "3":
		cmd := m.setTab(2)
		return m, cmd
	case "4":
		cmd := m.setTab(3)
		return m, cmd
	case "5":
		cmd := m.setTab(4)
		return m, cmd
	case "up", "k", "w":
		if m.erPanY > 0 {
			m.erPanY--
		}
		return m, nil
	case "down", "j", "s":
		m.erPanY++
		return m, nil
	case "a":
		if m.erPanX > 0 {
			m.erPanX -= 2
		}
		return m, nil
	case "d":
		m.erPanX += 2
		return m, nil
	case "pgup":
		m.erPanY -= 10
		if m.erPanY < 0 {
			m.erPanY = 0
		}
		return m, nil
	case "pgdown":
		m.erPanY += 10
		return m, nil
	case "n":
		m.erSel = erNextBox(m.erVisibleTables(), m.erSel, 1)
		return m, nil
	case "p":
		m.erSel = erNextBox(m.erVisibleTables(), m.erSel, -1)
		return m, nil
	case "f":
		m.erFocus = !m.erFocus
		if m.erFocus && m.erCenter == "" {
			m.erCenter = m.erSel
			if m.erCenter == "" {
				m.erCenter = m.table
			}
		}
		if m.erSel == "" {
			m.erSel = m.erCenter
		}
		m.erPanX, m.erPanY = 0, 0
		return m, nil
	case "enter":
		if m.erSel != "" {
			return m.recenterER(m.erSel)
		}
		return m, nil
	case "r":
		m.erSchema.loaded = false
		m.erSeq++
		var names []string
		for _, t := range m.erSchema.tables {
			names = append(names, t.name)
		}
		if len(names) == 0 && m.table != "" {
			names = []string{m.table}
		}
		return m, m.loadERSchema("", names)
	}
	if key == "tab" {
		return m, nil
	}
	return m, nil
}

// clampEditorScroll keeps the cursor inside the 8-row editor viewport.
func (m *Model) clampEditorScroll() {
	if m.editor.CurLine < m.editor.OffY {
		m.editor.OffY = m.editor.CurLine
	}
	if m.editor.CurLine >= m.editor.OffY+queryEditorH {
		m.editor.OffY = m.editor.CurLine - queryEditorH + 1
	}
	if m.editor.OffY < 0 {
		m.editor.OffY = 0
	}
}

// activeGrid returns the detail tab's grid for cursor movement.
func (m *Model) activeGrid() *dataTable {
	switch m.tab {
	case 0:
		return &m.colTable
	case 1:
		return &m.idxTable
	default:
		return &m.rowTable
	}
}

// activateConn connects to a saved profile; shared by enter-key and double-click.
func (m Model) activateConn(name string) (Model, tea.Cmd) {
	if m.db != nil {
		_ = m.db.Close()
		m.db = nil
	}
	m.connSeq++
	if p, ok := m.store.Get(name); ok {
		m.connStr = p.Conn
	}
	m.delArm = ""
	m.screen = screenBrowse
	m.focusDetail = false
	m.loading = true
	m.err = ""
	return m, m.openSaved(name)
}

// recenterER moves the focused diagram to a new center table and stays
// on the ER tab (enter/double-click). Detail columns reload in the
// background so the next tab switch is fresh; the seq guard drops stale
// replies.
func (m Model) recenterER(name string) (Model, tea.Cmd) {
	if name == "" {
		return m, nil
	}
	m.erFocus = true
	m.erCenter = name
	m.erSel = name
	m.hoverER = ""
	m.erPanX, m.erPanY = 0, 0
	m.table = name
	m.hoverTab = -1
	m.err = ""
	if m.db == nil {
		return m, nil
	}
	m.loading = true
	m.page = 0
	m.detailSeq++
	return m, m.loadDetail(name)
}

// inspectTable previews one table in the detail pane; shared by sidebar
// enter/click. The seq guard drops replies from superseded selections.
// Previewing blurs the filter input (the text stays applied) but never
// steals pane focus: sidebar previews stay in the sidebar (Tab jumps to
// detail), detail-initiated previews keep detail focus.
func (m Model) inspectTable(name string) (Model, tea.Cmd) {
	m.filtering = false
	m.filterInput.Blur()
	m.table = name
	m.tab = 0
	m.loading = true
	m.err = ""
	m.page = 0 // pageSize persists across tables
	m.hoverTab = -1
	m.detailSeq++
	return m, m.loadDetail(name)
}
