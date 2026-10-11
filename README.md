# db-peek

[![CI](https://github.com/hamzahraihan/db-peek/actions/workflows/ci.yml/badge.svg)](https://github.com/hamzahraihan/db-peek/actions)
[![Release](https://github.com/hamzahraihan/db-peek/actions/workflows/release.yml/badge.svg)](https://github.com/hamzahraihan/db-peek/actions)
![Go 1.26+](https://img.shields.io/badge/go-1.26+-blue)

Peek at Postgres, MySQL, MariaDB, SQL Server, ClickHouse, Redis, MongoDB, and SQLite from the terminal. Interactive TUI plus one-shot CLI (`--list --schema --rows --query`).

## Install

```sh
go install github.com/hamzahraihan/db-peek@latest
# or download db-peek_<os>_<arch>.zip/tar.gz + checksums.txt from a v* release
go build -o db-peek .
```

Requires Go 1.26+ for source builds.

## Quickstart

```sh
db-peek ./ecommerce.db
db-peek "postgres://user:pass@localhost:5432/db" --list
DATABASE_URL=postgres://user:pass@localhost:5432/db db-peek --list
db-peek ./ecommerce.db --rows users --limit 20 --offset 0
db-peek ./ecommerce.db --schema users
db-peek ./ecommerce.db --query "SELECT 1"
```

Conn forms: `postgres://...` | `mysql://...` | `mariadb://...` | `sqlserver://...` | `clickhouse://...` | `redis://...` | `mongodb://...` | `./app.db` | `:memory:` | `sqlite://path`.
A bare saved NAME resolves to it. `DATABASE_URL` fills conn when no arg is given.
SQLite paths must exist; nothing is auto-created.

## CLI flags

| Flag | Effect |
|------|--------|
| `--db STR` | connection string |
| `--list` | print table names |
| `--schema TABLE` | columns + indexes |
| `--rows TABLE` | print N rows (`--limit`, `--offset`) |
| `--query SQL` | SELECT prints up to 200 rows; writes print N rows affected |
| `--limit N` | row limit for `--rows` (default 10) |
| `--offset N` | rows to skip (default 0) |
| `--save NAME [conn]` | save connection |
| `--forget NAME` | delete saved connection |
| `--conns` | list saved connections |
| `--read-only` | refuse writes and cell edits (SELECT/reads only) |
| `--version` | print version |

## TUI keys (`?` shows all in-app)

| Where | Keys |
|-------|------|
| Global | `q` quit, `tab` pane, `?` help, `esc` cancel query |
| Sidebar | `/` filter, `enter` preview, `left/right` collapse/expand, `r` refresh, `c` connections |
| Detail | `1-5` schema/indexes/rows/query/er, `n/p` page, `s` size, `g/G` top/bottom, `,/.` column, `o` sort page, `/` find, `d` copy DDL, `e` edit cell, `E` export |
| Query editor | `ctrl+r/F5` run (selection, else statement at cursor), `ctrl+e/ctrl+y` explain, `ctrl+p/n` history, `ctrl+t` new buf, `ctrl+w` close, `ctrl+d` del line, `ctrl+o` open `.sql`, `F2` save `.sql` |
| Results | `H/L` buf, `t/X` new/close, `o` sort page, `/` find |
| Connections | `a/e/d` add/edit/forget, `enter` connect |

## Saved connections

Stored under the OS config dir (`db-peek/connections.json`, file `0600`); passwords masked in display. History and session restore automatically.

## Export

In TUI press `E`: `.csv` `.tsv` `.json` `.md` `.sql` by extension. NULL is empty (JSON null).

## Dev

```sh
go vet ./...
go test ./...
go build ./...
```

## License

MIT — see [LICENSE](LICENSE).
