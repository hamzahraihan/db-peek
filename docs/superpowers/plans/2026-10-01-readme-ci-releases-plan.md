# README + CI + Releases Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship README.md, MIT LICENSE, CI matrix, and tag-triggered GoReleaser binaries for db-peek.

**Architecture:** Five independent file-add tasks plus one 3-line version-var edit; no runtime behavior change except `--version` ldflags override. CI validates, release cross-builds.

**Tech Stack:** Go 1.26+, GitHub Actions (checkout v4, setup-go v5, goreleaser-action v6), GoReleaser v2, Markdown.

**Spec:** `docs/superpowers/specs/2026-10-01-readme-ci-releases-design.md`

## Global Constraints

- Go floor 1.26 (`go.mod:3` says `go 1.26.0`).
- Version fallback `0.6.0` when built without ldflags.
- Every README flag/key must match `main.go:38-48` and `internal/tui/whichkey.go:24`.
- No Homebrew/Scoop taps, no changelog automation, no macOS CI runner.
- 6 release assets: linux/darwin/windows x amd64/arm64.

## Review Focus

- `go build -ldflags "-X main.appVersion=v9.9.9"` then `--version` must print `db-peek v9.9.9`, not `0.6.0`.
- README `--query` 200-row cap must match `internal/db/db.go:594` (`if len(s.Rows) >= 200`).
- `ci.yml` windows job must pass with CRLF line endings (spec file warns LF->CRLF).
- Tag `v0.6.1` (no `v` prefix missing) must trigger release, plain `main` push must not.
- README install `go install github.com/hamzahraihan/db-peek@latest` must resolve to repo `origin https://github.com/hamzahraihan/db-peek.git`.

---

### Task 1: MIT LICENSE

**Files:**
- Create: `LICENSE`
- Test: none (text file; verified by existence check in Task 5)

**Interfaces:**
- Consumes: git author `hamzah raihan`, year 2026
- Produces: `LICENSE` referenced by README footer

- [ ] **Step 1: Create LICENSE**

Create `LICENSE` with exact content:

```
MIT License

Copyright (c) 2026 hamzah raihan

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

- [ ] **Step 2: Verify file exists**

Run: `ls LICENSE && head -3 LICENSE`
Expected: shows `MIT License` header

- [ ] **Step 3: Commit**

```bash
git add LICENSE
git commit -m "chore: add MIT LICENSE"
```

### Task 2: Version var wiring for ldflags

**Files:**
- Modify: `main.go:31-34`
- Test: manual `go build` version check (in step)

**Interfaces:**
- Consumes: current `const appVersion = "0.6.0"`
- Produces: `var appVersion` overridable via `-X main.appVersion=...`

- [ ] **Step 1: Edit main.go const to var**

Replace in `main.go`:

```go
// appVersion identifies the build; bump on user-visible changes so stale
// binaries (e.g. a project-local db-peek.exe shadowing the install) are
// diagnosable via --version.
const appVersion = "0.6.0"
```

With:

```go
// appVersion identifies the build; bump on user-visible changes so stale
// binaries (e.g. a project-local db-peek.exe shadowing the install) are
// diagnosable via --version. Overridden at release time via
// -ldflags "-X main.appVersion={{.Version}}"; default is dev fallback.
var appVersion = "0.6.0"
```

- [ ] **Step 2: Verify default version unchanged**

Run: `go build -o /tmp/db-peek-test . && /tmp/db-peek-test --version`
Expected: `db-peek 0.6.0`

- [ ] **Step 3: Verify ldflags override works**

Run: `go build -ldflags "-X main.appVersion=v9.9.9" -o /tmp/db-peek-ldflags . && /tmp/db-peek-ldflags --version`
Expected: `db-peek v9.9.9`

- [ ] **Step 4: Run vet+tests**

Run: `go vet ./... && go test ./...`
Expected: all 4 pkgs ok (`db`, `export`, `saved`, `tui`)

- [ ] **Step 5: Commit**

```bash
git add main.go
git commit -m "chore: appVersion var for goreleaser ldflags"
```

### Task 3: CI workflow

**Files:**
- Create: `.github/workflows/ci.yml`
- Test: YAML parse + `go vet`/`go test` locally ( Actions runner verified on push)

**Interfaces:**
- Consumes: Go versions 1.26/1.27, os ubuntu/windows
- Produces: green `CI` badge target for README

- [ ] **Step 1: Create ci.yml**

Create `.github/workflows/ci.yml` with exact content:

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:

jobs:
  build:
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-latest, windows-latest]
        go: ["1.26", "1.27"]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: ${{ matrix.go }}
          cache: true
      - run: go vet ./...
      - run: go test ./...
```

- [ ] **Step 2: Validate YAML parses**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yml')); print('yaml ok')"`
Expected: `yaml ok` (if pyyaml missing, use `go run` fallback: `python -c "open('.github/workflows/ci.yml').read(); print('exists')"`)

