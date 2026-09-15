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
	// connSeq is the connection generation: bumped every time we start
	// switching connections (activate/disconnect). Async DB replies carry
	// the generation they were issued under and are dropped when it no
	// longer matches, so a slow Supabase response can never paint stale
	// rows, counts, columns or ER boxes into the new session.
	connSeq int

	conns  list.Model
	delArm string // profile name armed for delete confirmation

	nameInput textinput.Model
	connInput textinput.Model
	formFocus int    // 0 name, 1 conn
	editing   string // profile being edited, "" when adding

	explorer    Explorer
	filterInput textinput.Model
	filtering   bool // sidebar filter input focused; keystrokes belong to it
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

	editor      Editor
	queryFocus  int // 0 editor, 1 results (only meaningful when tab==3)
	querySample *dbpkg.Sample
	queryMs     int64
	querySeq    int
	queryTable  dataTable

	erLinks  []dbpkg.ForeignKey
	erSeq    int
	erOffset int
	erCache  map[string][]dbpkg.ForeignKey

	erSchema erSchemaState
	erPanX   int
	erPanY   int
	erSel    string
	hoverER  string
	erFocus  bool   // true = 1-hop focused view around erCenter (default); false = full schema
	erCenter string // diagram center table for focused view

	showHelp bool // which-key overlay (Task 7): modal, toggled by ?

	// Mouse: rows per item in each picker (from the item delegates), plus
	// last click for double-click detection and hover deduplication.
	connsItemH     int
	lastClickAt    time.Time
	lastClickIdx   int
	lastClickWhere screen
	lastHoverIdx   int
	lastHoverWhere screen
	hoverTab       int // tab under the cursor, -1 when none
}

// erTable is one box on the ER canvas.
type erTable struct {
	name string
	cols []dbpkg.Column
	pk   map[string]bool
	fk   map[string]bool
}

// erSchemaState is the full-schema ER cache (one load per session).
type erSchemaState struct {
	tables []erTable
	links  []dbpkg.ForeignKey
	loaded bool
	err    string
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

	filterInput := textinput.New()
	filterInput.Prompt = "/"
	filterInput.Placeholder = "filter tables"
	filterInput.CharLimit = 64
	filterInput.Width = 32

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

	m := Model{
		connStr: strings.TrimSpace(connStr), store: store,
		conns: cl, explorer: NewExplorer("", []string{}), count: -1, pageSize: 10, hoverTab: -1,
		nameInput: nameInput, connInput: connInput, filterInput: filterInput,
		connsItemH: cdelegate.Height() + cdelegate.Spacing(),
		editor:     NewEditor(),
		erFocus:    true,
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
	full := []string{"1 schema", "2 indexes", fmt.Sprintf("3 rows x%d", m.pageSize), "4 query", "5 er"}
	if m.width > 0 {
		w := 0
		for _, t := range full {
			w += lipgloss.Width(inactiveTab.Render(t)) + 1
		}
		// The strip renders inside the detail pane, so compare against
		// the pane width (not the terminal width) to keep render and
		// hit-testing aligned. Unknown width still yields full labels.
		if w > m.paneInnerW() {
			return []string{"1", "2", "3", "4", "5"}
		}
	}
	return full
}

// setTab switches detail tabs and refits the table to the new chrome
// (pager line, DDL echo) so the layout always fills the terminal.
// Entering the ER tab bumps erSeq and kicks off an ER load (cached
// when the table was already visited). The diagram defaults to the
// focused 1-hop view centered on the current table.
func (m *Model) setTab(i int) tea.Cmd {
	m.tab = i
	if i == 4 {
		m.erSeq++
		if m.erCenter == "" || m.erCenter != m.table {
			m.erCenter = m.table
			m.erSel = m.table
			m.erPanX, m.erPanY = 0, 0
		}
		m.sizeTables()
		if m.erSchema.loaded {
			return nil
		}
		var names []string
		for _, s := range m.explorer.Schemas {
			for _, tb := range s.Tables {
				names = append(names, tb.Name)
			}
		}
		if len(names) == 0 && m.table != "" {
			names = []string{m.table}
		}
		return m.loadERSchema("", names)
	}
	m.sizeTables()
	return nil
}
