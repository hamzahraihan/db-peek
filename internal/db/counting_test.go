package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// countingConn counts the queries that reach the driver so a test can
// assert on statement counts rather than wall-clock time.
type countingConn struct {
	driver.Conn
	pragmas *atomic.Int64
}

func (c countingConn) QueryContext(ctx context.Context, q string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(strings.ToLower(q), "foreign_key_list") {
		c.pragmas.Add(1)
	}
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, q, args)
}

type countingDriver struct {
	inner   driver.Driver
	pragmas *atomic.Int64
}

func (d countingDriver) Open(name string) (driver.Conn, error) {
	conn, err := d.inner.Open(name)
	if err != nil {
		return nil, err
	}
	return countingConn{Conn: conn, pragmas: d.pragmas}, nil
}

// countingConnector lets sql.OpenDB reach the wrapped driver.
type countingConnector struct {
	inner   driver.Driver
	pragmas *atomic.Int64
}

func (c countingConnector) Connect(context.Context) (driver.Conn, error) {
	return countingDriver{inner: c.inner, pragmas: c.pragmas}.Open("")
}

func (c countingConnector) Driver() driver.Driver { return c.inner }

// openCountedDB opens a file-backed SQLite database and returns it with a
// counter of the PRAGMA foreign_key_list statements the driver has seen.
// A file (not :memory:) keeps every pooled connection on the same schema.
func openCountedDB(t *testing.T) (*DB, *atomic.Int64) {
	t.Helper()
	probe, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "seed.db"))
	if err != nil {
		t.Fatal(err)
	}
	inner := probe.Driver()
	probe.Close()

	var pragmas atomic.Int64
	sqlDB := sql.OpenDB(countingConnector{inner: inner, pragmas: &pragmas})
	t.Cleanup(func() { sqlDB.Close() })
	return &DB{SQL: sqlDB, Driver: SQLite, Display: "counted.db"}, &pragmas
}

var (
	_ driver.Conn           = countingConn{}
	_ driver.QueryerContext = countingConn{}
	_ driver.Driver         = countingDriver{}
	_ driver.Connector      = countingConnector{}
)
