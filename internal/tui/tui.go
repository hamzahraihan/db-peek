package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	dbpkg "db-peek/internal/db"
	"db-peek/internal/saved"
)

type screen int

const (
	screenConns screen = iota
	screenForm
	screenTables
	screenDetail
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	activeTab   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("4")).Padding(0, 2)
	inactiveTab = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Padding(0, 2)
	// Line highlighter: the selected row anywhere (pickers, data tables).
	selTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Padding(0, 1)
	selDesc  = lipgloss.NewStyle().Foreground(lipgloss.Color("254")).Background(lipgloss.Color("62")).Padding(0, 1)
)

// tableItem is one row in the fuzzy-filterable table list.
type tableItem struct{ name string }

func (t tableItem) FilterValue() string { return t.name }
func (t tableItem) Title() string       { return t.name }
func (t tableItem) Description() string { return "table" }

// connItem is one saved connection in the picker.
type connItem struct {
	name   string
	masked string
}

func (c connItem) FilterValue() string { return c.name + " " + c.masked }
func (c connItem) Title() string       { return c.name }
func (c connItem) Description() string { return c.masked }

type (
	tablesLoadedMsg struct {
		names []string
		err   error
	}
	detailLoadedMsg struct {
		table   string
		cols    []dbpkg.Column
		indexes []dbpkg.Index
		sample  *dbpkg.Sample
		count   int64
		err     error
	}
	rowsPageMsg struct {
		sample *dbpkg.Sample
		page   int
		err    error
	}
	connectMsg struct {
		db    *dbpkg.DB
		names []string
		err   error
	}
)

type Model struct {
	db      *dbpkg.DB
	store   *saved.Store
	connStr string
	screen  screen

	conns  list.Model
	delArm string // profile name armed for delete confirmation

	nameInput textinput.Model
	connInput textinput.Model
	formFocus int    // 0 name, 1 conn
	editing   string // profile being edited, "" when adding

	list     list.Model
	tab      int // 0 schema, 1 indexes, 2 rows
	table    string
	cols     []dbpkg.Column
	indexes  []dbpkg.Index
	sample   *dbpkg.Sample
	count    int64
	page     int // 0-based page in the rows tab
	pageSize int // rows per page: one of pageSizes
	loading  bool
	err      string
	status   string
	width    int
	height   int
	colTable table.Model
	idxTable table.Model
	rowTable table.Model

	// Mouse: rows per item in each picker (from the item delegates), plus
	// last click for double-click detection and hover deduplication.
	connsItemH     int
	tablesItemH    int
	lastClickAt    time.Time
	lastClickIdx   int
	lastClickWhere screen
	lastHoverIdx   int
	lastHoverWhere screen
}

func Run(connStr string, store *saved.Store) error {
	m := New(connStr, store)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}

func New(connStr string, store *saved.Store) Model {
	if store == nil {
		store = &saved.Store{}
	}
	nameInput := textinput.New()
	nameInput.Placeholder = "prod-pg"
	nameInput.CharLimit = 64
	nameInput.Width = 60

	connInput := textinput.New()
	connInput.Placeholder = "postgres://user:pass@localhost:5432/db  |  mysql://...  |  ./app.db"
	connInput.CharLimit = 512
	connInput.Width = 80

	cdelegate := list.NewDefaultDelegate()
	cdelegate.SetSpacing(0)
	cstyles := list.NewDefaultItemStyles()
	cstyles.SelectedTitle = selTitle
	cstyles.SelectedDesc = selDesc
	cdelegate.Styles = cstyles
	cl := list.New(nil, cdelegate, 0, 0)
	cl.Title = "Connections  (enter to connect • a to add)"
	cl.SetShowStatusBar(true)
	cl.SetFilteringEnabled(true)
	cl.SetShowHelp(true)

	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetSpacing(0)
	tstyles := list.NewDefaultItemStyles()
	tstyles.SelectedTitle = selTitle
	tstyles.SelectedDesc = selDesc
	delegate.Styles = tstyles
	l := list.New(nil, delegate, 0, 0)
	l.Title = "Tables  (type to filter • enter to inspect)"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)

	m := Model{
		connStr: strings.TrimSpace(connStr), store: store,
		conns: cl, list: l, count: -1, pageSize: 10,
		nameInput: nameInput, connInput: connInput,
		connsItemH:  cdelegate.Height() + cdelegate.Spacing(),
		tablesItemH: delegate.Height() + delegate.Spacing(),
	}
	m.refreshConns()
	if m.connStr == "" {
		m.screen = screenConns
	} else {
		m.screen = screenTables
		m.loading = true
	}
	return m
}

