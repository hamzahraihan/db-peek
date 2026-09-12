package tui

import dbpkg "db-peek/internal/db"

// Messages are the events Update reacts to. Async DB work reports back
// through these; every other branch of Update is a state transition.
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
