package tui

// Commands are the side-effecting half of MVU: each returns a tea.Cmd that
// runs off the UI loop and reports back through a message in msg.go.

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	dbpkg "db-peek/internal/db"
	"db-peek/internal/saved"
)

// newOpContext cancels any in-flight command and returns a fresh bounded
// context for a new one, registering the cancel on the model so esc (or
// the next selection) can abort it. Every command constructor must build
// its context here, never inside the returned closure. Siblings issued
// together in one tea.Batch must share a single context: constructing a
// second one would cancel the first before it runs.
func (m *Model) newOpContext(d time.Duration) context.Context {
	if m.cancelInFlight != nil {
		m.cancelInFlight()
	}
	ctx, cancel := context.WithTimeout(context.Background(), d)
	m.cancelInFlight = cancel
	return ctx
}

// cancelOp aborts the in-flight command (if any) and clears loading.
// Safe to call when nothing is running.
func (m *Model) cancelOp() {
	if m.cancelInFlight != nil {
		m.cancelInFlight()
		m.cancelInFlight = nil
	}
	m.loading = false
}

func (m *Model) openAndLoad(connStr string) tea.Cmd {
	ctx := m.newOpContext(10 * time.Second)
	return func() tea.Msg {
		db, err := dbpkg.Open(connStr)
		if err != nil {
			return connectMsg{err: err}
		}
		if err := db.Ping(ctx); err != nil {
			_ = db.Close()
			return connectMsg{err: fmt.Errorf("connect failed: %w", err)}
		}
		return connectMsg{db: db}
	}
}

// saveHistoryCmd persists the store off the UI goroutine; a failure is
// reported in the status line, never as a query error.
func saveHistoryCmd(h *saved.History) tea.Cmd {
	if h == nil {
		return nil
	}
	return func() tea.Msg { return historySavedMsg{err: h.Save()} }
}

func (m *Model) openSaved(name string) tea.Cmd {
	p, ok := m.store.Get(name)
	if !ok {
		return func() tea.Msg { return connectMsg{err: fmt.Errorf("no saved connection %q", name)} }
	}
	m.connStr = p.Conn
	return m.openAndLoad(p.Conn)
}

// loadDetail fetches columns, indexes and the first page, then hands the
// exact row count to a sibling command sharing the same context: an exact
// COUNT(*) on a large table would otherwise hold the first paint hostage.
func (m *Model) loadDetail(schema, table string) tea.Cmd {
	return m.loadDetailCtx(m.newOpContext(15*time.Second), schema, table)
}

// loadDetailCtx is loadDetail under a caller-owned context, for the schema
// load that issues its detail load and its count batch as one cancellable
// operation.
func (m *Model) loadDetailCtx(ctx context.Context, schema, table string) tea.Cmd {
	db := m.db
	size := m.pageSize
	seq := m.detailSeq
	conn := m.connSeq
	sampleCmd := func() tea.Msg {
		cols, err := db.Columns(ctx, table)
		if err != nil {
			return detailLoadedMsg{table: table, schema: schema, err: err, colsErr: err, seq: seq, conn: conn}
		}
		idx, err := db.Indexes(ctx, table)
		if err != nil {
			return detailLoadedMsg{table: table, schema: schema, err: err, seq: seq, conn: conn}
		}
		sample, err := db.PageRows(ctx, table, size, 0)
		if err != nil {
			return detailLoadedMsg{table: table, schema: schema, err: err, seq: seq, conn: conn}
		}
		return detailLoadedMsg{
			table:   table,
			schema:  schema,
			cols:    cols,
			indexes: idx,
			sample:  sample,
			count:   -1,
			seq:     seq,
			conn:    conn,
		}
	}
	return tea.Batch(sampleCmd, m.loadCount(ctx, table))
}

