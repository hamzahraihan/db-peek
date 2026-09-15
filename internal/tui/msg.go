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
		err     error
	}
	rowsPageMsg struct {
		sample *dbpkg.Sample
		page   int
		seq    int
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
		err     error
	}
	tableCountMsg struct {
		schema string
		table  string
		count  int64
		err    error
	}
	columnsLoadedMsg struct {
		schema  string
		table   string
		columns []dbpkg.Column
		err     error
	}
	queryDoneMsg struct {
		sql    string
		sample *dbpkg.Sample
		ms     int64
		seq    int
		err    error
	}
	erLoadedMsg struct {
		table string
		links []dbpkg.ForeignKey
		seq   int
		err   error
	}
	erSchemaLoadedMsg struct {
		tables []erTable
		links  []dbpkg.ForeignKey
		seq    int
		err    error
	}
)
