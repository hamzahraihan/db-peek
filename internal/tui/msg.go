package tui

import dbpkg "db-peek/internal/db"

// Messages are the events Update reacts to. Async DB work reports back
// through these; every other branch of Update is a state transition.
type (
	detailLoadedMsg struct {
		table   string
		schema  string // explorer schema of the table; "" when not explorer-driven
		cols    []dbpkg.Column
		indexes []dbpkg.Index
		sample  *dbpkg.Sample
		count   int64
		seq     int // detailSeq at request time; stale replies are dropped
		conn    int // connSeq at request time; other-connection replies are dropped
		colsErr error // set only when db.Columns failed, so a column failure
		// collapses the tree node while an unrelated failure does not
		err error
	}
	// detailCountMsg carries the exact row count for the open table. It
	// arrives after the grid has already painted, so a slow COUNT(*) on
	// a huge table no longer blocks the first paint.
	detailCountMsg struct {
		table string
		count int64
		seq   int
		conn  int
	err   error
	}
	rowsPageMsg struct {
		sample *dbpkg.Sample
		page   int
		seq    int
		conn   int
		err    error
	}
	connectMsg struct {
		db  *dbpkg.DB
		err error
	}
	schemasLoadedMsg struct {
		schemas []string
		tables  map[string][]dbpkg.TableRef
		conn    int
		err     error
	}
	// countsLoadedMsg carries ApproxCount for every table in one schema,
	// fetched concurrently (bounded at 4, mirroring loadERSchema). errs
	// latches per table so a failure renders "?" and is never retried.
	countsLoadedMsg struct {
		schema string
		counts map[string]int64
		errs   map[string]bool
		conn   int
	}
	queryDoneMsg struct {
		sql          string
		sample       *dbpkg.Sample
		affected     int64  // rows affected for writes, -1 for row-returning queries
		previewTable string // auto-preview target for writes, "" when none
		isDDL        bool   // true when the write was DDL (refresh schemas, no grid)
		ms           int64
		seq          int
		conn         int
		qbufID       int // query buffer id at run time; replies for closed buffers are dropped
		err          error
	}
	erLoadedMsg struct {
		table string
		links []dbpkg.ForeignKey
		seq   int
		conn  int
		err   error
	}
	erSchemaLoadedMsg struct {
		tables []erTable
		links  []dbpkg.ForeignKey
		seq    int
		conn   int
		err    error
	}
)

// historySavedMsg reports the outcome of persisting the query history.
// A failure is a status-line note, never a query error.
type historySavedMsg struct {
	err error
}

// cellUpdatedMsg reports the outcome of one cell UPDATE.
type cellUpdatedMsg struct {
	table, col string
	affected   int64
	seq, conn  int
	err        error
}
