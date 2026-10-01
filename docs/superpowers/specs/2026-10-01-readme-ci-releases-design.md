# README + CI + Releases Design — db-peek

Date: 2026-10-01
Status: approved (approach A Minimal)
Scope: README.md + MIT LICENSE + CI workflow + GoReleaser + tag release workflow + ldflags version wiring.
Repo: github.com/hamzahraihan/db-peek
Non-goals: Homebrew/Scoop taps, auto-changelog, macOS CI runner, schema-qualified fixes, CLI export flags.

## 1. Intent

Make `db-peek` installable and trustworthy for a new user in <5 min:
`go install` or download a versioned binary for linux/darwin/windows,
run `--help` / `--version`, connect to sqlite/postgres/mysql.
Success = README quickstart works verbatim, `main` CI green on push/PR,
`git tag vX.Y.Z` produces 6 binaries + checksums with correct `--version`.

Assumptions: MIT license; Go 1.26 floor (`go.mod:3` says `go 1.26.0`,
local toolchain `go1.27.1`); entrypoint `main.go:16`; version const
`appVersion = "0.6.0"` at `main.go:34`; TUI keys are source-of-truth in
`internal/tui/whichkey.go:24` (`keyRegistry`); export formats source-of-truth
in `internal/export/export.go:20` (csv/tsv/json/md).

## 2. README.md (root)

Sections:
1. Title + badges: `CI` (ci.yml), `Release` (release.yml), `Go 1.26+`.
   Badges link to `https://github.com/hamzahraihan/db-peek/actions`.
2. What: one paragraph — terminal peek at postgres/mysql/sqlite; TUI + one-shot CLI.
3. Install: `go install github.com/hamzahraihan/db-peek@latest`; release binaries
   (`v*` tag assets `db-peek_<os>_<arch>.zip/tar.gz`); `go build -o db-peek .` for source.
4. Quickstart (verbatim-tested):
   - `db-peek ./ecommerce.db` (sqlite file must exist; `:memory:`, `sqlite://path` also accepted per `internal/db/db.go:58`)
   - `db-peek "postgres://user:pass@localhost:5432/db" --list`
   - `DATABASE_URL=postgres://... db-peek --list`
   - `db-peek [conn] --rows users --limit 20 --offset 0`
   - `db-peek [conn] --schema users`, `--query "SELECT 1"`, `--conns/--save/--forget`
5. Flags table mirroring `flag` defs in `main.go:38-48` (`--db --list --schema --rows --query --limit --offset --save --forget --conns --version`).
   Document: `--query` caps SELECT at 200 rows (`internal/db/db.go:594`); writes print `N rows affected`.
6. TUI keys: condensed table generated from `keyRegistry` — Global (`q`, `tab`, `?`, `esc` cancel),
   Sidebar (`/`, `enter`, `left/right`, `r`, `c`), Detail (`1-5` tabs, `n/p`, `s`, `g/G`, `,/.`, `e`, `E`),
   Query editor (`ctrl+r/F5` run, `ctrl+e/ctrl+y` explain, `ctrl+p/n` history, `ctrl+t/w`, `H/L`, `t/X`),
   Connections (`a/e/d`, `enter`). Full list stays in-app via `?`.
7. Saved connections + session: `~/.config/dbp...` via `saved.DefaultDir` (`internal/saved/saved.go:30`),
   file `0600`, `Mask` for display (`internal/saved/saved.go:179`); history/session restore note.
8. Export: `E` in TUI writes `.csv/.tsv/.json/.md` via `FormatFromExt` (`internal/export/export.go:31`); NULL → empty (JSON null).
9. Dev: `go vet ./...`, `go test ./...`, `go build ./...`; Go 1.26+ required.
10. License: MIT footer + link to `LICENSE`.

Constraint: every command/flag/key in README must match code (no invented flags).

## 3. CI — `.github/workflows/ci.yml`

- Name `CI`; on `push` to `main` + `pull_request`.
- Matrix: `os: [ubuntu-latest, windows-latest]` × `go: ["1.26", "1.27"]`.
  No macOS runner (cost); macOS coverage comes from cross-compile in release job.
- Steps: `actions/checkout@v4`, `actions/setup-go@v5` (`go-version`, `cache: true`),
  `go vet ./...`, `go test ./...`.
- Fail-fast false so windows/ubuntu results both visible.

## 4. Release — `.goreleaser.yaml` + `.github/workflows/release.yml`

`.goreleaser.yaml` (v2 schema):
- `version: 2`; `builds[0]`: `main: .`, `binary: db-peek`, `goos: [linux, windows, darwin]`,
  `goarch: [amd64, arm64]` → 6 assets,
  `ldflags: ["-s -w -X main.appVersion={{.Version}}"]`.
- `archives`: `format_overrides: [{goos: windows, format: zip}]` (else tar.gz), `name_template: "{{ .Binary }}_{{ .Os }}_{{ .Arch }}"`.
- `checksum`: `name_template: "checksums.txt"`.
- No brews/scoops/nfpm in v1.

`release.yml`: on `push.tags: ["v*"]`, `permissions: contents: write`,
`runs-on: ubuntu-latest`, steps checkout (fetch-depth 0), setup-go 1.27,
`goreleaser/goreleaser-action@v6` with `args: release --clean`.
Requires `GORELEASER_TOKEN` default `GITHUB_TOKEN` only.

## 5. Version wiring — `main.go:34`

Keep `const appVersion = "0.6.0"` as dev fallback. GoReleaser `ldflags -X main.appVersion`
requires `var` not `const`, so change to `var appVersion = "0.6.0 // dev"` with comment.
`--version` output `db-peek 0.6.0` unchanged locally; tagged build prints `vX.Y.Z`.
No other code change.

## 6. LICENSE (MIT)

Standard MIT text, copyright `2026 hamzah raihan` (git author + repo owner `hamzahraihan`),
matching `git remote -v` origin `https://github.com/hamzahraihan/db-peek.git`.
Year 2026 from file timestamps.

## 7. Verification

- `go vet ./...` clean; `go test ./...` 4 pkgs ok (`db`, `export`, `saved`, `tui`).
- `go build -o /tmp/db-peek-test . && /tmp/db-peek-test --version` → `db-peek 0.6.0`.
- `goreleaser check` clean (if goreleaser installed; else `go run` skip with note).
- README commands dry-run against `test.db`/`ecommerce.db` where read-only (`--list`, `--rows`, `--schema`);
  no write query in verification.
- CI file validates via `actionlint` if available, else YAML parse.

## 8. Files touched

- ADD `README.md`, `LICENSE`, `.github/workflows/ci.yml`, `.goreleaser.yaml`, `.github/workflows/release.yml`
- EDIT `main.go:34` (`const` → `var` for ldflags)
- ADD this spec `docs/superpowers/specs/2026-10-01-readme-ci-releases-design.md`
