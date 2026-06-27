# Espígol (Go) — Overview Design

**Status:** Approved for phased implementation · **Date:** 2026-06-26

This is the high-level design for the **final stage** of the Espígol project: a single Go
application that replaces both the original Go `espigol-cmd` and the Java `espigol-java`
migration. It manages the annual subsidy budget of the Cooperativa d'Estellencs.

This document is authoritative for *how the system is structured and what each phase
delivers*. It does **not** restate the business rules (allocation algorithm, section
warning, scope waterfall, Catalan labels, currency formatting); those live in
`espigol-java`'s `private/espigol-cmd-spec.md` and must be reproduced faithfully. Where
this design deliberately diverges from `espigol-java`, it says so explicitly.

---

## 1. Purpose & lineage

Espígol has gone through three stages:

1. **`espigol-cmd`** — first version: Go, command-based, MongoDB, PDF generation. Its
   PDF **and Markdown reports are more refined** than the Java ones and are the layout
   reference for this stage.
2. **`espigol-java`** — Spring Boot migration. Its **data structures, hexagonal design,
   per-year taxonomy, immutable domain, allocation algorithm, and SQLite schema** are the
   logical reference for this stage.
3. **This stage (Go, final)** — one Go executable with two run modes:
   - `espigol` → **TUI** (terminal UI), admin-only, used only by the administrator.
   - `espigol --server` → **HTTP server**, accessible only to Google-authenticated socis.

The guiding principle: **reuse espigol-cmd's report layouts, on top of espigol-java's
domain model and logic, rebuilt natively in Go.**

---

## 2. Architecture

### 2.1 One module, one binary, two run modes

A single Go module in the `espigol` repository produces one executable. `cmd/espigol`
parses flags and dispatches:

- no flag → launches the Bubble Tea TUI
- `--server` → launches the HTTP server

Both modes share the same domain, persistence, and report code. They differ only in their
*driving adapter* (TUI vs HTTP).

### 2.2 Hexagonal layout (golang-standards/project-layout)

```
espigol/
├── cmd/espigol/main.go            # entrypoint: flag parsing → TUI or server
├── internal/
│   ├── domain/
│   │   ├── model/                 # immutable structs: Partner, ExpenseForecast,
│   │   │                          #   ExpenseType/Subtype, Section, PartnerSection,
│   │   │                          #   SubmissionWindow, Report, AuditEvent,
│   │   │                          #   BoardAuthorization, Money, scope kinds
│   │   ├── ports/                 # interfaces: repositories, Clock, ReportRenderer, AuditLog
│   │   └── services/              # allocation (FairShareAllocator), window-close orchestration
│   ├── adapters/
│   │   ├── persistence/           # SQLite: sqlc-generated queries (DBOs) + mappers
│   │   ├── tui/                   # Bubble Tea program, panels, keymaps, styles
│   │   ├── web/                   # HTTP server, handlers, html/template views, auth
│   │   └── report/                # maroto PDF + Markdown renderers (ReportRenderer impl)
│   ├── config/                    # ~/.config/espigol resolution, $ESPIGOL_HOME override
│   └── wire/                      # dependency injection wiring
├── db/
│   ├── schema.sql                 # sqlc schema
│   └── queries/*.sql              # sqlc query sources
├── sqlc.yaml
├── Makefile
├── go.mod
└── README.md
```

### 2.3 Immutability & boundaries

- Domain types are plain Go structs with unexported fields and value semantics —
  constructed via constructors, "changed" via copy methods (the Go equivalent of Java
  records). No setters.
- Mutability is confined to the sqlc-generated row structs inside `persistence/`,
  translated to/from domain types by mappers at the port boundary.
- **The domain imports nothing from `adapters/`.** No SQL, no HTTP, no TUI types leak in.

### 2.4 Cross-cutting decisions

| Concern | Decision |
|---|---|
| TUI framework | **Bubble Tea** + Lip Gloss. (Not gocui — lazygit parity is visual/UX, rebuilt, not a code port.) |
| SQLite driver | **`modernc.org/sqlite`** — pure Go, no CGO → single static cross-compilable binary. |
| Query layer | **sqlc** — hand-written SQL → type-safe generated Go (acts as DBOs). |
| Money | Value type backed by **`shopspring/decimal`**, scale 2, HALF_UP. **No `float64` in the domain.** The spec's `< 0.001` guard → scale-2 `compareTo`. |
| Dates/times | `time.Time`; a `Clock` port is injected for tests. |
| Config | **viper**, `config.yaml` under `$ESPIGOL_HOME`, env vars override. |
| Web | Go stdlib `net/http` (1.22+ routing) + `html/template`. No SPA, no JS framework. |
| PDF/MD | **maroto** for PDF (as in espigol-cmd) + a Markdown renderer, behind a `ReportRenderer` port. |