func (m *Model) refreshConns() {
	profiles := m.store.List()
	items := make([]list.Item, len(profiles))
	for i, p := range profiles {
		items[i] = connItem{name: p.Name, masked: saved.Mask(p.Conn)}
	}
	m.conns.SetItems(items)
}

func (m Model) Init() tea.Cmd {
	if m.connStr != "" {
		return m.openAndLoad(m.connStr)
	}
	return nil
}

func (m Model) openAndLoad(connStr string) tea.Cmd {
	return func() tea.Msg {
		db, err := dbpkg.Open(connStr)
		if err != nil {
			return connectMsg{err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			_ = db.Close()
			return connectMsg{err: fmt.Errorf("connect failed: %w", err)}
		}
		names, err := db.ListTables(ctx)
		if err != nil {
			_ = db.Close()
			return connectMsg{err: err}
		}
		return connectMsg{db: db, names: names}
	}
}

func (m Model) openSaved(name string) tea.Cmd {
	p, ok := m.store.Get(name)
	if !ok {
		return func() tea.Msg { return connectMsg{err: fmt.Errorf("no saved connection %q", name)} }
	}
	m.connStr = p.Conn
	return m.openAndLoad(p.Conn)
}

// pageSizes are the rows-tab page options, cycled with s.
var pageSizes = []int{10, 20, 50, 100}

func nextPageSize(cur int) int {
	for i, s := range pageSizes {
		if s == cur {
			return pageSizes[(i+1)%len(pageSizes)]
		}
	}
	return pageSizes[0]
}

// hasNextPage reports whether another rows page exists. Without an exact
// count a full page implies more rows may follow.
func (m Model) hasNextPage() bool {
	if m.count >= 0 {
		return int64(m.page+1)*int64(m.pageSize) < m.count
	}
	return m.sample != nil && len(m.sample.Rows) == m.pageSize
}

// pageCount is the 1-based page total, or -1 when the count is unknown.
func (m Model) pageCount() int {
	if m.count < 0 {
		return -1
	}
	n := int((m.count + int64(m.pageSize) - 1) / int64(m.pageSize))
	if n < 1 {
		n = 1
	}
	return n
}

// pagerLine describes rows-tab position, e.g. "page 2/5 • 20 rows/page • n next • p prev • s size".
func (m Model) pagerLine() string {
	if n := m.pageCount(); n > 0 {
		return fmt.Sprintf("page %d/%d • %d rows/page • n next • p prev • s size", m.page+1, n, m.pageSize)
	}
	return fmt.Sprintf("page %d • %d rows/page • n next • p prev • s size", m.page+1, m.pageSize)
}

func (m Model) loadDetail(table string) tea.Cmd {
	db := m.db
	size := m.pageSize
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cols, err := db.Columns(ctx, table)
		if err != nil {
			return detailLoadedMsg{table: table, err: err}
		}
		idx, err := db.Indexes(ctx, table)
		if err != nil {
			return detailLoadedMsg{table: table, err: err}
		}
		sample, err := db.PageRows(ctx, table, size, 0)
		if err != nil {
			return detailLoadedMsg{table: table, err: err}
		}
		count, err := db.Count(ctx, table)
		if err != nil {
			count = -1 // sample still useful; count failure is non-fatal
		}
		return detailLoadedMsg{table: table, cols: cols, indexes: idx, sample: sample, count: count}
	}
}

