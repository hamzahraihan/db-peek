package tui

import dbpkg "db-peek/internal/db"

// Messages are the events Update reacts to. Async DB work reports back
// through these; every other branch of Update is a state transition.
type (
	detailLoadedMsg struct {
		table   string
		cols    []dbpkg.Column
		indexes []dbpkg.Index
		sample  *dbpkg.Sample
		count   int64
		seq     int // detailSeq at request time; stale replies are dropped
		conn    int // connSeq at request time; other-connection replies are dropped
		err     error
	}
	rowsPageMsg struct {
		sample *dbpkg.Sample
		page   int
		seq    int
		conn   int
		err    error
	}
	connectMsg struct {
		db    *dbpkg.DB
		names []string
		err   error
	}
	schemasLoadedMsg struct {
		schemas []string
		tables  map[string][]dbpkg.TableRef
		conn    int
		err     error
	}
	tableCountMsg struct {
		schema string
		table  string
		count  int64
		conn   int
		err    error
	}
	columnsLoadedMsg struct {
		schema  string
		table   string
		columns []dbpkg.Column
		conn    int
		err     error
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
