package tui

// Update is the Elm update function: (Model, Msg) -> (Model, Cmd).
// Message arrivals mutate state and kick off commands; key and mouse
// input delegate to per-screen handlers.

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
		m.focusDetail = false
		m.err = ""
		m.explorer = NewExplorer(m.db.Display, nil)
		return m, m.loadSchemas()

	case detailLoadedMsg:
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
		return m, nil

	case rowsPageMsg:
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
		if msg.seq != m.querySeq {
			return m, nil // superseded by a newer run
		}
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil // keep editor text + old results
		}
		m.querySample = msg.sample
		m.queryMs = msg.ms
		m.err = ""
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
		m.queryTable.setData(qcols, qrows)
		m.sizeTables()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)
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
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		if m.db != nil {
			_ = m.db.Close()
		}
		return m, tea.Quit
	}

	switch m.screen {
	case screenConns:
		return m.connsKey(msg, key)
	case screenForm:
		return m.formKey(msg, key)
	case screenBrowse:
		// q quits unless the sidebar filter is being typed into or the
		// query editor has focus (there it inserts the rune).
		if key == "q" && m.explorer.Filter == "" && !(m.focusDetail && m.tab == 3 && m.queryFocus == 0) {
			if m.db != nil {
				_ = m.db.Close()
			}
			return m, tea.Quit
		}
		// tab jumps between sidebar and detail.
		if key == "tab" || key == "shift+tab" {
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

// disconnect closes the database and returns to the picker.
func (m *Model) disconnect() {
	if m.db != nil {
		_ = m.db.Close()
		m.db = nil
	}
	m.screen = screenConns
	m.focusDetail = false
	m.err = ""
	m.refreshConns()
}

// sidebarKeys handles keys on the browse sidebar (explorer is the source
// of truth; no legacy list filter exists — "/" clears the filter).
func (m Model) sidebarKeys(msg tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		m.explorer.MoveUp()
		return m, nil
	case "down", "j":
		m.explorer.MoveDown()
		return m, nil
	case "left", "right":
		m.explorer.Toggle()
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
		// Filter input UI deferred to Task 5; clear filter for now.
		m.explorer.SetFilter("")
		return m, nil
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

// detailKey handles keys on the schema/indexes/rows/query/er tabs.
// Tabs 3 (query) and 4 (er) route to their own handlers first so editing
// runes never trigger the schema/indexes/rows bindings below.
func (m Model) detailKey(msg tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
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
		m.setTab((m.tab + 1) % 5)
		return m, nil
	case "shift+tab", "left", "h":
		m.setTab((m.tab + 4) % 5)
		return m, nil
	case "1":
		m.setTab(0)
		return m, nil
	case "2":
		m.setTab(1)
		return m, nil
	case "3":
		m.setTab(2)
		return m, nil
	case "4":
		m.setTab(3)
		return m, nil
	case "5":
		m.setTab(4)
		return m, nil
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
func (m Model) queryKeys(msg tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
	if m.queryFocus == 0 {
		switch key {
		case "esc":
			m.queryFocus = 1
			return m, nil
		case "ctrl+r", "f5":
			m.querySeq++
			m.loading = true
			return m, m.runQuery()
		case "enter":
			m.editor.Newline()
			m.clampEditorScroll()
			return m, nil
		case "backspace":
			m.editor.Backspace()
			m.clampEditorScroll()
			return m, nil
		case "delete":
			m.editor.Delete()
			return m, nil
		case "up":
			m.editor.MoveUp()
			m.clampEditorScroll()
			return m, nil
		case "down":
			m.editor.MoveDown()
			m.clampEditorScroll()
			return m, nil
		case "left":
			m.editor.MoveLeft()
			m.clampEditorScroll()
			return m, nil
		case "right":
			m.editor.MoveRight()
			m.clampEditorScroll()
			return m, nil
		case "home":
			m.editor.Home()
			return m, nil
		case "end":
			m.editor.End()
			return m, nil
		}
		// Every rune inserts while editing, including "?" (Postgres JSON
		// operators need it). Task 7's "?"-overlay must intercept "?"
		// before detailKey routing except when the editor is focused.
		if msg.Type == tea.KeyRunes {
			for _, r := range msg.Runes {
				m.editor.Insert(r)
			}
			m.clampEditorScroll()
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
		m.setTab((m.tab + 1) % 5)
		return m, nil
	case "shift+tab", "left", "h":
		m.setTab((m.tab + 4) % 5)
		return m, nil
	case "1":
		m.setTab(0)
		return m, nil
	case "2":
		m.setTab(1)
		return m, nil
	case "3":
		m.setTab(2)
		return m, nil
	case "4":
		m.setTab(3)
		return m, nil
	case "5":
		m.setTab(4)
		return m, nil
	case "ctrl+r", "f5", "r":
		m.querySeq++
		m.loading = true
		return m, m.runQuery()
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

// erKeys is the Task 6 placeholder for the er tab (tab 4): tab switching
// works, cursor motion is a noop until the diagram lands.
func (m Model) erKeys(_ tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "backspace":
		m.focusDetail = false
		m.err = ""
		m.hoverTab = -1
		return m, nil
	case "tab", "right", "l":
		m.setTab((m.tab + 1) % 5)
		return m, nil
	case "shift+tab", "left", "h":
		m.setTab((m.tab + 4) % 5)
		return m, nil
	case "1":
		m.setTab(0)
		return m, nil
	case "2":
		m.setTab(1)
		return m, nil
	case "3":
		m.setTab(2)
		return m, nil
	case "4":
		m.setTab(3)
		return m, nil
	case "5":
		m.setTab(4)
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
// activateConn connects to a saved profile; shared by enter-key and double-click.
func (m Model) activateConn(name string) (Model, tea.Cmd) {
	m.delArm = ""
	m.screen = screenBrowse
	m.focusDetail = false
	m.loading = true
	m.err = ""
	return m, m.openSaved(name)
}

// inspectTable previews one table in the detail pane; shared by sidebar
// enter/click. The seq guard drops replies from superseded selections.
func (m Model) inspectTable(name string) (Model, tea.Cmd) {
	m.focusDetail = true
	m.table = name
	m.tab = 0
	m.loading = true
	m.err = ""
	m.page = 0 // pageSize persists across tables
	m.hoverTab = -1
	m.detailSeq++
	return m, m.loadDetail(name)
}