// loadRowsPage fetches one page of the current table for the rows tab.
func (m Model) loadRowsPage() tea.Cmd {
	db, table, size, page := m.db, m.table, m.pageSize, m.page
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		sample, err := db.PageRows(ctx, table, size, page*size)
		if err != nil {
			return rowsPageMsg{err: err}
		}
		return rowsPageMsg{sample: sample, page: page}
	}
}

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
		switch key {
		case "enter":
			sel, ok := m.list.SelectedItem().(tableItem)
			if !ok {
				return m, nil
			}
			return m.inspectTable(sel.name)
		case "r":
			m.loading = true
			db := m.db
			return m, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				names, err := db.ListTables(ctx)
				return tablesLoadedMsg{names: names, err: err}
			}
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

	default: // detail
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
}

// connsKey drives the saved-connection picker.
func (m Model) connsKey(msg tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q":
		if m.conns.IsFiltered() || m.conns.FilterInput.Focused() {
			break // list consumes it
		}
		return m, tea.Quit
	case "enter":
		sel, ok := m.conns.SelectedItem().(connItem)
		if !ok {
			m.err = "no saved connections yet — press a to add one"
			return m, nil
		}
		return m.activateConn(sel.name)
	case "a":
		m.delArm = ""
		m.editing = ""
		m.nameInput.SetValue("")
		m.connInput.SetValue("")
		m.formFocus = 0
		m.nameInput.Focus()
		m.connInput.Blur()
		m.screen = screenForm
		m.err = ""
		return m, textinput.Blink
	case "e":
		sel, ok := m.conns.SelectedItem().(connItem)
		if !ok {
			return m, nil
		}
		m.delArm = ""
		p, _ := m.store.Get(sel.name)
		m.editing = p.Name
		m.nameInput.SetValue(p.Name)
		m.connInput.SetValue(p.Conn)
		m.formFocus = 1
		m.nameInput.Blur()
		m.connInput.Focus()
		m.screen = screenForm
		m.err = ""
		return m, textinput.Blink
	case "d":
		sel, ok := m.conns.SelectedItem().(connItem)
		if !ok {
			return m, nil
		}
		if m.delArm != sel.name {
			m.delArm = sel.name
			m.status = fmt.Sprintf("press d again to forget %q", sel.name)
			return m, nil
		}
		m.delArm = ""
		if err := m.store.Delete(sel.name); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.refreshConns()
		m.status = fmt.Sprintf("forgot %q", sel.name)
		return m, nil
	}
	m.delArm = ""
	var cmd tea.Cmd
	m.conns, cmd = m.conns.Update(msg)
	return m, cmd
}

// formKey drives the add/edit connection form.
func (m Model) formKey(msg tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.screen = screenConns
		m.err = ""
		m.nameInput.Blur()
		m.connInput.Blur()
		return m, nil
	case "tab", "shift+tab", "up", "down":
		if m.formFocus == 0 {
			m.formFocus = 1
			m.nameInput.Blur()
			m.connInput.Focus()
		} else {
			m.formFocus = 0
			m.connInput.Blur()
			m.nameInput.Focus()
		}
		return m, textinput.Blink
	case "enter":
		name := strings.TrimSpace(m.nameInput.Value())
		conn := strings.TrimSpace(m.connInput.Value())
		if name == "" || conn == "" {
			m.err = "name and connection string are both required"
			return m, nil
		}
		if err := m.store.Upsert(name, conn); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.refreshConns()
		m.nameInput.Blur()
		m.connInput.Blur()
		m.connStr = conn
		m.screen = screenTables
		m.loading = true
		m.err = ""
		return m, m.openAndLoad(conn)
	}
	var cmd tea.Cmd
	if m.formFocus == 0 {
		m.nameInput, cmd = m.nameInput.Update(msg)
	} else {
		m.connInput, cmd = m.connInput.Update(msg)
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

// listChromeH is the fixed header above list items: 2 title rows + 2 status rows
// (bubbles list default styles: TitleBar and StatusBar each pad one blank line).
const listChromeH = 4

// handleMouse implements click-to-select, double-click-to-open, wheel scroll,
// hover highlight, tab clicks, and form field focus. Coordinates are
// 0-indexed terminal cells.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.MouseWheelUp:
		return m.wheel(-3)
	case tea.MouseWheelDown:
		return m.wheel(3)
	case tea.MouseMotion:
		return m.hover(msg)
	}
	if msg.Type != tea.MouseLeft || msg.Action != tea.MouseActionPress {
		return m, nil // ignore release
	}
	if m.loading {
		return m, nil
	}
	switch m.screen {
	case screenConns:
		return m.clickList(msg.Y, screenConns)
	case screenTables:
		return m.clickList(msg.Y, screenTables)
	case screenForm:
		return m.clickForm(msg.Y)
	default:
		return m.clickTabs(msg.X, msg.Y)
	}
}