// loadCount resolves the exact row count. A count the sidebar already
// fetched (m.counts) answers without touching the database.
func (m *Model) loadCount(ctx context.Context, table string) tea.Cmd {
	db := m.db
	seq := m.detailSeq
	conn := m.connSeq
	if cached, ok := m.counts[table]; ok {
		return func() tea.Msg { return detailCountMsg{table: table, count: cached, seq: seq, conn: conn} }
	}
	return func() tea.Msg {
		n, err := db.Count(ctx, table)
		return detailCountMsg{table: table, count: n, seq: seq, conn: conn, err: err}
	}
}

// loadRowsPage fetches one page of the current table for the rows tab.
func (m *Model) loadRowsPage() tea.Cmd {
	db, table, size, page := m.db, m.table, m.pageSize, m.page
	seq := m.detailSeq
	conn := m.connSeq
	ctx := m.newOpContext(15 * time.Second)
	return func() tea.Msg {
		sample, err := db.PageRows(ctx, table, size, page*size)
		if err != nil {
			return rowsPageMsg{err: err, seq: seq, conn: conn}
		}
		return rowsPageMsg{sample: sample, page: page, seq: seq, conn: conn}
	}
}

func (m *Model) loadSchemas() tea.Cmd {
	db := m.db
	conn := m.connSeq
	ctx := m.newOpContext(10 * time.Second)
	return func() tea.Msg {
		schemas, err := db.ListSchemas(ctx)
		if err != nil {
			return schemasLoadedMsg{err: err, conn: conn}
		}
		tables := map[string][]dbpkg.TableRef{}
		for _, s := range schemas {
			refs, err := db.ListTablesInSchema(ctx, s)
			if err != nil {
				return schemasLoadedMsg{err: err, conn: conn}
			}
			tables[s] = refs
		}
		return schemasLoadedMsg{schemas: schemas, tables: tables, conn: conn}
	}
}

// loadCounts fetches every row count in one schema concurrently (bounded
// at 4, mirroring loadERSchema) instead of one message round-trip and one
// serialized query per table.
func (m *Model) loadCounts(ctx context.Context, schema string, tables []string) tea.Cmd {
	db := m.db
	conn := m.connSeq
	names := append([]string(nil), tables...)
	return func() tea.Msg {
		type res struct {
			table string
			count int64
			err   error
		}
		ch := make(chan res, len(names))
		sem := make(chan struct{}, 4)
		for _, t := range names {
			go func() {
				sem <- struct{}{}
				defer func() { <-sem }()
				n, err := db.ApproxCount(ctx, schema, t)
				ch <- res{table: t, count: n, err: err}
			}()
		}
		out := countsLoadedMsg{schema: schema, counts: map[string]int64{}, errs: map[string]bool{}, conn: conn}
		for range names {
			r := <-ch
			if r.err != nil {
				// Latch: a failure renders "?" and is never retried.
				out.errs[r.table] = true
				continue
			}
			out.counts[r.table] = r.count
		}
		return out
	}
}

func (m *Model) runQuery() tea.Cmd {
	m.ensureQueryBufs()
	db, sql, seq := m.db, m.editor.Text(), m.qbufs[m.qcur].seq
	qid := m.qbufs[m.qcur].id
	conn := m.connSeq
	// ctrl+e/ctrl+y explain the buffer instead of running it. The buffer
	// itself is never rewritten, so the plan lands in the grid while the
	// user keeps the statement they typed.
	if m.pendingExplain && db != nil {
		sql = db.Explain(sql, m.pendingAnalyze)
	}
	m.pendingExplain, m.pendingAnalyze = false, false
	ctx := m.newOpContext(15 * time.Second)
	return func() tea.Msg {
		start := time.Now()
		s, n, err := db.RunUserQuery(ctx, sql)
		ms := time.Since(start).Milliseconds()
		if err != nil {
			return queryDoneMsg{sql: sql, ms: ms, seq: seq, err: err, conn: conn, qbufID: qid}
		}
		if n < 0 {
			// SELECT or RETURNING path: returned rows are the result view.
			return queryDoneMsg{sql: sql, sample: s, affected: -1, ms: ms, seq: seq, conn: conn, qbufID: qid}
		}
		tbl, isDDL := dbpkg.PreviewTarget(sql)
		if isDDL {
			return queryDoneMsg{sql: sql, affected: n, isDDL: true, ms: ms, seq: seq, conn: conn, qbufID: qid}
		}
		if tbl == "" {
			return queryDoneMsg{sql: sql, affected: n, ms: ms, seq: seq, conn: conn, qbufID: qid}
		}
		preview, perr := db.PageRows(ctx, tbl, 20, 0)
		if perr != nil {
			// Swallowed by design: affected-count is the source of truth.
			return queryDoneMsg{sql: sql, affected: n, ms: time.Since(start).Milliseconds(), seq: seq, conn: conn, qbufID: qid}
		}
		return queryDoneMsg{sql: sql, sample: preview, affected: n, previewTable: tbl, ms: time.Since(start).Milliseconds(), seq: seq, conn: conn, qbufID: qid}
	}
}

