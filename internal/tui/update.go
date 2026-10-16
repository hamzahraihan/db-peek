package tui

// Update is the Elm update function: (Model, Msg) -> (Model, Cmd).
// Message arrivals mutate state and kick off commands; key and mouse
// input delegate to per-screen handlers.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	dbpkg "peekerdb/internal/db"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeBrowse()
		m.conns.SetSize(msg.Width-4, m.contentH())
		return m, nil

	case historySavedMsg:
		if msg.err != nil {
			// A history write failure must never look like a query error.
			m.status = "history not saved: " + msg.err.Error()
		}
		return m, nil

	case cellUpdatedMsg:
		if msg.conn != m.connSeq || msg.seq != m.detailSeq {
			return m, nil // a different table (or connection) is open
		}
		m.loading = false
		if errors.Is(msg.err, context.Canceled) {
			return m, nil
		}
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.status = fmt.Sprintf("updated %s.%s", msg.table, msg.col)
		return m, m.loadRowsPage()

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
		m.explorerGen++
		return m, m.loadSchemas()

	case detailLoadedMsg:
		if msg.conn != m.connSeq {
			return m, nil // superseded by a connection switch
		}
		if msg.seq != m.detailSeq {
			return m, nil // superseded by a newer selection
		}
		m.loading = false
		if errors.Is(msg.err, context.Canceled) {
			return m, nil // aborted on purpose (esc)
		}
		// Populate the explorer's column children for this table. This
		// replaces the separate loadColumns round-trip: loadDetail
		// already fetched them.
		if msg.schema != "" {
			for si := range m.explorer.Schemas {
				if m.explorer.Schemas[si].Name != msg.schema {
					continue
				}
				for ti := range m.explorer.Schemas[si].Tables {
					node := &m.explorer.Schemas[si].Tables[ti]
					if node.Name != msg.table {
						continue
					}
					switch {
					case msg.colsErr != nil:
						// Column failure: collapse, as the old
						// separate loadColumns round-trip did.
						node.Expanded = false
					case msg.cols != nil:
						cols := make([]ColumnNode, len(msg.cols))
						for i, c := range msg.cols {
							cols[i] = ColumnNode{
								Name:     c.Name,
								DataType: c.Type,
								IsPK:     strings.HasPrefix(c.Extra, "PK"),
								IsFK:     strings.HasSuffix(c.Name, "_id"),
							}
						}
						node.Columns = cols
					}
				}
			}
			m.explorerGen++ // the tree's column children changed
			m.explorer.ensureVisible(m.sidebarTreeH())
		}
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.table = msg.table
		m.cols = msg.cols
		m.indexes = msg.indexes
		m.tableDDL = msg.ddl
		m.colsGen++
		m.sample = msg.sample
		m.count = msg.count
		m.page = 0
		m.err = ""
		m.buildTables()
		m.sizeTables()
		return m, nil

	case detailCountMsg:
		if msg.conn != m.connSeq {
			return m, nil // superseded by a connection switch
		}
		if msg.seq != m.detailSeq || msg.table != m.table {
			return m, nil // a different table (or selection) is open
		}
		if errors.Is(msg.err, context.Canceled) {
			return m, nil
		}
		if msg.err == nil {
			m.count = msg.count
		}
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
		m.explorerGen++
		m.status = fmt.Sprintf("%d tables • %s (%s)", total, m.db.Display, m.db.Driver)
		// One bounded-concurrency count batch per schema, issued in the
		// same operation as the first table's detail load so both share
		// one cancellable context.
		ctx := m.newOpContext(15 * time.Second)
		cmds := make([]tea.Cmd, 0, len(m.explorer.Schemas)+1)
		for si := range m.explorer.Schemas {
			s := m.explorer.Schemas[si]
			names := make([]string, len(s.Tables))
			for ti, tb := range s.Tables {
				names[ti] = tb.Name
			}
			cmds = append(cmds, m.loadCounts(ctx, s.Name, names))
		}
		// A restored session wins over "open the first table": the user
		// left peekerdb on that table, tab and filter last time.
		if m.restoreFilter != "" {
			// The search field is always on screen, so a restored filter
			// has to live in the input as well as in the tree.
			m.filterInput.SetValue(m.restoreFilter)
			m.explorer.SetFilter(m.restoreFilter)
		}
		restoreTable, restoreTab := m.restoreTable, m.restoreTab
		m.restoreTable, m.restoreFilter, m.restoreTab = "", "", 0
		if restoreTable != "" {
			for _, s := range m.explorer.Schemas {
				for _, tb := range s.Tables {
					if tb.Name != restoreTable {
						continue
					}
					m2, inspectCmd := m.inspectTableCtx(ctx, s.Name, tb.Name)
					m = m2
					if restoreTab >= 0 && restoreTab <= 4 {
						m.tab = restoreTab
					}
					return m, tea.Batch(append(cmds, inspectCmd)...)
				}
			}
		}
		for _, s := range m.explorer.Schemas {
			for _, tb := range s.Tables {
				m2, inspectCmd := m.inspectTableCtx(ctx, s.Name, tb.Name)
				m = m2
				return m, tea.Batch(append(cmds, inspectCmd)...)
			}
		}
		return m, tea.Batch(cmds...)

	case countsLoadedMsg:
		if msg.conn != m.connSeq {
			return m, nil // superseded by a connection switch
		}
		if m.counts == nil {
			m.counts = map[string]int64{}
		}
		for si := range m.explorer.Schemas {
			if m.explorer.Schemas[si].Name != msg.schema {
				continue
			}
			for ti := range m.explorer.Schemas[si].Tables {
				name := m.explorer.Schemas[si].Tables[ti].Name
				key := dbpkg.QualTable{Schema: msg.schema, Name: name}.String()
				if msg.errs[key] {
					m.explorer.Schemas[si].Tables[ti].CountErr = true
					continue
				}
				if n, ok := msg.counts[key]; ok {
					m.explorer.Schemas[si].Tables[ti].Count = n
					m.explorer.Schemas[si].Tables[ti].CountOK = true
					m.explorer.Schemas[si].Tables[ti].CountErr = false
					m.counts[key] = n
				}
			}
		}
		return m, nil

	case rowsPageMsg:
		if msg.conn != m.connSeq {
			return m, nil // superseded by a connection switch
		}
		if msg.seq != m.detailSeq {
			return m, nil // superseded by a newer selection
		}
		m.loading = false
		if errors.Is(msg.err, context.Canceled) {
			return m, nil // aborted on purpose (esc)
		}
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
		if errors.Is(msg.err, context.Canceled) {
			return m, nil // aborted on purpose (esc): not a query error
		}
		// Record the attempt either way: a query that failed is exactly
		// the one the user wants to recall and fix.
		saveCmd := m.recordHistory(msg.sql)
		if msg.err != nil {
			errStr := msg.err.Error() + dbpkg.HintForError(m.connStr, msg.err)
			m.qbufs[bufIdx].errStr = errStr
			if bufIdx == m.qcur {
				m.err = errStr
			}
			return m, saveCmd // keep editor text + old results
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
				return m, tea.Batch(m.loadSchemas(), saveCmd)
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
				// Best-effort count refresh for the previewed table: the
				// cached value is stale after a write, so re-fetch just it.
				for _, s := range m.explorer.Schemas {
					for _, tb := range s.Tables {
						if tb.Name == msg.previewTable {
							ctx := m.newOpContext(10 * time.Second)
							return m, tea.Batch(m.loadCounts(ctx, s.Name, []string{tb.Name}), saveCmd)
						}
					}
				}
				return m, saveCmd
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
			return m, saveCmd
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
		return m, saveCmd

	case erLoadedMsg:
		if msg.conn != m.connSeq {
			return m, nil // superseded by a connection switch
		}
		if msg.seq != m.erSeq {
			return m, nil // superseded by a newer tab enter
		}
		if errors.Is(msg.err, context.Canceled) {
			return m, nil // aborted on purpose (esc)
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
		if errors.Is(msg.err, context.Canceled) {
			return m, nil // aborted on purpose (esc)
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
			m.setERSchema(erSchemaState{tables: msg.tables, links: msg.links, loaded: true, err: msg.err.Error()})
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
		m.setERSchema(erSchemaState{tables: msg.tables, links: msg.links, loaded: true})
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
		// Keyboard and mouse drag are mutually exclusive: any key
		// cancels an armed drag (selection, if any, is handled by the
		// key itself via ClearSelection/ExtendSelectionTo).
		m.draggingEditor = false
		return m.handleKey(msg)

	case tea.PasteMsg:
		return m.handlePaste(msg)

	case tea.PasteStartMsg, tea.PasteEndMsg:
		return m, nil

	case tea.MouseClickMsg:
		return m.handleMouseClick(msg)
	case tea.MouseWheelMsg:
		// v2 reports wheel direction in Button; release events arrive
		// as MouseReleaseMsg (handled below).
		m.draggingEditor = false
		if msg.Button == tea.MouseWheelUp {
			return m.wheel(-3)
		}
		return m.wheel(3)
	case tea.MouseReleaseMsg:
		return m.handleMouseRelease(msg.X, msg.Y)
	case tea.MouseMotionMsg:
		if m.draggingEditor && !m.loading {
			if next, ok := m.dragQueryLine(msg.Y); ok {
				// x arrives absolute; clickQuery math is border-relative.
				return m.dragQueryTo(next, m.dragQueryCol(msg.X-m.paneX()-1, next))
			}
			return m, nil
		}
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

// handlePaste inserts bracketed-paste content into whichever input owns
// the focus. Terminal paste arrives as tea.PasteMsg (not KeyPressMsg),
// so without this the query editor silently drops every paste.
func (m Model) handlePaste(msg tea.PasteMsg) (tea.Model, tea.Cmd) {
	if msg.Content == "" {
		return m, nil
	}
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
	case screenBrowse:
		if m.filtering {
			var cmd tea.Cmd
			m.filterInput, cmd = m.filterInput.Update(msg)
			m.explorer.SetFilter(m.filterInput.Value())
			return m, cmd
		}
		if m.gridFiltering {
			var cmd tea.Cmd
			m.gridFilterInput, cmd = m.gridFilterInput.Update(msg)
			if g := m.filterGrid(); g != nil {
				g.SetFilter(m.gridFilterInput.Value())
			}
			return m, cmd
		}
		if m.sqlFiling {
			var cmd tea.Cmd
			m.sqlFileInput, cmd = m.sqlFileInput.Update(msg)
			return m, cmd
		}
		if m.cellEditing {
			var cmd tea.Cmd
			m.cellInput, cmd = m.cellInput.Update(msg)
			return m, cmd
		}
		if m.exporting {
			var cmd tea.Cmd
			m.exportInput, cmd = m.exportInput.Update(msg)
			return m, cmd
		}
		if m.focusDetail && m.tab == 3 && m.queryFocus == 0 {
			m.editor.InsertText(msg.Content)
			m.clampEditorScroll()
			m.refreshCompletionAfterEdit()
			return m, nil
		}
		return m, nil
	}
	return m, nil
}
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		m.cancelOp()
		if m.db != nil {
			_ = m.db.Close()
		}
		return m, tea.Quit
	}
	// esc aborts an in-flight command. While loading, the pane renders only
	// "loading...", so esc's other meanings (clear selection, step focus,
	// back to sidebar) are all no-ops here.
	if key == "esc" && m.loading {
		m.cancelOp()
		m.status = "cancelled"
		return m, nil
	}

	// Which-key overlay is modal: while open every key except ?/esc
	// (which close it) is swallowed before screen dispatch.
	if m.showHelp {
		if key == "?" || key == "esc" {
			m.showHelp = false
		}
		return m, nil
	}
	if m.themeModal {
		switch key {
		case "esc", "q":
			m.cancelThemeModal()
			return m, nil
		case "enter":
			m.confirmThemeModal()
			return m, nil
		case "up", "k":
			m.moveThemeCursor(-1)
			return m, nil
		case "down", "j":
			m.moveThemeCursor(1)
			return m, nil
		}
		return m, nil
	}
	if m.cellModal {
		switch key {
		case "esc", "q", "enter", "v", "V":
			m.cellModal = false
		case "up", "k":
			if m.cellModalOffset > 0 {
				m.cellModalOffset--
			}
		case "down", "j":
			m.cellModalOffset++
		case "g", "home":
			m.cellModalOffset = 0
		case "G", "end":
			m.cellModalOffset = 999999
		case "c", "y":
			m.status = m.copySelectionText(m.cellModalVal)
		}
		return m, nil
	}
	if m.historyModal {
		switch key {
		case "esc":
			m.historyModal = false
			return m, nil
		case "enter":
			m.selectHistoryEntry()
			return m, nil
		case "ctrl+y":
			m.status = m.copySelectedHistory()
			return m, nil
		case "up", "ctrl+p":
			if m.historyCursor > 0 {
				m.historyCursor--
				m.clampHistoryOffset()
			}
			return m, nil
		case "down", "ctrl+n":
			if m.historyCursor < len(m.historyFiltered)-1 {
				m.historyCursor++
				m.clampHistoryOffset()
			}
			return m, nil
		case "ctrl+c":
			m.historyModal = false
			return m, nil
		}
		var cmd tea.Cmd
		m.historyFilterInput, cmd = m.historyFilterInput.Update(msg)
		m.rebuildHistoryFiltered()
		return m, cmd
	}
	// ? opens the overlay, except where ? is text: conns filter input,
	// form inputs, or the query editor (tab==3, editor focused) where
	// Postgres JSON operators need the rune.
	if key == "?" && m.helpToggleAllowed() {
		m.showHelp = true
		return m, nil
	}
	if (key == "T" || key == "f3") && m.themeToggleAllowed(key) {
		m.openThemeModal()
		return m, nil
	}

	switch m.screen {
	case screenConns:
		return m.connsKey(msg, key)
	case screenForm:
		return m.formKey(msg, key)
	case screenBrowse:
		// q quits unless a filter/find prompt is being typed into or the
		// query editor has focus (there it inserts the rune).
		if key == "q" && !m.filtering && !m.gridFiltering && !m.sqlFiling && m.sqlConfirmPath == "" && m.explorer.Filter == "" && !(m.focusDetail && m.tab == 3 && m.queryFocus == 0) {
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
		return !(m.focusDetail && m.tab == 3 && m.queryFocus == 0) && !m.filtering && !m.gridFiltering && !m.sqlFiling
	}
	return true
}

// themeToggleAllowed reports whether T or F3 may open the theme picker on the
// current screen. F3 is global (except during modal prompts); T is allowed
// whenever the user is not actively typing in an input field.
func (m Model) themeToggleAllowed(key string) bool {
	if m.cellModal || m.historyModal || m.cellEditing || m.exporting {
		return false
	}
	if key == "f3" {
		return !m.gridFiltering && !m.sqlFiling && m.sqlConfirmPath == ""
	}
	switch m.screen {
	case screenConns:
		return !m.conns.SettingFilter()
	case screenForm:
		return !m.nameInput.Focused() && !m.connInput.Focused()
	case screenBrowse:
		return !(m.focusDetail && m.tab == 3 && m.queryFocus == 0) && !m.filtering && !m.gridFiltering && !m.sqlFiling && m.sqlConfirmPath == ""
	}
	return true
}
// clearConnState drops every piece of per-connection UI state so the
// next connection never shows the previous database's tables, counts,
// columns, query results or ER diagram. Seq counters bump alongside so
// in-flight replies carrying the old detail/er/query seq are dropped
// even before their conn generation is checked.
func (m *Model) clearConnState() {
	m.cancelOp()
	m.table = ""
	m.cols = nil
	m.tableDDL = ""
	m.sqlConfirmPath = ""
	m.colsGen++
	m.explorerGen++
	m.ccMemo = &completionMemo{}
	m.indexes = nil
	m.sample = nil
	m.count = -1
	m.counts = nil
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
	m.setERSchema(erSchemaState{})
	m.erMemo = &erMemo{} // positions belong to the old connection
	m.hlMemo = &hlMemo{}
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
			return m.inspectTable(row.Schema, row.Table)
		}
		if row.Kind != RowTable {
			return m, nil
		}
		return m.inspectTable(row.Schema, row.Table)
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
		m.clearFilter()
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

// clearFilter drops the applied sidebar filter and blurs the search
// field. esc and the field's × affordance both land here.
func (m *Model) clearFilter() {
	m.filtering = false
	m.filterInput.Blur()
	m.filterInput.SetValue("")
	m.explorer.SetFilter("")
}

// detailKey handles keys on the schema/indexes/rows/query/er tabs.
// Tabs 3 (query) and 4 (er) route to their own handlers first so editing
// runes never trigger the schema/indexes/rows bindings below.
func (m Model) detailKey(msg tea.KeyPressMsg, key string) (tea.Model, tea.Cmd) {
	// The inline cell editor is modal: every key belongs to it until
	// enter commits or esc abandons the edit.
	if m.cellEditing {
		switch key {
		case "esc":
			m.stopCellEdit()
			return m, nil
		case "enter":
			return m, m.submitCellEdit()
		}
		var cmd tea.Cmd
		m.cellInput, cmd = m.cellInput.Update(msg)
		return m, cmd
	}
	// The export prompt is modal inside the detail pane: every key
	// belongs to the path input until enter or esc.
	if m.exporting {
		switch key {
		case "esc":
			m.stopExport()
			return m, nil
		case "enter":
			m.finishExport()
			return m, nil
		}
		var cmd tea.Cmd
		m.exportInput, cmd = m.exportInput.Update(msg)
		return m, cmd
	}
	// Pending open-file confirm owns every key until y/n/esc.
	if m.sqlConfirmPath != "" {
		switch key {
		case "y", "Y":
			path := m.sqlConfirmPath
			m.sqlConfirmPath = ""
			m.finishLoadBufferFromPath(path)
			return m, nil
		case "n", "N", "esc":
			m.sqlConfirmPath = ""
			m.status = "kept buffer"
			return m, nil
		}
		return m, nil
	}
	// Grid find prompt is modal on the active grid: every key belongs to
	// it until enter keeps the filter or esc clears it.
	if m.gridFiltering {
		return m.gridFilterKeys(msg, key)
	}
	// SQL file prompt is modal on the query tab for the same reason.
	if m.sqlFiling {
		return m.sqlFileKeys(msg, key)
	}
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
		return m, m.loadDetail(m.explorer.schemaOf(m.table), m.table)
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
	case "E":
		m.startExport()
		return m, nil
	case ",":
		m.activeGrid().MoveColLeft()
		return m, nil
	case ".":
		m.activeGrid().MoveColRight()
		return m, nil
	case "o", "O":
		g := m.activeGrid()
		g.SortBy(g.Col())
		m.status = sortStatus(g)
		return m, nil
	case "/":
		if !m.openGridFilter() {
			return m, nil
		}
		return m, m.gridFilterInput.Focus()
	case "e":
		// Only the rows tab has editable cells; elsewhere `e` is free.
		if m.tab == 2 {
			if m.readOnly {
				m.status = "read-only: cell edits blocked"
				return m, nil
			}
			if m.startCellEdit() {
				return m, nil
			}
		}
		return m, nil
	case "v", "V":
		if m.openCellModal() {
			return m, nil
		}
	case "y":
		m.status = m.copyCellText()
		return m, nil
	case "Y":
		m.status = m.copyRowText()
		return m, nil
	case "F":
		m.status = m.quickFilterCell()
		return m, nil
	case "d", "D":
		// Copy the table DDL (schema tab echo); empty when unavailable.
		if strings.TrimSpace(m.tableDDL) == "" {
			m.status = "no DDL for this table"
			return m, nil
		}
		m.status = m.copySelectionText(m.tableDDL)
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
		// ctrl+p/ctrl+n walk the query history; ctrl+e/ctrl+y explain
		// the buffer on the next run without rewriting it.
		switch key {
		case "ctrl+p":
			m.historyPrev()
			return m, nil
		case "ctrl+n":
			m.historyNext()
			return m, nil
		case "ctrl+h":
			if m.openHistoryModal() {
				return m, nil
			}
			return m, nil
		case "ctrl+v", "shift+insert", "ctrl+shift+v":
			// Clipboard fallback for terminals without bracketed paste.
			// Bracketed paste arrives as tea.PasteMsg (handlePaste).
			txt, err := clipboardReadAll()
			if err != nil {
				m.status = "paste failed: " + err.Error()
				return m, nil
			}
			if txt == "" {
				return m, nil
			}
			m.editor.InsertText(txt)
			m.clampEditorScroll()
			m.refreshCompletionAfterEdit()
			return m, nil
		case "ctrl+o":
			m.openSQLFile()
			if m.sqlFiling {
				return m, m.sqlFileInput.Focus()
			}
			return m, nil
		case "f2":
			m.saveSQLFile()
			if m.sqlFiling {
				return m, m.sqlFileInput.Focus()
			}
			return m, nil
		case "ctrl+e":
			m.pendingExplain, m.pendingAnalyze = true, false
			if m.readOnlyBlocks() {
				return m, nil
			}
			m.startQueryRun()
			cmd := m.runQuery()
			return m, cmd
		case "ctrl+y":
			m.pendingExplain, m.pendingAnalyze = true, true
			if m.readOnlyBlocks() {
				return m, nil
			}
			m.startQueryRun()
			cmd := m.runQuery()
			return m, cmd
		}
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
			case "shift+left", "shift+right":
				m.showComplete = false
				m.completeItems = nil
				m.completeIdx = 0
				m.editor.ExtendSelectionChar(key == "shift+right")
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
			case "left", "right", "home", "end":
				// Cursor navigation while open dismisses instead of
				// re-opening: only edits should show suggestions.
				m.dismissCompletion()
				m.editor.ClearSelection()
				switch key {
				case "left":
					m.editor.MoveLeft()
				case "right":
					m.editor.MoveRight()
				case "home":
					m.editor.Home()
				case "end":
					m.editor.End()
				}
				m.clampEditorScroll()
				return m, nil
			case "ctrl+r", "f5":
				m.showComplete = false
				m.completeItems = nil
				m.completeIdx = 0
				if m.readOnlyBlocks() {
					return m, nil
				}
				m.startQueryRun()
				cmd := m.runQuery()
				return m, cmd
			case "ctrl+t":
				m.showComplete = false
				m.completeItems = nil
				m.completeIdx = 0
				m.saveActiveBuf()
				m.newQueryBuf()
				m.dismissCompletion()
				return m, nil
			case "ctrl+w":
				m.showComplete = false
				m.completeItems = nil
				m.completeIdx = 0
				m.closeQueryBuf()
				m.dismissCompletion()
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
			m.refreshCompletionAfterEdit()
			return m, nil
		case "shift+up":
			m.editor.ExtendSelectionTo(m.editor.CurLine - 1)
			m.clampEditorScroll()
			m.dismissCompletion()
			return m, nil
		case "shift+down":
			m.editor.ExtendSelectionTo(m.editor.CurLine + 1)
			m.clampEditorScroll()
			m.dismissCompletion()
			return m, nil
		case "shift+left":
			m.editor.ExtendSelectionChar(false)
			m.clampEditorScroll()
			m.dismissCompletion()
			return m, nil
		case "shift+right":
			m.editor.ExtendSelectionChar(true)
			m.clampEditorScroll()
			m.dismissCompletion()
			return m, nil
		case "ctrl+d":
			if _, _, active := m.editor.SelectedRange(); active {
				m.editor.DeleteRange()
			} else {
				// Delete current line without clipboard (spec: not a cut).
				m.editor.DeleteCurrentLine()
			}
			m.clampEditorScroll()
			m.refreshCompletionAfterEdit()
			return m, nil
		case "ctrl+/", "ctrl+_":
			m.editor.ToggleCommentRange()
			m.clampEditorScroll()
			m.refreshCompletionAfterEdit()
			return m, nil
		case "ctrl+r", "f5":
			if m.readOnlyBlocks() {
				return m, nil
			}
			m.startQueryRun()
			cmd := m.runQuery()
			return m, cmd
		case "ctrl+t":
			m.saveActiveBuf()
			m.newQueryBuf()
			m.dismissCompletion()
			return m, nil
		case "ctrl+w":
			m.closeQueryBuf()
			m.dismissCompletion()
			return m, nil
		case "enter":
			m.editor.ClearSelection()
			m.editor.Newline()
			m.clampEditorScroll()
			m.refreshCompletionAfterEdit()
			return m, nil
		case "backspace":
			m.editor.ClearSelection()
			m.editor.Backspace()
			m.clampEditorScroll()
			m.refreshCompletionAfterEdit()
			return m, nil
		case "delete":
			m.editor.ClearSelection()
			m.editor.Delete()
			m.refreshCompletionAfterEdit()
			return m, nil
		case "up":
			m.editor.ClearSelection()
			m.editor.MoveUp()
			m.clampEditorScroll()
			m.dismissCompletion()
			return m, nil
		case "down":
			m.editor.ClearSelection()
			m.editor.MoveDown()
			m.clampEditorScroll()
			m.dismissCompletion()
			return m, nil
		case "left":
			m.editor.ClearSelection()
			m.editor.MoveLeft()
			m.clampEditorScroll()
			m.dismissCompletion()
			return m, nil
		case "right":
			m.editor.ClearSelection()
			m.editor.MoveRight()
			m.clampEditorScroll()
			m.dismissCompletion()
			return m, nil
		case "home":
			m.editor.ClearSelection()
			m.editor.Home()
			m.dismissCompletion()
			return m, nil
		case "end":
			m.editor.ClearSelection()
			m.editor.End()
			m.dismissCompletion()
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
				switch r {
				case '\r':
					continue
				case '\n':
					m.editor.Newline()
				case '\x00':
					continue
				default:
					m.editor.Insert(r)
				}
			}
			m.clampEditorScroll()
			m.refreshCompletionAfterEdit()
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
		if m.readOnlyBlocks() {
			return m, nil
		}
		m.startQueryRun()
		cmd := m.runQuery()
		return m, cmd
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
	case "E":
		m.startExport()
		return m, nil
	case "ctrl+h":
		if m.openHistoryModal() {
			return m, nil
		}
		return m, nil
	case "v", "V":
		if m.openCellModal() {
			return m, nil
		}
	case "y":
		m.status = m.copyCellText()
		return m, nil
	case "Y":
		m.status = m.copyRowText()
		return m, nil
	case "F":
		m.status = m.quickFilterCell()
		return m, nil
	case "n":
		if m.queryPage < m.queryPageCount()-1 {
			m.queryPage++
			m.rebuildQueryPage()
			m.status = fmt.Sprintf("page %d/%d (%d rows/page)", m.queryPage+1, m.queryPageCount(), m.queryPageSize)
		}
		return m, nil
	case "p":
		if m.queryPage > 0 {
			m.queryPage--
			m.rebuildQueryPage()
			m.status = fmt.Sprintf("page %d/%d (%d rows/page)", m.queryPage+1, m.queryPageCount(), m.queryPageSize)
		}
		return m, nil
	case "s":
		switch m.queryPageSize {
		case 50:
			m.queryPageSize = 100
		case 100:
			m.queryPageSize = 200
		default:
			m.queryPageSize = 50
		}
		m.queryPage = 0
		m.rebuildQueryPage()
		m.status = fmt.Sprintf("query page size: %d", m.queryPageSize)
		return m, nil
	case ",":
		m.queryTable.MoveColLeft()
		return m, nil
	case ".":
		m.queryTable.MoveColRight()
		return m, nil
	case "o", "O":
		m.queryTable.SortBy(m.queryTable.Col())
		m.status = sortStatus(&m.queryTable)
		return m, nil
	case "ctrl+o":
		m.openSQLFile()
		if m.sqlFiling {
			return m, m.sqlFileInput.Focus()
		}
		return m, nil
	case "f2":
		m.saveSQLFile()
		if m.sqlFiling {
			return m, m.sqlFileInput.Focus()
		}
		return m, nil
	case "/":
		if !m.openGridFilter() {
			return m, nil
		}
		return m, m.gridFilterInput.Focus()
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
		m.setERSchema(erSchemaState{tables: m.erSchema.tables, links: m.erSchema.links})
		m.erSeq++
		var quals []dbpkg.QualTable
		for _, t := range m.erSchema.tables {
			quals = append(quals, dbpkg.QualTable{Schema: m.explorer.schemaOf(t.name), Name: t.name})
		}
		if len(quals) == 0 && m.table != "" {
			quals = []dbpkg.QualTable{{Schema: m.explorer.schemaOf(m.table), Name: m.table}}
		}
		// Never-silent rule: on pg an ambiguous center surfaces the
		// resolver-style error with zero DB calls instead of reloading a
		// possibly-wrong schema. Same candidates-gate as recenterER.
		if m.table != "" && m.db != nil && m.db.Driver == dbpkg.Postgres && m.explorer.schemaOf(m.table) == "" {
			if cands := m.explorer.candidates(m.table); len(cands) > 1 {
				amb := make([]dbpkg.QualTable, len(cands))
				for i, s := range cands {
					amb[i] = dbpkg.QualTable{Schema: s, Name: m.table}
				}
				m.loading = false
				m.err = dbpkg.AmbiguousErr(m.table, amb).Error()
				return m, nil
			}
		}
		// Same shared-context rule as setTab: one operation, one cancel.
		ctx := m.newOpContext(15 * time.Second)
		if m.table != "" {
			return m, tea.Batch(m.loadERCtx(ctx, m.table), m.loadERSchemaCtx(ctx, quals))
		}
		return m, m.loadERSchemaCtx(ctx, quals)
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

// filterGrid returns the grid a find prompt applies to, or nil when the
// current focus owns the keystroke (query editor, ER, modals).
func (m *Model) filterGrid() *dataTable {
	if m.tab < 3 {
		return m.activeGrid()
	}
	if m.tab == 3 && m.queryFocus == 1 {
		return &m.queryTable
	}
	return nil
}

// readOnlyBlocks reports whether the pending run is a write refused in
// read-only mode, clearing any armed explain flags. Call sites check this
// before startQueryRun so a blocked run never shows loading.
func (m *Model) readOnlyBlocks() bool {
	if !m.readOnly {
		return false
	}
	if dbpkg.IsWriteStatement(m.editor.RunSQL()) {
		m.pendingExplain, m.pendingAnalyze = false, false
		m.status = "read-only: writes blocked"
		return true
	}
	return false
}

// sortStatus labels a sort result as page-local so large paged tables
// never read as globally sorted.
func sortStatus(g *dataTable) string {
	if g == nil || len(g.cols) == 0 {
		return ""
	}
	st := g.SortState()
	if st == "" {
		return "sort cleared"
	}
	name := ""
	if c := g.SortedCol(); c >= 0 && c < len(g.cols) {
		name = g.cols[c]
	}
	dir := "asc"
	if len(st) >= 4 && st[len(st)-4:] == "desc" {
		dir = "desc"
	}
	if name != "" {
		return "sorted page by " + name + " " + dir
	}
	return "sorted page " + dir
}

// openGridFilter arms the find prompt on the active grid, seeding it with
// the grid's current filter. It reports whether a grid was available.
func (m *Model) openGridFilter() bool {
	g := m.filterGrid()
	if g == nil {
		return false
	}
	m.gridFiltering = true
	m.gridFilterInput.SetValue(g.FilterText())
	return true
}

// gridFilterKeys routes keystrokes while the find prompt is open: esc
// clears and closes, enter keeps and closes, everything else filters live.
func (m Model) gridFilterKeys(msg tea.KeyPressMsg, key string) (tea.Model, tea.Cmd) {
	g := m.filterGrid()
	if g == nil {
		m.gridFiltering = false
		m.gridFilterInput.Blur()
		return m, nil
	}
	switch key {
	case "esc":
		g.ClearFilter()
		m.gridFiltering = false
		m.gridFilterInput.Blur()
		m.gridFilterInput.SetValue("")
		return m, nil
	case "enter":
		m.gridFiltering = false
		m.gridFilterInput.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.gridFilterInput, cmd = m.gridFilterInput.Update(msg)
	g.SetFilter(m.gridFilterInput.Value())
	return m, cmd
}

// sqlFileKeys routes keystrokes while the SQL file prompt is open: esc
// abandons, enter confirms the armed open or save.
func (m Model) sqlFileKeys(msg tea.KeyPressMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.stopSQLFile()
		return m, nil
	case "enter":
		m.confirmSQLFile()
		return m, nil
	}
	var cmd tea.Cmd
	m.sqlFileInput, cmd = m.sqlFileInput.Update(msg)
	return m, cmd
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
	// No explorer schema: the ER diagram owns the selection here, so the
	// tree node's columns are left alone. On pg an ambiguous bare name
	// surfaces the resolver-style error with zero DB calls instead of
	// guessing a schema.
	schema := m.explorer.schemaOf(name)
	if schema == "" && m.db != nil && m.db.Driver == dbpkg.Postgres {
		if cands := m.explorer.candidates(name); len(cands) > 1 {
			quals := make([]dbpkg.QualTable, len(cands))
			for i, s := range cands {
				quals[i] = dbpkg.QualTable{Schema: s, Name: name}
			}
			// surface without a DB roundtrip: reuse the seq-guarded error path
			m.loading = false
			m.err = dbpkg.AmbiguousErr(name, quals).Error()
			return m, nil
		}
	}
	return m, m.loadDetail(schema, name)
}

// inspectTable previews one table in the detail pane; shared by sidebar
// enter/click. The seq guard drops replies from superseded selections.
// Previewing blurs the filter input (the text stays applied) but never
// steals pane focus: sidebar previews stay in the sidebar (Tab jumps to
// detail), detail-initiated previews keep detail focus.
func (m Model) inspectTable(schema, name string) (Model, tea.Cmd) {
	m.filtering = false
	m.filterInput.Blur()
	m.gridFiltering = false
	m.gridFilterInput.Blur()
	m.table = name
	m.tab = 0
	m.loading = true
	m.err = ""
	m.page = 0 // pageSize persists across tables
	m.hoverTab = -1
	m.detailSeq++
	return m, m.loadDetail(schema, name)
}

// inspectTableCtx is inspectTable under a caller-owned context, so a
// batched operation can issue it alongside sibling commands.
func (m Model) inspectTableCtx(ctx context.Context, schema, name string) (Model, tea.Cmd) {
	m.filtering = false
	m.filterInput.Blur()
	m.gridFiltering = false
	m.gridFilterInput.Blur()
	m.table = name
	m.tab = 0
	m.loading = true
	m.err = ""
	m.page = 0
	m.hoverTab = -1
	m.detailSeq++
	return m, m.loadDetailCtx(ctx, schema, name)
}
