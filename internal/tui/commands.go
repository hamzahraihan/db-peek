package tui

// Commands are the side-effecting half of MVU: each returns a tea.Cmd that
// runs off the UI loop and reports back through a message in msg.go.

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	dbpkg "db-peek/internal/db"
)

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

func (m Model) loadDetail(table string) tea.Cmd {
	db := m.db
	size := m.pageSize
	seq := m.detailSeq
	conn := m.connSeq
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cols, err := db.Columns(ctx, table)
		if err != nil {
			return detailLoadedMsg{table: table, err: err, seq: seq, conn: conn}
		}
		idx, err := db.Indexes(ctx, table)
		if err != nil {
			return detailLoadedMsg{table: table, err: err, seq: seq, conn: conn}
		}
		sample, err := db.PageRows(ctx, table, size, 0)
		if err != nil {
			return detailLoadedMsg{table: table, err: err, seq: seq, conn: conn}
		}
		count, err := db.Count(ctx, table)
		if err != nil {
			count = -1 // sample still useful; count failure is non-fatal
		}
		return detailLoadedMsg{table: table, cols: cols, indexes: idx, sample: sample, count: count, seq: seq, conn: conn}
	}
}

// loadRowsPage fetches one page of the current table for the rows tab.
func (m Model) loadRowsPage() tea.Cmd {
	db, table, size, page := m.db, m.table, m.pageSize, m.page
	seq := m.detailSeq
	conn := m.connSeq
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		sample, err := db.PageRows(ctx, table, size, page*size)
		if err != nil {
			return rowsPageMsg{err: err, seq: seq, conn: conn}
		}
		return rowsPageMsg{sample: sample, page: page, seq: seq, conn: conn}
	}
}

func (m Model) loadSchemas() tea.Cmd {
	db := m.db
	conn := m.connSeq
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
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

func (m Model) loadOneCount(schema, table string) tea.Cmd {
	db := m.db
	conn := m.connSeq
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		n, err := db.ApproxCount(ctx, schema, table)
		return tableCountMsg{schema: schema, table: table, count: n, err: err, conn: conn}
	}
}

func (m Model) runQuery() tea.Cmd {
	db, sql, seq := m.db, m.editor.Text(), m.querySeq
	conn := m.connSeq
	return func() tea.Msg {
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		s, n, err := db.RunUserQuery(ctx, sql)
		ms := time.Since(start).Milliseconds()
		if err != nil {
			return queryDoneMsg{sql: sql, ms: ms, seq: seq, err: err, conn: conn}
		}
		if n < 0 {
			// SELECT or RETURNING path: returned rows are the result view.
			return queryDoneMsg{sql: sql, sample: s, affected: -1, ms: ms, seq: seq, conn: conn}
		}
		tbl, isDDL := dbpkg.PreviewTarget(sql)
		if isDDL {
			return queryDoneMsg{sql: sql, affected: n, isDDL: true, ms: ms, seq: seq, conn: conn}
		}
		if tbl == "" {
			return queryDoneMsg{sql: sql, affected: n, ms: ms, seq: seq, conn: conn}
		}
		preview, perr := db.PageRows(ctx, tbl, 20, 0)
		if perr != nil {
			// Swallowed by design: affected-count is the source of truth.
			return queryDoneMsg{sql: sql, affected: n, ms: time.Since(start).Milliseconds(), seq: seq, conn: conn}
		}
		return queryDoneMsg{sql: sql, sample: preview, affected: n, previewTable: tbl, ms: time.Since(start).Milliseconds(), seq: seq, conn: conn}
	}
}

func (m Model) loadER(table string) tea.Cmd {
	db, seq := m.db, m.erSeq
	conn := m.connSeq
	if links, ok := m.erCache[table]; ok {
		return func() tea.Msg { return erLoadedMsg{table: table, links: links, seq: seq, conn: conn} }
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		links, err := db.ForeignKeys(ctx, table)
		return erLoadedMsg{table: table, links: links, seq: seq, err: err, conn: conn}
	}
}

// loadERSchema fetches columns for every table plus all FKs (bounded
// concurrency 4, 15s timeout). Tables come from the caller (current schema).
func (m Model) loadERSchema(schema string, tables []string) tea.Cmd {
	db, seq := m.db, m.erSeq
	conn := m.connSeq
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
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
		_ = schema // scoping is explorer-side; db calls take plain table names
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
}

// loadColumns resolves table names within the connection's default
// schema/search_path (consistent with the ApproxCount parked ruling):
// the schema arg scopes explorer state only; db.Columns takes a plain
// table name used in information_schema queries. Do not qualify here and
// do not change the db package.
func (m Model) loadColumns(schema, table string) tea.Cmd {
	db := m.db
	conn := m.connSeq
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cols, err := db.Columns(ctx, table)
		return columnsLoadedMsg{schema: schema, table: table, columns: cols, err: err, conn: conn}
	}
}