func (m *Model) loadER(table string) tea.Cmd {
	return m.loadERCtx(m.newOpContext(10*time.Second), table)
}

// loadERSchema fetches columns for every table plus all FKs (bounded
// concurrency 4, 15s timeout). Tables come from the caller (current schema).
func (m *Model) loadERSchema(schema string, tables []string) tea.Cmd {
	_ = schema // scoping is explorer-side; db calls take plain table names
	return m.loadERSchemaCtx(m.newOpContext(15*time.Second), schema, tables)
}

// loadERCtx is loadER under a caller-owned context, for the ER tab
// entry that issues the per-table FK load and the schema load as one
// operation: both must share a context or the second cancels the first.
func (m *Model) loadERCtx(ctx context.Context, table string) tea.Cmd {
	db, seq := m.db, m.erSeq
	conn := m.connSeq
	if links, ok := m.erCache[table]; ok {
		return func() tea.Msg { return erLoadedMsg{table: table, links: links, seq: seq, conn: conn} }
	}
	return func() tea.Msg {
		links, err := db.ForeignKeys(ctx, table)
		return erLoadedMsg{table: table, links: links, seq: seq, err: err, conn: conn}
	}
}

// loadERSchemaCtx is loadERSchema under a caller-owned context.
func (m *Model) loadERSchemaCtx(ctx context.Context, schema string, tables []string) tea.Cmd {
	db, seq := m.db, m.erSeq
	conn := m.connSeq
	return func() tea.Msg {
		return erSchemaResult(ctx, db, seq, conn, tables)
	}
}

// erSchemaResult fetches columns for every table plus all FKs (bounded
// concurrency 4). Tables come from the caller (current schema).
func erSchemaResult(ctx context.Context, db *dbpkg.DB, seq, conn int, tables []string) erSchemaLoadedMsg {
	type res struct {
		t    string
		cols []dbpkg.Column
		err  error
	}
	ch := make(chan res, len(tables))
	sem := make(chan struct{}, 4)
	for _, t := range tables {
		t := t
		go func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			cols, err := db.Columns(ctx, t)
			ch <- res{t: t, cols: cols, err: err}
		}()
	}
	byName := map[string][]dbpkg.Column{}
	failed := map[string]bool{}
	var firstErr error
	for range tables {
		r := <-ch
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
			}
			// Drop the failed table so the update branch can
			// tell it apart from a genuinely column-less one
			// and still apply the successfully-loaded rest.
			failed[r.t] = true
			continue
		}
		byName[r.t] = r.cols
	}
	links, err := db.AllForeignKeys(ctx, tables)
	if err != nil && firstErr == nil {
		firstErr = err
	}
	var out []erTable
	for _, t := range tables {
		if failed[t] {
			continue
		}
		cols := byName[t]
		pk := map[string]bool{}
		fk := map[string]bool{}
		for _, c := range cols {
			if len(c.Extra) >= 2 && c.Extra[:2] == "PK" {
				pk[c.Name] = true
			}
		}
		for _, l := range links {
			if l.FromTable == t {
				fk[l.FromColumn] = true
			}
		}
		out = append(out, erTable{name: t, cols: cols, pk: pk, fk: fk})
	}
	return erSchemaLoadedMsg{tables: out, links: links, seq: seq, err: firstErr, conn: conn}
}
