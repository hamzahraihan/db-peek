package tui

// Model is the single source of UI state (Elm: Model). It is mutated only
// by Update transitions and read by View; async work arrives as messages.

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
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
	screenBrowse // sidebar table list + detail preview in one split
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

	list        list.Model
	focusDetail bool // browse split: sidebar list vs detail pane
	sidebarW    int  // sidebar width in cells, set on resize
	tab         int  // 0 schema, 1 indexes, 2 rows
	table       string
	cols        []dbpkg.Column
	indexes     []dbpkg.Index
	sample      *dbpkg.Sample
	count       int64
	page        int // 0-based page in the rows tab
	pageSize    int // rows per page: one of pageSizes
	detailSeq   int // guards against stale async detail/page loads
	loading     bool
	err         string
	status      string
	width       int
	height      int
	colTable    dataTable
	idxTable    dataTable
	rowTable    dataTable

	// Mouse: rows per item in each picker (from the item delegates), plus
	// last click for double-click detection and hover deduplication.
	connsItemH     int
	tablesItemH    int
	lastClickAt    time.Time
	lastClickIdx   int
	lastClickWhere screen
	lastHoverIdx   int
	lastHoverWhere screen
	hoverTab       int // tab under the cursor, -1 when none
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

	customFilter := func(term string, targets []string) []list.Rank {
		if re, err := regexp.Compile("(?i)" + term); err == nil {
			var ranks []list.Rank
			for i, target := range targets {
				if re.MatchString(target) {
					ranks = append(ranks, list.Rank{Index: i})
				}
			}
			return ranks
		}
		return list.DefaultFilter(term, targets)
	}

	cdelegate := list.NewDefaultDelegate()
	cdelegate.SetSpacing(0)
	cstyles := list.NewDefaultItemStyles()
	cstyles.SelectedTitle = selTitle
	cstyles.SelectedDesc = selDesc
	cdelegate.Styles = cstyles
	cl := list.New(nil, cdelegate, 0, 0)
	cl.Title = "Connections"
	cl.SetShowStatusBar(true)
	cl.SetFilteringEnabled(true)
	cl.SetShowHelp(false)
	cl.Filter = customFilter

	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	delegate.SetSpacing(0)
	tstyles := list.NewDefaultItemStyles()
	tstyles.SelectedTitle = selTitle
	tstyles.SelectedDesc = selDesc
	delegate.Styles = tstyles
	l := list.New(nil, delegate, 0, 0)
	l.Title = "Tables"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	l.Filter = customFilter

	m := Model{
		connStr: strings.TrimSpace(connStr), store: store,
		conns: cl, list: l, count: -1, pageSize: 10, hoverTab: -1,
		nameInput: nameInput, connInput: connInput,
		connsItemH:  cdelegate.Height() + cdelegate.Spacing(),
		tablesItemH: delegate.Height() + delegate.Spacing(),
	}
	m.refreshConns()
	if m.connStr == "" {
		m.screen = screenConns
	} else {
		m.screen = screenBrowse
		m.loading = true
	}
	return m
}

func (m *Model) refreshConns() {
	profiles := m.store.List()
	items := make([]list.Item, len(profiles))
	for i, p := range profiles {
		items[i] = connItem{name: p.Name, masked: saved.Mask(p.Conn), icon: connIcon}
	}
	m.conns.SetItems(items)
}

func (m Model) Init() tea.Cmd {
	if m.connStr != "" {
		return m.openAndLoad(m.connStr)
	}
	return nil
}

// Pure derivations over model state (no side effects, safe for View).

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

// hasIndexDDL reports whether any index carries DDL, used to reserve the
// echo row independent of cursor position.
func (m Model) hasIndexDDL() bool {
	for _, ix := range m.indexes {
		if ix.DDL != "" {
			return true
		}
	}
	return false
}

// detailTabLabels renders the tab strip; the rows tab shows the page size.
// On narrow terminals the labels shrink so the strip never wraps and
// mouse rows stay aligned. Render and hit-testing share this source.
func (m Model) detailTabLabels() []string {
	full := []string{"1 schema", "2 indexes", fmt.Sprintf("3 rows x%d", m.pageSize)}
	if m.width > 0 {
		w := 0
		for _, t := range full {
			w += lipgloss.Width(inactiveTab.Render(t)) + 1
		}
		if w > m.width {
			return []string{"1", "2", "3"}
		}
	}
	return full
}

// setTab switches detail tabs and refits the table to the new chrome
// (pager line, DDL echo) so the layout always fills the terminal.
func (m *Model) setTab(i int) {
	m.tab = i
	m.sizeTables()
}
