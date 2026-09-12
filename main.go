// Command db-peek inspects a database from the terminal.
//
//	Usage:
//	  db-peek [conn|name]            interactive TUI (connection picker when empty)
//	  db-peek [conn|name] --list     print table names
//	  db-peek [conn|name] --schema TABLE  print columns + indexes
//	  db-peek [conn|name] --rows TABLE    print first N rows (see --limit)
//	  db-peek [conn|name] --query SQL     run a SELECT and print up to 200 rows
//	  db-peek --conns                list saved connections
//	  db-peek --save NAME [conn]     save the connection as NAME
//	  db-peek --forget NAME          delete a saved connection
//
//	Conn forms: postgres://... | mysql://... | ./app.db | :memory: | sqlite://path
//	A bare NAME matching a saved connection resolves to it. Env DATABASE_URL
//	is used when no argument is given.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	dbpkg "db-peek/internal/db"
	"db-peek/internal/saved"
	"db-peek/internal/tui"
)

// appVersion identifies the build; bump on user-visible changes so stale
// binaries (e.g. a project-local db-peek.exe shadowing the install) are
// diagnosable via --version.
const appVersion = "0.6.0"

func main() {
	var (
		dbFlag   = flag.String("db", "", "connection string (postgres://, mysql://, sqlite path)")
		listF    = flag.Bool("list", false, "print table names and exit")
		schemaF  = flag.String("schema", "", "print columns + indexes for TABLE and exit")
		rowsF    = flag.String("rows", "", "print N rows of TABLE and exit (see --limit/--offset)")
		queryF   = flag.String("query", "", "run SELECT SQL and exit")
		limitF   = flag.Int("limit", 10, "row limit for --rows")
		offsetF  = flag.Int("offset", 0, "rows to skip for --rows")
		saveF    = flag.String("save", "", "save the connection as NAME and exit")
		forgetF  = flag.String("forget", "", "delete saved connection NAME and exit")
		connsF   = flag.Bool("conns", false, "list saved connections and exit")
		versionF = flag.Bool("version", false, "print version and exit")
	)
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, `db-peek — peek at databases from the terminal

Usage:
  db-peek [conn|name]            interactive TUI (picker when empty)
  db-peek [conn|name] --list     print table names
  db-peek [conn|name] --rows TABLE    print N rows (--limit, --offset)
  db-peek [conn|name] --query SQL     run a SELECT, print up to 200 rows
  db-peek --conns                list saved connections
  db-peek --save NAME [conn]     save the connection as NAME
  db-peek --forget NAME          delete a saved connection

Conn: postgres://... | mysql://... | ./app.db | :memory: | sqlite://path
or a saved NAME. Env DATABASE_URL fills conn when no argument is given.

`)
		flag.PrintDefaults()
	}
	// Accept `db-peek [conn] --flags` as well as `db-peek --flags [conn]`:
	// the std flag package stops parsing at the first positional, so hoist a
	// leading conn argument out before parsing.
	connArg := ""
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		connArg = os.Args[1]
		os.Args = append([]string{os.Args[0]}, os.Args[2:]...)
	}
	flag.Parse()

	if *versionF {
		fmt.Println("db-peek " + appVersion)
		return
	}

	store, err := saved.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "db-peek: warning: saved connections unavailable:", err)
		store = &saved.Store{}
	}

	if *connsF {
		for _, p := range store.List() {
			fmt.Printf("%s\t%s\n", p.Name, saved.Mask(p.Conn))
		}
		return
	}
	if *forgetF != "" {
		if err := store.Delete(*forgetF); err != nil {
			fatal(err)
		}
		fmt.Fprintf(os.Stderr, "forgot %q\n", *forgetF)
		return
	}

	conn := strings.TrimSpace(*dbFlag)
	if conn == "" {
		conn = strings.TrimSpace(connArg)
	}
	if conn == "" && flag.NArg() > 0 {
		conn = strings.TrimSpace(flag.Arg(0))
	}
	if conn == "" {
		conn = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	// A bare NAME matching a saved connection resolves to it.
	if conn != "" {
		if p, ok := store.Get(conn); ok {
			conn = p.Conn
		}
	}
	if *saveF != "" {
		if conn == "" {
			fmt.Fprintln(os.Stderr, "db-peek: --save needs a connection: db-peek --save NAME [conn]")
			os.Exit(2)
		}
		if err := store.Upsert(*saveF, conn); err != nil {
			fatal(err)
		}
		fmt.Fprintf(os.Stderr, "saved %q\n", *saveF)
		return
	}

	oneShot := *listF || *schemaF != "" || *rowsF != "" || *queryF != ""
	if !oneShot {
		if err := tui.Run(conn, store); err != nil {
			fmt.Fprintln(os.Stderr, "db-peek:", err)
			os.Exit(1)
		}
		return
	}

	if conn == "" {
		fmt.Fprintln(os.Stderr, "db-peek: need a connection string: db-peek [conn] --list")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	db, err := dbpkg.Open(conn)
	if err != nil {
		fatal(err)
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		fatal(fmt.Errorf("connect failed: %w", err))
	}

	switch {
	case *listF:
		names, err := db.ListTables(ctx)
		if err != nil {
			fatal(err)
		}
		for _, n := range names {
			fmt.Println(n)
		}
	case *schemaF != "":
		cols, err := db.Columns(ctx, *schemaF)
		if err != nil {
			fatal(err)
		}
		if len(cols) == 0 {
			fatal(fmt.Errorf("table %q not found", *schemaF))
		}
		fmt.Printf("== %s: columns ==\n", *schemaF)
		render([]string{"column", "type", "null", "default", "extra"}, colRows(cols))
		idx, err := db.Indexes(ctx, *schemaF)
		if err != nil {
			fatal(err)
		}
		fmt.Printf("\n== %s: indexes ==\n", *schemaF)
		if len(idx) == 0 {
			fmt.Println("(none)")
			return
		}
		rows := make([][]string, len(idx))
		for i, x := range idx {
			rows[i] = []string{x.Name, x.Unique, x.Columns}
		}
		render([]string{"index", "unique", "columns"}, rows)
		for _, x := range idx {
			if x.DDL != "" {
				fmt.Println("  " + x.DDL + ";")
			}
		}
	case *rowsF != "":
		n, err := db.Count(ctx, *rowsF)
		if err == nil {
			fmt.Printf("== %s (%d rows, limit %d offset %d) ==\n", *rowsF, n, *limitF, *offsetF)
		}
		s, err := db.PageRows(ctx, *rowsF, *limitF, *offsetF)
		if err != nil {
			fatal(err)
		}
		render(s.Columns, s.Rows)
	case *queryF != "":
		s, err := db.Query(ctx, *queryF)
		if err != nil {
			fatal(err)
		}
		render(s.Columns, s.Rows)
	}
}

func colRows(cols []dbpkg.Column) [][]string {
	rows := make([][]string, len(cols))
	for i, c := range cols {
		rows[i] = []string{c.Name, c.Type, c.Nullable, c.Default, c.Extra}
	}
	return rows
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "db-peek:", err)
	os.Exit(1)
}