- [ ] **Step 3: Run the same commands CI runs**

Run: `go vet ./... && go test ./...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: vet and test on ubuntu/windows go 1.26/1.27"
```

### Task 4: GoReleaser + tag release workflow

**Files:**
- Create: `.goreleaser.yaml`
- Create: `.github/workflows/release.yml`
- Test: `goreleaser check` + ldflags build from Task 2

**Interfaces:**
- Consumes: `var appVersion` from Task 2
- Produces: 6 binaries + checksums on `v*` tags

- [ ] **Step 1: Create .goreleaser.yaml**

Exact content:

```yaml
version: 2

builds:
  - main: .
    binary: db-peek
    goos: [linux, windows, darwin]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w -X main.appVersion={{.Version}}

archives:
  - name_template: "{{ .Binary }}_{{ .Os }}_{{ .Arch }}"
    format_overrides:
      - goos: windows
        format: zip

checksum:
  name_template: "checksums.txt"
```

- [ ] **Step 2: Create release.yml**

Exact content:

```yaml
name: Release

on:
  push:
    tags: ["v*"]

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version: "1.27"
          cache: true
      - uses: goreleaser/goreleaser-action@v6
        with:
          distribution: goreleaser
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

- [ ] **Step 3: Verify goreleaser config**

Run: `goreleaser check`
Expected: `Valid config` (if goreleaser not installed, run `go vet ./...` and note skip in commit message body)

- [ ] **Step 4: Commit**

```bash
git add .goreleaser.yaml .github/workflows/release.yml
git commit -m "chore: goreleaser 6 binaries on v* tags"
```

### Task 5: README + final verification

**Files:**
- Create: `README.md`
- Test: README commands against `test.db`

**Interfaces:**
- Consumes: `LICENSE` (Task 1), CI badge (Task 3), release assets (Task 4), flags `main.go:38-48`, keys `whichkey.go:24`, export `export.go:31`
- Produces: installable project front page

- [ ] **Step 1: Create README.md**

Exact content:

```markdown
# db-peek

[![CI](https://github.com/hamzahraihan/db-peek/actions/workflows/ci.yml/badge.svg)](https://github.com/hamzahraihan/db-peek/actions)
[![Release](https://github.com/hamzahraihan/db-peek/actions/workflows/release.yml/badge.svg)](https://github.com/hamzahraihan/db-peek/actions)
![Go 1.26+](https://img.shields.io/badge/go-1.26+-blue)

Peek at Postgres, MySQL, and SQLite from the terminal. Interactive TUI plus one-shot CLI (`--list --schema --rows --query`).

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

Conn forms: `postgres://...` | `mysql://...` | `./app.db` | `:memory:` | `sqlite://path`.
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
| `--version` | print version |

## TUI keys (`?` shows all in-app)

| Where | Keys |
|-------|------|
| Global | `q` quit, `tab` pane, `?` help, `esc` cancel query |
| Sidebar | `/` filter, `enter` preview, `left/right` collapse/expand, `r` refresh, `c` connections |
| Detail | `1-5` schema/indexes/rows/query/er, `n/p` page, `s` size, `g/G` top/bottom, `,/.` column, `e` edit cell, `E` export |
| Query editor | `ctrl+r/F5` run, `ctrl+e/ctrl+y` explain, `ctrl+p/n` history, `ctrl+t` new buf, `ctrl+w` close, `ctrl+d` del line |
| Results | `H/L` buf, `t/X` new/close |
| Connections | `a/e/d` add/edit/forget, `enter` connect |

## Saved connections

Stored under the OS config dir (`db-peek/connections.json`, file `0600`); passwords masked in display. History and session restore automatically.

## Export

In TUI press `E`: `.csv` `.tsv` `.json` `.md` by extension. NULL is empty (JSON null).

## Dev

```sh
go vet ./...
go test ./...
go build ./...
```

## License

MIT — see [LICENSE](LICENSE).
```

- [ ] **Step 2: Verify README commands (read-only)**

Run: `go build -o /tmp/db-peek-readme . && /tmp/db-peek-readme ./test.db --list && /tmp/db-peek-readme ./test.db --rows sqlite_master --limit 2 2>&1 | head -20`
Expected: table list prints, no error (adjust table name to first `--list` result if `sqlite_master` empty; still PASS if `(no rows)`)

- [ ] **Step 3: Verify version + full suite**

Run: `go vet ./... && go test ./... && /tmp/db-peek-readme --version`
Expected: vet clean, 4 pkgs ok, `db-peek 0.6.0`

Run: `grep -n "len(s.Rows) >= 200" internal/db/db.go`
Expected: match at `db.go:594` proving README 200-row cap matches code

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: add README quickstart flags keys"
```
