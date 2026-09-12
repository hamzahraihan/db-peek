package tui

// Update is the Elm update function: (Model, Msg) -> (Model, Cmd).
// Message arrivals mutate state and kick off commands; key and mouse
// input delegate to per-screen handlers.

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Header line + footer block (foot + optional err/status lines).
		top := m.height - 4
		if top < 5 {
			top = 5
		}
		m.list.SetSize(msg.Width-4, top)
		m.conns.SetSize(msg.Width-4, top)
		m.sizeTables()
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
		m.screen = screenTables
		m.err = ""
		items := make([]list.Item, len(msg.names))
		for i, n := range msg.names {
			items[i] = tableItem{name: n}
		}
		m.list.SetItems(items)
		m.status = fmt.Sprintf("%d tables • %s (%s)", len(items), m.db.Display, m.db.Driver)
		return m, nil

	case tablesLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		items := make([]list.Item, len(msg.names))
		for i, n := range msg.names {
			items[i] = tableItem{name: n}
		}
		m.list.SetItems(items)
		return m, nil

	case detailLoadedMsg:
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

	case rowsPageMsg:
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
	case screenTables:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	switch m.tab {
	case 0:
		m.colTable, cmd = m.colTable.Update(msg)
	case 1:
		m.idxTable, cmd = m.idxTable.Update(msg)
	default:
		m.rowTable, cmd = m.rowTable.Update(msg)
	}
	return m, cmd
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
	case screenTables:
		switch key {
		case "q":
			if m.list.IsFiltered() || m.list.FilterInput.Focused() {
				break // list consumes it
			}
			if m.db != nil {
				_ = m.db.Close()
			}
			return m, tea.Quit
		}
	}

	switch m.screen {
	case screenTables:
		return m.tablesKey(msg, key)
	default: // detail
		return m.detailKey(msg, key)
	}
}

// tablesKey handles keys on the table picker.
func (m Model) tablesKey(msg tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "enter":
		sel, ok := m.list.SelectedItem().(tableItem)
		if !ok {
			return m, nil
		}
		return m.inspectTable(sel.name)
	case "r":
		m.loading = true
		return m, m.reloadTables()
	case "c":
		// Disconnect back to the connection picker.
		if m.db != nil {
			_ = m.db.Close()
			m.db = nil
		}
		m.screen = screenConns
		m.err = ""
		m.refreshConns()
		return m, nil
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// detailKey handles keys on the schema/indexes/rows tabs.
func (m Model) detailKey(msg tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "backspace":
		m.screen = screenTables
		m.err = ""
		return m, nil
	case "tab", "right", "l":
		m.setTab((m.tab + 1) % 3)
		return m, nil
	case "shift+tab", "left", "h":
		m.setTab((m.tab + 2) % 3)
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
	var cmd tea.Cmd
	switch m.tab {
	case 0:
		m.colTable, cmd = m.colTable.Update(msg)
	case 1:
		m.idxTable, cmd = m.idxTable.Update(msg)
	default:
		m.rowTable, cmd = m.rowTable.Update(msg)
	}
	return m, cmd
}

// activateConn connects to a saved profile; shared by enter-key and double-click.
func (m Model) activateConn(name string) (Model, tea.Cmd) {
	m.delArm = ""
	m.screen = screenTables
	m.loading = true
	m.err = ""
	return m, m.openSaved(name)
}

// inspectTable opens the detail view for one table; shared by enter-key and double-click.
func (m Model) inspectTable(name string) (Model, tea.Cmd) {
	m.screen = screenDetail
	m.table = name
	m.tab = 0
	m.loading = true
	m.err = ""
	m.cols, m.indexes, m.sample, m.count = nil, nil, nil, -1
	m.page = 0 // pageSize persists across tables
	return m, m.loadDetail(name)
}