// wheel moves the focused list or detail table by n rows (negative = up).
func (m Model) wheel(n int) (tea.Model, tea.Cmd) {
	if m.loading {
		return m, nil
	}
	up := n < 0
	steps := n
	if steps < 0 {
		steps = -steps
	}
	switch m.screen {
	case screenConns:
		for range steps {
			if up {
				m.conns.CursorUp()
			} else {
				m.conns.CursorDown()
			}
		}
		return m, nil
	case screenTables:
		for range steps {
			if up {
				m.list.CursorUp()
			} else {
				m.list.CursorDown()
			}
		}
		return m, nil
	default: // detail: scroll the active tab's table
		switch m.tab {
		case 0:
			if up {
				m.colTable.MoveUp(steps)
			} else {
				m.colTable.MoveDown(steps)
			}
		case 1:
			if up {
				m.idxTable.MoveUp(steps)
			} else {
				m.idxTable.MoveDown(steps)
			}
		default:
			if up {
				m.rowTable.MoveUp(steps)
			} else {
				m.rowTable.MoveDown(steps)
			}
		}
		return m, nil
	}
}

// clickList maps a click row to a picker item: single click selects,
// double-click opens. Items are top-anchored right below the 4 chrome rows.
func (m Model) clickList(y int, which screen) (tea.Model, tea.Cmd) {
	idx, ok := m.listIndexAt(y, which)
	if !ok {
		return m, nil
	}
	var l *list.Model
	if which == screenConns {
		l = &m.conns
	} else {
		l = &m.list
	}
	l.Select(idx)
	if which == m.lastClickWhere && idx == m.lastClickIdx && time.Since(m.lastClickAt) < 500*time.Millisecond {
		m.lastClickAt = time.Time{}
		if which == screenConns {
			if sel, ok := l.SelectedItem().(connItem); ok {
				return m.activateConn(sel.name)
			}
			return m, nil
		}
		if sel, ok := l.SelectedItem().(tableItem); ok {
			return m.inspectTable(sel.name)
		}
		return m, nil
	}
	m.lastClickWhere, m.lastClickIdx, m.lastClickAt = which, idx, time.Now()
	return m, nil
}

// listIndexAt resolves a terminal row to a global item index in a picker.
func (m Model) listIndexAt(y int, which screen) (int, bool) {
	var l *list.Model
	var itemH int
	if which == screenConns {
		l, itemH = &m.conns, m.connsItemH
	} else {
		l, itemH = &m.list, m.tablesItemH
	}
	if itemH <= 0 {
		return 0, false
	}
	row := y - 1 - listChromeH // -1 for our header line
	if row < 0 {
		return 0, false
	}
	perPage := l.Paginator.PerPage
	if perPage <= 0 {
		return 0, false
	}
	vis := l.VisibleItems()
	start := l.Paginator.Page * perPage
	onPage := len(vis) - start
	if onPage > perPage {
		onPage = perPage
	}
	if onPage <= 0 || row/itemH >= onPage {
		return 0, false // empty padding
	}
	return start + row/itemH, true
}