---

## 3. Domain model

Ported from espigol-java's records (the authoritative, cleaned-up versions), with the
**sections model made data-driven** (see §3.2).

### 3.1 Core entities

- **`Partner`** — id, name, surname, vatCode, email, mobile, partnerType, riaNumber,
  addedOn, `boardMember`. Section membership is **no longer** boolean columns (see §3.2).
- **`ExpenseForecast`** — id, partnerId, concept, description, grossAmount (`Money`),
  approvedAmount (`Money`), approvedOn, plannedDate, year, `subtypeCode`, `scopeKind`,
  `sectionCode` (nullable), addedOn, enabled. No attachments (deferred, as in Java v1). The id is string, generated following this format "CPYYnnn" where `YY` are the last two digits of the `year` field, and `nnn` is a sequential digit for that year, if we reach 999 the we will start adding letters.
- **`ExpenseType` / `ExpenseSubtype`** — **per-year** tables, composite key `(year, code)`.
  Opaque codes (the `a2`/`a3` quirk preserved), editable Catalan labels. Verbatim
  bracketed codes (`[a]`, `[a1]`…). Copied from the previous year on new-year creation;
  locked when the window moves to `OPEN`.
- **`SubmissionWindow`** — `year` PK; state `DRAFT → OPEN → CLOSED`; openedAt, closedAt,
  deadline, currentExpenseLimit, investmentExpenseLimit. **At most one `OPEN` at a time.**
- **`Report`** — id, year, generatedAt, snapshotJSON, pdf BLOB, supersededAt.
- **`AuditEvent`** — actorId, actorEmail, kind, entityType, entityId, timestamp, payload.

### 3.2 Sections (data-driven — divergence from espigol-java)

espigol-java hardcodes two sections (olive, livestock) as a fixed enum, with
`oliveSection`/`livestockSection` booleans on `Partner`. Because the cooperative expects
sections to grow (e.g. adding `vineyard`), this stage makes sections **data**:

- **`Section`** — global entity: `code` (e.g. `oliva`, `ramaderia`, `vinya`), `label`
  (Catalan, e.g. "Secció d'oliva"), `active`, `displayOrder`. Adding a section is a TUI
  data entry — **no code change**.
- **`PartnerSection`** — partner↔section membership (many-to-many), replacing the two
  booleans.
- **`ExpenseScope`** is a **kind**: `COMMON` | `SECTION` | `PARTNER`. When the kind is
  `SECTION`, the forecast's `sectionCode` (FK → `Section`) names which one. Catalan display
  derives from the section's label; `COMMON` shows "Comú", `PARTNER` shows "Soci".
- **`BoardAuthorization`** — `(partnerId, scopeKind, sectionCode?)`: which non-`Soci`
  scopes a board member may edit **on the web**. A regular soci implicitly manages their
  own `PARTNER`-scope forecasts (not stored).

### 3.3 Immutability & storage

Domain types are immutable; storage uses `UPDATE`s on edits (not event sourcing). The
`AuditEvent` table captures actor + before/after. Mutability is contained inside the
persistence adapter.

---

## 4. Persistence

### 4.1 Schema

`db/schema.sql` is based on espigol-java's `V2__schema.sql` (partner, submission_window,
expense_type, expense_subtype, expense_forecast, report, audit_event, including the
`one_open_window` partial unique index and per-connection `PRAGMA foreign_keys=ON`), with
these **changes for the data-driven sections model**:

- New `section(code, label, active, display_order)` table.
- New `partner_section(partner_id, section_code)` join table; the `olive_section` /
  `livestock_section` columns on `partner` are removed.
- `expense_forecast.scope` becomes `scope_kind` (`COMMON`/`SECTION`/`PARTNER`) plus a
  nullable `section_code` (FK → `section`), replacing the 4-value Catalan-string enum.
- New `board_authorization(partner_id, scope_kind, section_code)` table.
- Optional `session` table for server-side web sessions.

### 4.2 Adopting the inherited database

The Go app **inherits the SQLite database produced by the espigol-java Mongo→SQLite
migration** — there is no Mongo code in Go. Because of the §4.1 schema changes, adoption
is not verbatim; a one-time transform (part of the Foundation phase) does:

1. Create `section` rows for `oliva` and `ramaderia` (labels "Secció d'oliva",
   "Secció de ramaderia").
2. Convert each partner's `olive_section` / `livestock_section` booleans into
   `partner_section` rows.
3. Map each forecast's scope string → `(scope_kind, section_code)`:
   `Comú`→`COMMON`, `Soci`→`PARTNER`, `Secció d'oliva`→`SECTION`+`oliva`,
   `Secció de ramaderia`→`SECTION`+`ramaderia`.

### 4.3 Access model

- **`modernc.org/sqlite`** (pure Go), opened in **WAL mode with `busy_timeout`**.
- The TUI and the server both open the SQLite file **directly** (no TUI-calls-server
  indirection), so the TUI works whether or not the server is running.
- **Operational model:** the authoritative `espigol.db` lives on the VPS. The
  administrator SSHes into the VPS and runs the TUI there, against the **same** file the
  server uses. WAL + `busy_timeout` make the rare concurrent write between the
  long-running server and an occasional SSH TUI session safe. No syncing, no second copy.

---

## 5. Allocation & window lifecycle

### 5.1 Window state machine

`DRAFT → OPEN → CLOSED`. Taxonomy and limits are editable in `DRAFT`, locked on `OPEN`.
At most one `OPEN` window at a time; multiple `DRAFT`/`CLOSED` coexist. **Closing is
manual** (done by the admin in the TUI). There is no auto-close scheduler in v1.

### 5.2 Allocation algorithm

Port espigol-java's **`FairShareAllocator`** (authoritative per spec §8.5), **not**
espigol-cmd's float-based `distributeRemainder`:

- `Money`/decimal semantics throughout; the `< 0.001` guard becomes a scale-2 compare.
- Non-positive section pools are handled by capping all partners at the mean (which may be
  ≤ 0) — matching the reference; clamping to 0 would be a behavior change.
- **Generalized to N sections.** The scope waterfall stays Common → Sections → Socis, but
  the sections step sums over all active `Section`s, and the section-warning splits
  `availableForSections` proportionally by producer-count per section, iterating every
  active section instead of naming olive/livestock.

### 5.3 Closing a window

In one transaction: run allocation over enabled forecasts for the year → compute the
`ReportData` snapshot → persist `approvedAmount`/`approvedOn` on every `PARTNER`-scope
forecast → insert a `Report` row (JSON snapshot + PDF BLOB) → flip state to `CLOSED` →
write a `WINDOW_CLOSED` audit event.

### 5.4 Amending a closed year

The admin edits forecasts in a `CLOSED` window in the TUI (no reopen state). Regenerating
marks the current `Report` `supersededAt = now` and inserts a new one; socis always see
the latest non-superseded report.

---

## 6. Reports

Reports are **generated only in the TUI**. Two artifacts per generation, both reusing
espigol-cmd's refined layouts, rebuilt on the Java-style domain:

- **PDF** via **maroto** — sub-report structure lifted from espigol-cmd's
  `expense_forecast_report_service.go` (common table, sections table, remainder summary,
  section warning, partner allocations + adjustment, per-scope detail, per-partner detail,
  `CPyyNNN` codes, red highlighting for capped amounts), with `Money`/decimal,
  `FairShareAllocator`, and generic sections.
- **Markdown** — the espigol-cmd MD report, same data and reconciliation.

### 6.1 Single-snapshot data flow