// render prints a clean left-aligned terminal table.
func render(header []string, rows [][]string) {
	if len(header) == 0 {
		return
	}
	widths := make([]int, len(header))
	for i, h := range header {
		widths[i] = runeLen(h)
	}
	for _, r := range rows {
		for i := range header {
			if i < len(r) && runeLen(r[i]) > widths[i] {
				widths[i] = runeLen(r[i])
			}
		}
	}
	const maxW = 40
	for i := range widths {
		if widths[i] > maxW {
			widths[i] = maxW
		}
	}
	line := func(cells []string) string {
		var b strings.Builder
		for i := range header {
			v := ""
			if i < len(cells) {
				v = cells[i]
			}
			b.WriteString(pad(v, widths[i]))
			if i < len(header)-1 {
				b.WriteString("  ")
			}
		}
		return strings.TrimRight(b.String(), " ")
	}
	fmt.Println(line(header))
	sep := make([]string, len(header))
	for i, w := range widths {
		sep[i] = strings.Repeat("-", w)
	}
	fmt.Println(line(sep))
	if len(rows) == 0 {
		fmt.Println("(no rows)")
		return
	}
	for _, r := range rows {
		fmt.Println(line(r))
	}
}

func pad(s string, w int) string {
	r := []rune(s)
	if len(r) > w {
		return string(r[:w-1]) + "…"
	}
	return s + strings.Repeat(" ", w-len(r))
}

func runeLen(s string) int { return len([]rune(s)) }