// hover moves the highlight to follow the mouse without activating anything.
// Motion events are deduplicated so holding the cursor still is a no-op.
func (m Model) hover(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.loading {
		return m, nil
	}
	switch m.screen {
	case screenConns:
		return m.hoverList(msg.Y, screenConns)
	case screenTables:
		return m.hoverList(msg.Y, screenTables)
	default:
		return m.hoverTable(msg.Y)
	}
}

func (m Model) hoverList(y int, which screen) (tea.Model, tea.Cmd) {
	idx, ok := m.listIndexAt(y, which)
	if !ok {
		return m, nil
	}
	if which == m.lastHoverWhere && idx == m.lastHoverIdx {
		return m, nil
	}
	m.lastHoverWhere, m.lastHoverIdx = which, idx
	if which == screenConns {
		m.conns.Select(idx)
	} else {
		m.list.Select(idx)
	}
	return m, nil
}

// detailTableTop is the first terminal row of a detail table: header, table
// title, blank, tabs, blank, then the table itself.
const detailTableTop = 5

// detailHeaderH is the table's own header height: 1 title row plus the
// bottom border drawn by dataTableStyles.
const detailHeaderH = 2

// hoverTable highlights the data row under the cursor. The table viewport
// only ever scrolls via MoveUp/MoveDown, so the first visible row is
// derivable as clamp(cursor-height, 0, cursor).
func (m Model) hoverTable(y int) (tea.Model, tea.Cmd) {
	var t *table.Model
	switch m.tab {
	case 0:
		t = &m.colTable
	case 1:
		t = &m.idxTable
	default:
		t = &m.rowTable
	}
	rel := y - detailTableTop - detailHeaderH
	if rel < 0 {
		return m, nil
	}
	h := t.Height()
	cursor := t.Cursor()
	start := cursor - h
	if start < 0 {
		start = 0
	}
	if cursor < start {
		start = cursor
	}
	rows := len(t.Rows())
	end := cursor + h
	if end < cursor {
		end = cursor
	}
	if end > rows {
		end = rows
	}
	abs := start + rel
	if abs < start || abs >= end || abs == cursor {
		return m, nil
	}
	t.SetCursor(abs)
	return m, nil
}