Closing or regenerating runs allocation **once**, producing a `ReportData` snapshot
(ported from espigol-java's `domain/model/report/` structs). That snapshot is:

1. serialized to JSON and stored on the `Report` row — the source of truth for the web
   HTML view (deterministic, testable);
2. rendered to PDF (stored as the `Report.pdf` BLOB **and** written to the output dir);
3. rendered to Markdown (written to the output dir).

PDF, MD, and HTML all derive from one computed snapshot — no recomputation, no drift. The
`ReportRenderer` port abstracts PDF and MD; the web reads the stored JSON directly.

### 6.2 Testing

The snapshot is validated against golden values from
`private/report-examples/Previsions de despeses 2026.md` (the espigol-java golden-file
strategy), giving a byte-level guarantee the ported allocation reproduces the reference.
PDF/MD get smoke tests (render without error; expected sections present).

---

## 7. TUI (admin)

A Bubble Tea + Lip Gloss application reproducing **lazygit's UX feel** (visual/interaction
parity, rebuilt — not a code port of lazygit's gocui internals).

- **Layout** — lazygit-style: a left column of stacked navigable panels (the entities) and
  a main panel showing the selected item's detail/list. A bottom line shows
  context-sensitive keybindings; a top bar shows the current **year context**.
- **Left-column panels** — Anys (windows), Socis (partners), Seccions, Tipus i subtipus
  (per-year taxonomy), Previsions (forecasts for the selected year, all partners),
  Informes (reports).
- **State-as-color** (lazygit convention) — `DRAFT` grey/yellow, `OPEN` green, `CLOSED`
  blue; disabled forecasts dimmed; capped/over-budget amounts red.
- **Keymaps** — lazygit-simple: `↑/↓`/`j`/`k` navigate, `Tab`/arrows switch panels,
  `Enter` drills in, single-letter actions (`n` new, `e` edit, `d` delete, `o` open
  window, `c` close window, `r` generate report, `?` help, `q` quit), confirmation modal
  for destructive/irreversible actions.

**Capabilities** (everything not on the web):

- CRUD partners (and partner types) and their section memberships.
- CRUD **sections** (add `vinya`, etc.).
- CRUD per-year taxonomy (types/subtypes) while a window is `DRAFT`.
- Manage **board authorizations** (which scopes/sections each board member may edit on
  the web).
- Create/edit a `SubmissionWindow`, set limits + deadline, **open** and **close** it.
- Create/edit forecasts **impersonating any partner** (incl. `COMMON`/`SECTION` scopes);
  each write logs an `AuditEvent` with the admin as `actorEmail`.
- Generate **PDF + Markdown** reports for a year.

**No auth in the TUI** — it runs locally (over SSH on the VPS); the actor is recorded as
the administrator in the audit log.

The TUI calls the same domain services as the server; it is just a different driving
adapter.

---

## 8. Server (socis web)

The only externally reachable surface, drastically slimmer than the Java app. Everything
not needed by the socis frontend is removed (no REST CRUD API, no Swagger, no admin
endpoints).

- **Stack** — Go stdlib `net/http` (1.22+ routing) + `html/template`, server-rendered.
- **Auth** — Google OAuth2 (`golang.org/x/oauth2/google`). Identity = Google email. On
  callback, resolve email → `Partner`:
  - no match → Catalan error ("aquest correu no està registrat com a soci; contacta amb
    el Consell Rector"), not logged in;
  - match → server-side session (HttpOnly/Secure/SameSite=Lax cookie; session store in
    SQLite).
  - A **dev login bypass** (type any email) when running locally without Google
    credentials.

### 8.1 Routes (the entire surface)

| Route | Purpose |
|---|---|
| `GET /` | dashboard: current `OPEN` year, deadline, my forecasts, links to past reports |
| `GET /health` | liveness |
| `GET /login`, `GET /oauth2/callback`, `POST /logout` | auth |
| `GET /forecasts/new`, `POST /forecasts` | create forecast (while window `OPEN`) |
| `GET /forecasts/{id}/edit`, `POST /forecasts/{id}` | edit (while `OPEN`) |
| `POST /forecasts/{id}/delete` | delete (while `OPEN`) |
| `GET /reports/{year}` | read-only HTML report (closed years; from the stored snapshot) |

### 8.2 Authorization

- **Regular soci** — create/edit/delete only their own `PARTNER`-scope forecasts, only
  while the relevant year's window is `OPEN`. After close, sees their own approved amounts
  plus the read-only report.
- **Board member** (`boardMember = true`) — own forecasts **plus** `COMMON`/`SECTION`
  forecasts only for the scopes listed in their `BoardAuthorization`, while `OPEN`. The
  web scope selector shows only their authorized scopes.
- Everything else (other socis' forecasts, partners, sections, taxonomy, board
  authorizations, windows) is **TUI-only**.

Each web mutation writes an `AuditEvent`.

### 8.3 No background work in v1

The server is purely request/response. **No scheduler and no email** in v1 (auto-close
and notifications are deferred to a possible later phase). Window closing is done by the
admin in the TUI.

---

## 9. Configuration, build & deployment

### 9.1 Configuration & data location

- Default home: `~/.config/espigol/`, overridable via **`$ESPIGOL_HOME`**, resolved once
  at startup in `internal/config`.
- Contents: `espigol.db` (SQLite), `config.yaml`, `logo.png`, `reports/` (output dir).
- `config.yaml`: business name, server port, Google OAuth client id/secret (or env vars,
  env winning), output dir, backup dir, logo path. **Limits live in the DB** (`submission_window`),
  not config.

### 9.2 Build

- Single Go module; `make build` → one static binary `bin/espigol` (CGO-free,
  cross-compilable).
- Make targets adapted from espigol-cmd/espigol-java: `build`, `run`, `test`, `fmt`,
  `tidy`, `sqlc-generate`, `tui`, `server`.

### 9.3 Deployment

- **TUI** — run over SSH on the VPS, against the authoritative `espigol.db`.
- **Server** — `espigol --server` on a small VPS, managed by `systemd`, behind **Caddy**
  for TLS (auto Let's Encrypt), reverse-proxying `localhost:<port>`. Config/secrets via
  `EnvironmentFile`.
- **Backups** — nightly `sqlite3 .backup` + gzip + off-box copy.
- **No container in v1** — the binary is the artifact (`scp` + `systemctl restart`).

---

## 10. Phase roadmap

Each phase is its own spec → plan → implementation, committed step-by-step, with **one PR
per phase**. Phases 1–5 are pure backend/domain (no UI), de-risking the hardest logic
first; the server and TUI are then thin driving adapters over a proven core.

1. **Foundation** — Go module, project layout, `cmd/espigol` entrypoint with `--server`
   flag dispatch, config (`$ESPIGOL_HOME`, viper), Makefile, CI. Stub TUI + server that
   start and exit cleanly.
2. **Domain & persistence** — immutable domain model (incl. `Section`, `PartnerSection`,
   scope kinds, `BoardAuthorization`, `Money`), `db/schema.sql`, sqlc setup, queries,
   mappers, repositories implementing ports, and the adopt-the-Java-DB transform.
3. **Allocation algorithm** — port `FairShareAllocator` + section warning, generalized to
   N sections, `Money`/decimal semantics, `< 0.001` guard; validated against golden values.
4. **Window close & report snapshot** — window state machine, manual open/close
   orchestration, `ReportData` snapshot + JSON serialization, audit events.
5. **Reports** — maroto PDF + Markdown renderers behind `ReportRenderer`, reusing
   espigol-cmd layouts on the new snapshot; output to `reports/` + PDF BLOB.
6. **Server** — `net/http` + `html/template`, Google OAuth + sessions, dev login bypass,
   soci forecast CRUD, board-member scope authorization, read-only HTML report view.
7. **TUI** — Bubble Tea + Lip Gloss, lazygit-style panels/keymaps/state-colors; full
   admin: partners, sections + memberships, per-year taxonomy, board authorizations,
   windows (open/close), impersonated forecasts with audit, report generation.
8. **Deployment** — systemd unit, Caddy config, backup cron, README/ops docs.

---

## 11. Out of scope for v1

- Auto-close-on-deadline scheduler and email notifications (deferred; possible later phase).
- File attachments on forecasts.
- Income tracking; reconciliation against actuals.
- Public REST API; Swagger.
- Multilingual UI (Catalan only).
- Containerized deployment.
- Mongo code in the Go app (migration stays solved in espigol-java).

---

## 12. Non-negotiable domain conventions (carried from espigol-java)

- **All user-facing text is Catalan** — UI, errors, PDF/MD content, button text.
- **The board is "Consell Rector"** in every Catalan string (never "junta"). The
  code-level flag may be `boardMember`.
- **Expense-type codes `[a]`…`[c2]` appear verbatim**, brackets included.
- **Subtype codes are opaque** — `(year, code)` composite key; labels editable per year,
  codes not.
- **`ExpenseType`/`ExpenseSubtype` are per-year tables**, copied on new-year creation,
  locked on `OPEN`.
- **Currency:** `1.234,56 €` — period thousands, comma decimal, symbol after with space,
  always two decimals. `decimal`, scale 2, HALF_UP. No `float64` in the domain.
- **Scope waterfall:** Common → Sections → Socis; non-positive pools cap at mean.

---

## 13. References

- `espigol-java/private/espigol-cmd-spec.md` — authoritative domain rules.
- `espigol-java/private/db-dump/`, `private/report-examples/` — fixtures and golden files.
- `espigol-java/docs/superpowers/specs/2026-05-22-new-espigol-overview-design.md` — the
  Java-stage design this builds on.
- `espigol-cmd/internal/domain/services/reports/` — the refined PDF/MD report layouts.
- lazygit — TUI UX reference.
