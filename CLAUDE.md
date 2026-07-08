# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Espígol is a single Go binary that manages the annual subsidy budget of the Cooperativa
d'Estellencs, a small agricultural cooperative on Mallorca. One executable, two run modes:

- `espigol` (no flag) — the admin **TUI** (Bubble Tea), used only by the administrator,
  typically over SSH on the VPS against the live database.
- `espigol --server` — the **HTTP server** socis (members) use, behind Google OAuth.

Both modes share the same domain, persistence, and report code and differ only in their
driving adapter. Full design rationale lives in
`docs/superpowers/specs/2026-06-26-espigol-go-overview-design.md` — read it before making
structural changes; it documents *why* things are shaped this way (e.g. why sections are
data-driven rather than a hardcoded enum, why there's no window "reopen" state). Each
subsequent phase has its own spec (`docs/superpowers/specs/`) and plan
(`docs/superpowers/plans/`) — check those before assuming a design decision is undocumented.

`README.md` documents the admin panel's import/backup/restore/report-generation
keys and `docs/ops/DEPLOY.md` documents VPS provisioning end-to-end.

## Commands

```bash
make build          # go fmt + build -> bin/espigol
make tui             # build, then run the TUI
make server          # build, then run --server
make test            # go test ./...
make vet             # go vet ./...
make tidy            # go mod tidy
make sqlc-generate    # regenerate internal/adapters/persistence/sqlc from db/queries + db/migrations
make dist            # cross-compile linux-amd64, linux-arm64, darwin-arm64 into dist/<os>-<arch>/espigol
make adopt           # build cmd/adopt, the one-time Mongo(via espigol-java)->this-schema migration tool
```

Single test: `go test ./internal/domain/services/... -run TestFairShare -v` (standard Go
test filtering; substitute the package and test name).

CI (`.github/workflows/ci.yml`) runs `go mod tidy` (diff must be clean), `go vet`,
`go build ./...`, `go test ./...`, then a separate job cross-compiles all `make dist`
targets and validates the deployment artifacts (shellcheck on the backup script,
`systemd-analyze verify` on the units, `caddy validate` on the Caddyfile).

## Architecture

Hexagonal layout (golang-standards/project-layout). The dependency rule is strict: **the
domain imports nothing from `adapters/`** — no SQL, HTTP, or TUI types leak into
`internal/domain`.

```
cmd/espigol/main.go        entrypoint -> internal/app (flag parse: TUI vs --server vs --version)
cmd/adopt/                 one-time legacy-DB adoption tool (legacy/ read, transform/ convert)
internal/domain/
  model/                   immutable structs (Partner, ExpenseForecast, Section, Money, ...)
  ports/                   repository + Clock + ReportRenderer interfaces the domain depends on
  services/                pure algorithms: FairShareAllocator (allocation.go, fairshare.go)
internal/application/      orchestration services (ForecastService, WindowService, ...) that
                            compose domain services + ports; this is what wire.go wires into
                            both the TUI and the server
internal/adapters/
  persistence/             sqlc-generated queries (sqlc/) + hand-written mappers + repositories
  tui/                     Bubble Tea program: panels, keymaps, styles
  web/                     net/http handlers, html/template views
  auth/                    Google OAuth2 + session store (server-side, SQLite-backed)
  report/                  maroto PDF + Markdown renderers implementing ports.ReportRenderer
  importer/                forecast JSON import (admin panel "i" key)
  system/                  SystemClock (real time.Now, injected via ports.Clock)
internal/wire/              dependency injection: wire.Server(cfg) and wire.TUI(cfg) assemble
                            every concrete adapter into the application services
internal/config/            $ESPIGOL_HOME resolution + viper config.yaml loading
db/migrations/              goose SQL migrations (schema source of truth), applied automatically on Open
db/queries/*.sql            sqlc query sources -> generated into internal/adapters/persistence/sqlc
```

Domain types are immutable — plain structs with unexported fields, built via constructors,
"changed" via copy methods (no setters). Mutability is confined to the sqlc-generated row
structs inside `persistence/`; mappers translate at the port boundary.

### Key conventions worth knowing before touching code

- **Money** (`internal/domain/model/money.go`) wraps `shopspring/decimal`, always scale 2,
  HALF_UP. **Never use `float64` for money anywhere in the domain.**
- **Forecast ids** are `CPYYnnn` (`internal/domain/forecastid/`): decimal 000–999, then a
  letter-block scheme (`A00`…`Z99`) for 1000–3599, generated per-year by the repository.
- **Sections are data, not an enum** — `Section`/`PartnerSection` are DB rows; adding a
  section (e.g. a new crop) is a TUI data-entry operation, not a code change. Don't
  reintroduce hardcoded olive/livestock branching.
- **`ExpenseType`/`ExpenseSubtype` are per-year tables** (composite key `(year, code)`),
  copied forward on new-year creation, locked once the window state is `OPEN`.
- **Window lifecycle** is `DRAFT -> OPEN -> CLOSED`, one `OPEN` window at a time (enforced by
  a partial unique index), closing is a manual admin TUI action — there is no reopen state
  or auto-close scheduler.
- **Report generation is TUI-only.** Closing/regenerating computes one `ReportData`
  snapshot, serialized to JSON on the `Report` row; PDF (maroto), Markdown, and the web's
  read-only HTML view all render from that single stored snapshot — never recomputed
  separately, to avoid drift between formats.
- **All user-facing text is Catalan** (UI strings, errors, PDF/MD/HTML content). The board
  is always "Consell Rector" in Catalan strings (the Go-side flag is `boardMember`).
- Currency formatting: `1.234,56 €` (period thousands, comma decimal, symbol after with a
  space, always two decimals).
- The allocation algorithm (`internal/domain/services/fairshare.go`,
  `allocation.go`) is validated against golden fixtures in
  `internal/domain/services/golden_test.go` and `private/report-examples/` — treat any
  numeric drift in its test output as a correctness bug, not a fixture-update situation,
  unless you've confirmed the business rule actually changed.

### Config & data location

`~/.config/espigol/` by default, overridable with `$ESPIGOL_HOME`. Contains `espigol.db`
(SQLite via `modernc.org/sqlite`, pure Go/no CGO, opened WAL + busy_timeout so the TUI and
server can safely touch the same file concurrently), `config.yaml`, `logo.png`, `reports/`,
`backups/`, `import/`. Config keys can be overridden by `ESPIGOL_<KEY>` env vars (viper);
submission-window limits live in the DB, not config.

### `private/`

A symlink into a Dropbox folder holding real cooperative data (past forecasts, report
PDFs, golden-file report examples) — not part of the repo, gitignored. Referenced by specs
and the golden test as fixtures; don't assume its contents are reproducible from git alone.