// clickForm focuses the clicked field. Layout: title, blank, name label,
// name input, blank, conn label, conn input — so inputs sit on rows 3 and 6.
func (m Model) clickForm(y int) (tea.Model, tea.Cmd) {
	switch y {
	case 3:
		m.formFocus = 0
		m.nameInput.Focus()
		m.connInput.Blur()
		return m, textinput.Blink
	case 6:
		m.formFocus = 1
		m.nameInput.Blur()
		m.connInput.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

// detailTabLabels renders the tab strip; the rows tab shows the page size.
func (m Model) detailTabLabels() []string {
	return []string{"1 schema", "2 indexes", fmt.Sprintf("3 rows ×%d", m.pageSize)}
}

// clickTabs switches tabs when a tab label is clicked. Tab labels sit on row 3
// (header, table title, blank, tabs) with one space between labels.
func (m Model) clickTabs(x, y int) (tea.Model, tea.Cmd) {
	if y != 3 {
		return m, nil
	}
	xpos := 0
	for i, t := range m.detailTabLabels() {
		w := lipgloss.Width(inactiveTab.Render(t))
		if x >= xpos && x < xpos+w {
			m.setTab(i)
			return m, nil
		}
		xpos += w + 1
	}
	return m, nil
}

func (m *Model) buildTables() {
	colCols := []table.Column{
		{Title: "column", Width: 24}, {Title: "type", Width: 22},
		{Title: "null", Width: 6}, {Title: "default", Width: 22}, {Title: "extra", Width: 18},
	}
	var colRows []table.Row
	for _, c := range m.cols {
		colRows = append(colRows, table.Row{c.Name, c.Type, c.Nullable, c.Default, c.Extra})
	}
	m.colTable = table.New(
		table.WithColumns(colCols), table.WithRows(colRows),
		table.WithFocused(true), table.WithHeight(12),
	)
	m.colTable.SetStyles(dataTableStyles())

	idxCols := []table.Column{{Title: "index", Width: 30}, {Title: "unique", Width: 8}, {Title: "columns", Width: 50}}
	var idxRows []table.Row
	for _, ix := range m.indexes {
		idxRows = append(idxRows, table.Row{ix.Name, ix.Unique, ix.Columns})
	}
	m.idxTable = table.New(
		table.WithColumns(idxCols), table.WithRows(idxRows),
		table.WithFocused(true), table.WithHeight(12),
	)
	m.idxTable.SetStyles(dataTableStyles())

	m.buildRowTable()
}

// buildRowTable rebuilds only the rows tab so paging keeps schema/index cursors.
func (m *Model) buildRowTable() {
	var rowCols []table.Column
	var rowRows []table.Row
	if m.sample != nil {
		for _, c := range m.sample.Columns {
			rowCols = append(rowCols, table.Column{Title: c, Width: 20})
		}
		for _, r := range m.sample.Rows {
			rowRows = append(rowRows, table.Row(r))
		}
	}
	if len(rowCols) == 0 {
		rowCols = []table.Column{{Title: "rows", Width: 40}}
		rowRows = []table.Row{{"(no rows)"}}
	}
	m.rowTable = table.New(
		table.WithColumns(rowCols), table.WithRows(rowRows),
		table.WithFocused(true), table.WithHeight(12),
	)
	m.rowTable.SetStyles(dataTableStyles())
}

// dataTableStyles gives every data table a bright selected row and a
// distinct header so the cursor is never lost in plain text.
func dataTableStyles() table.Styles {
	s := table.DefaultStyles()
	s.Header = s.Header.Foreground(lipgloss.Color("12")).BorderBottom(true).
		BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("8"))
	s.Selected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62"))
	return s
}

// selectedIndexDDL returns the full DDL of the highlighted index for the
// echo line under the indexes table, or "" when none applies.
func (m Model) selectedIndexDDL() string {
	if m.tab != 1 || len(m.indexes) == 0 {
		return ""
	}
	cur := m.idxTable.Cursor()
	if cur < 0 || cur >= len(m.indexes) {
		return ""
	}
	return m.indexes[cur].DDL
}

// setTab switches detail tabs and refits the table to the new chrome
// (pager line, DDL echo) so the layout always fills the terminal.
func (m *Model) setTab(i int) {
	m.tab = i
	m.sizeTables()
}

func (m *Model) sizeTables() {
	w := m.width - 8
	if w < 40 {
		w = 40
	}
	// Fill the terminal: header, table title, blank, tabs, blank, footer.
	chrome := 6
	if m.err != "" {
		chrome++ // error line
	}
	if m.tab == 2 {
		chrome++ // pager line on the rows tab
	}
	if m.selectedIndexDDL() != "" {
		chrome++ // DDL echo on the indexes tab
	}
	h := m.height - chrome
	if h < 4 {
		h = 4
	}
	for _, t := range []*table.Model{&m.colTable, &m.idxTable, &m.rowTable} {
		t.SetWidth(w)
		t.SetHeight(h)
		cols := t.Columns()
		if len(cols) == 0 {
			continue
		}
		// Distribute width across columns; last column takes the slack.
		per := (w - 4) / len(cols)
		if per < 8 {
			per = 8
		}
		for i := range cols {
			if i == len(cols)-1 {
				cols[i].Width = w - 4 - per*(len(cols)-1)
				if cols[i].Width < 10 {
					cols[i].Width = 10
				}
			} else {
				cols[i].Width = per
			}
		}
		t.SetColumns(cols)
	}
}

func (m Model) View() string {
	switch m.screen {
	case screenConns:
		return m.connsView()
	case screenForm:
		return m.formView()
	}

	header := titleStyle.Render("db-peek") + " " + dimStyle.Render(m.status)
	if m.screen == screenTables {
		body := m.list.View()
		foot := dimStyle.Render("↑↓/wheel navigate • hover/click select • 2×click inspect • / filter • r refresh • c conns • q quit")
		if m.loading {
			foot += "  " + "loading…"
		}
		if m.err != "" {
			foot += "\n" + errStyle.Render(m.err)
		}
		return header + "\n" + body + "\n" + foot
	}

	// Detail screen.
	var b strings.Builder
	b.WriteString(header + "\n")
	b.WriteString(titleStyle.Render("⌂ " + m.table))
	if m.count >= 0 {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  •  %d rows", m.count)))
	}
	b.WriteString("\n\n")
	for i, t := range m.detailTabLabels() {
		if i == m.tab {
			b.WriteString(activeTab.Render(t))
		} else {
			b.WriteString(inactiveTab.Render(t))
		}
		b.WriteString(" ")
	}
	b.WriteString("\n\n")
	if m.loading {
		b.WriteString("loading…\n")
	} else {
		switch m.tab {
		case 0:
			if len(m.cols) == 0 {
				b.WriteString("(no columns)\n")
			} else {
				b.WriteString(m.colTable.View() + "\n")
			}
		case 1:
			if len(m.indexes) == 0 {
				b.WriteString("(no indexes)\n")
			} else {
				b.WriteString(m.idxTable.View() + "\n")
			}
			// Show full DDL for the selected index when postgres/sqlite provides it.
			if ddl := m.selectedIndexDDL(); ddl != "" {
				b.WriteString(dimStyle.Render(ddl) + "\n")
			}
		default:
			b.WriteString(dimStyle.Render(m.pagerLine()) + "\n")
			if m.sample == nil || len(m.sample.Rows) == 0 {
				b.WriteString("(no rows)\n")
			} else {
				b.WriteString(m.rowTable.View() + "\n")
			}
		}
	}
	if m.err != "" {
		b.WriteString(errStyle.Render(m.err) + "\n")
	}
	b.WriteString(dimStyle.Render("hover highlights • click tabs • wheel scroll • 1/2/3 tabs • r reload • esc back • q quit (from list)") + "\n")
	return b.String()
}

func (m Model) connsView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("db-peek — connections") + "\n")
	body := m.conns.View()
	foot := dimStyle.Render("↑↓/wheel navigate • hover/click select • 2×click connect • a add • e edit • d forget • q quit")
	if m.loading {
		foot += "  " + "connecting…"
	}
	if m.status != "" {
		foot += "\n" + dimStyle.Render(m.status)
	}
	if m.err != "" {
		foot += "\n" + errStyle.Render(m.err)
	}
	b.WriteString(body + "\n" + foot)
	return b.String()
}

func (m Model) formView() string {
	var b strings.Builder
	if m.editing == "" {
		b.WriteString(titleStyle.Render("db-peek — new connection") + "\n\n")
	} else {
		b.WriteString(titleStyle.Render("db-peek — edit "+m.editing) + "\n\n")
	}
	b.WriteString(fieldLabel("name", m.formFocus == 0) + "\n")
	b.WriteString(m.nameInput.View() + "\n\n")
	b.WriteString(fieldLabel("connection  (postgres://… • mysql://… • ./app.db)", m.formFocus == 1) + "\n")
	b.WriteString(m.connInput.View() + "\n\n")
	if m.err != "" {
		b.WriteString(errStyle.Render(m.err) + "\n")
	}
	b.WriteString(dimStyle.Render("click/tab switch field • enter save + connect • esc cancel") + "\n")
	return b.String()
}

func fieldLabel(s string, focused bool) string {
	if focused {
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Render("> " + s)
	}
	return dimStyle.Render("  " + s)
}
