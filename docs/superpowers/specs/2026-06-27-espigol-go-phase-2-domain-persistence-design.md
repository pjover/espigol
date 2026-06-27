# Espígol (Go) — Phase 2: Domain & Persistence — Design

**Status:** Approved for implementation · **Date:** 2026-06-27

Phase 2 of the Espígol Go rewrite. Builds the immutable domain model and the SQLite
persistence layer on top of the Phase 1 foundation. Parent design:
`docs/superpowers/specs/2026-06-26-espigol-go-overview-design.md` (§2–§4). Business rules
are not restated here; they live in `espigol-java/private/espigol-cmd-spec.md` and are
reproduced in later phases.

This phase delivers **no business logic** (allocation, close, reports, UI all come later).
It delivers the data model, the schema, the generated query layer, mappers, repositories,
and a one-off tool to adopt the existing Java SQLite database.

---

## 1. Decisions (resolved during brainstorming)

| Decision | Choice | Rationale |
|---|---|---|
| Money in Go | `shopspring/decimal`, scale 2, HALF_UP | Go's BigDecimal equivalent; no `float64` in the domain. |
| Money in SQLite | **TEXT** (canonical decimal string, e.g. `"31900.00"`) | SQLite has no decimal type; `DECIMAL(15,2)` (NUMERIC affinity) silently stores fractional values as REAL/float. `shopspring/decimal`'s `sql.Valuer`/`Scanner` serialize to a decimal string, so `Money ↔ decimal ↔ TEXT` round-trips exactly. Money is never SUM-ed in SQL (aggregation happens in Go). |
| Schema mechanism | **goose** (embedded, versioned `.sql`) | Flyway equivalent; CGO-free; works with modernc; clean phase-by-phase evolution. |
| sqlc schema source | the **goose migrations** | Single source of truth for the schema. |
| Adopt transform | **standalone `cmd/adopt` tool** | One-off cutover utility; keeps legacy-schema-reading code out of the shipped `espigol` binary. |
| `CPYYnnn` overflow | hundreds digit → letter (`000`–`999`, then `A00`–`Z99` = 1000–3599) | Keeps the 3-char width, never collides with existing decimal ids, human-readable. |

---

## 2. Domain model (`internal/domain/model`)

Immutable Go structs: unexported fields, a validating constructor, and `With…` copy
methods for mutation. Value semantics. **Zero imports from `adapters/`.**

### 2.1 Value types

- **`Money`** — wraps `decimal.Decimal`, normalized to scale 2 / HALF_UP in the
  constructor. Methods mirror the Java record:
  `Plus`, `Minus`, `Times(int)`, `TimesRatio(decimal.Decimal)`, `DividedBy(int)`
  (divisor ≥ 1, HALF_UP scale 2), `Negate`, `Cmp(Money) int`, `IsZero() bool`,
  `Decimal() decimal.Decimal`, `String()` (plain `"31900.00"`; EU formatting `1.234,56 €`
  is a report-layer concern, not here).
  Constructors: `MoneyFromString(string) (Money, error)`, `MoneyOf(int64)`, `ZeroMoney()`.
- **`ExpenseScope`** — kind `COMMON | SECTION | PARTNER` plus an optional `sectionCode`.
  Constructor enforces: `sectionCode` is set **iff** kind == `SECTION`. Catalan display is
  resolved elsewhere from the owning `Section`'s label.
- **Enums as Go types** with `String()` and parse helpers: `ScopeKind`, `WindowState`
  (`DRAFT|OPEN|CLOSED`), `ExpenseCategory` (`CURRENT|INVESTMENT`), `PartnerType`,
  `AuditKind`.

### 2.2 Entities

- **`Partner`** — id (int), name, surname, vatCode, email, mobile, partnerType,
  riaNumber, addedOn (date), boardMember (bool). **No section booleans** — membership is
  relational (§2.3). `WithBoardMember`.
- **`Section`** — code (string PK, e.g. `oliva`), label (Catalan), active (bool),
  displayOrder (int).
- **`PartnerSection`** — partnerID (int), sectionCode (string).
- **`ExpenseForecast`** — id (`CPYYnnn` string), partnerID, concept, description,
  grossAmount (`Money`), approvedAmount (`Money`), approvedOn (time, nullable),
  plannedDate (date), year (int), subtypeCode (string), scope (`ExpenseScope`),
  addedOn (time), enabled (bool). Invariant: `year == plannedDate.Year()`. Copy methods:
  `WithApprovedAmount`, `WithApprovedOn`, `WithEnabled`.
- **`ExpenseType`** — year, code, label (Catalan, verbatim bracketed), category.
  Per-year; composite key `(year, code)`.
- **`ExpenseSubtype`** — year, code, label, typeCode. Per-year; composite key
  `(year, code)`. Opaque codes (the `a2`/`a3` shared-label quirk is preserved).
- **`SubmissionWindow`** — year (PK), state, openedAt (nullable), closedAt (nullable),
  deadline, currentExpenseLimit (`Money`), investmentExpenseLimit (`Money`).
- **`Report`** — id (int), year, generatedAt (time), snapshotJSON (string), pdf
  (`[]byte`), supersededAt (time, nullable).
- **`AuditEvent`** — id (int), actorID (nullable int), actorEmail, kind, entityType,
  entityID, timestamp, payload (string, nullable).
- **`BoardAuthorization`** — partnerID, scopeKind, sectionCode (nullable; required when
  scopeKind == `SECTION`). Which non-`Soci` scopes a board member may edit on the web.

### 2.3 Forecast id generation

A domain helper package `forecastid`:
- `Format(year, seq int) string` → `CP` + 2-digit year + sequence: `seq` in `[0,999]`
  rendered `%03d`; `seq` in `[1000,3599]` rendered as `letter('A'+(m/100)) + %02d(m%100)`
  where `m = seq-1000`; `seq > 3599` is an error.
- `ParseSeq(id string) (year, seq int, err error)` → inverse, so the repository can derive
  the current max sequence for a year and compute the next.

---

## 3. Persistence (`db/`, `internal/adapters/persistence`)

### 3.1 Schema — goose migration `db/migrations/00001_init.sql`

A fresh Go schema (no incremental alters — this DB is created new). Based on espigol-java's
`V2__schema.sql` with the Phase-2 changes baked in:

- `partner` — as Java **minus** `olive_section` / `livestock_section`.
- `section(code TEXT PRIMARY KEY, label TEXT NOT NULL, active INTEGER NOT NULL DEFAULT 1,
  display_order INTEGER NOT NULL)`.
- `partner_section(partner_id INTEGER NOT NULL, section_code TEXT NOT NULL,
  PRIMARY KEY(partner_id, section_code), FK partner_id → partner(id),
  FK section_code → section(code))`.
- `submission_window` — limits `current_expense_limit` / `investment_expense_limit` as
  **TEXT**; `one_open_window` partial unique index on `state='OPEN'` retained.
- `expense_type(year, code, label, category CHECK IN ('CURRENT','INVESTMENT'),
  PRIMARY KEY(year, code), FK year → submission_window(year))`.
- `expense_subtype(year, code, label, type_code, PRIMARY KEY(year, code),
  FK (year, type_code) → expense_type(year, code))`.
- `expense_forecast` — `gross_amount` / `approved_amount` as **TEXT**;
  `scope` replaced by `scope_kind TEXT NOT NULL CHECK IN ('COMMON','SECTION','PARTNER')`
  plus `section_code TEXT` (nullable, FK → section(code)); a table CHECK enforcing
  `section_code` is non-null iff `scope_kind = 'SECTION'`. Retains the year/subtype FKs and
  the `idx_forecast_year_enabled` / `idx_forecast_partner` indexes.
- `report` — as Java (`snapshot_json TEXT`, `pdf BLOB`, `superseded_at`, partial index).
- `audit_event` — as Java.
- `board_authorization(partner_id INTEGER NOT NULL, scope_kind TEXT NOT NULL CHECK IN
  ('COMMON','SECTION'), section_code TEXT, FK partner_id → partner(id), FK section_code →
  section(code))`. Only non-`Soci` scopes are stored (`PARTNER`/own-forecasts is implicit).
  `section_code` is non-null iff `scope_kind = 'SECTION'` (table CHECK). Because SQLite
  treats NULLs as distinct in a PRIMARY KEY, uniqueness is enforced with a unique
  expression index on `(partner_id, scope_kind, COALESCE(section_code, ''))` rather than a
  composite PK.

Migrations are `go:embed`-ed and applied idempotently (`goose.Up`) when the app opens the
DB, and runnable standalone via a `make` target. Every connection sets WAL mode,
`busy_timeout`, and `PRAGMA foreign_keys=ON`.

### 3.2 sqlc

- `sqlc.yaml`: schema = `db/migrations/`, queries = `db/queries/*.sql`, engine sqlite,
  `database/sql` output compatible with the modernc driver.
- Generated package `internal/adapters/persistence/sqlc` — typed row structs (DBOs) +
  query methods. TEXT money columns scan as `string`; nullable columns as
  `sql.Null*`/pointers. The domain never sees these types.

### 3.3 Mappers & repositories

- `mapper` translates sqlc rows ↔ domain structs at the boundary:
  `string ↔ Money`, `(scope_kind, section_code) ↔ ExpenseScope`, `int(0/1) ↔ bool`,
  RFC3339 `TEXT ↔ time.Time`, nullable handling.
- One repository per aggregate, implementing the matching `internal/domain/ports`
  interface: `PartnerRepository`, `SectionRepository` (incl. partner-section membership),
  `TaxonomyRepository` (types + subtypes), `WindowRepository`, `ForecastRepository`,
  `ReportRepository`, `AuditLog`, `BoardAuthorizationRepository`, plus a `Clock` adapter.
  Each implements the CRUD/query methods needed now; later phases add queries.
- `ForecastRepository` owns **next-id allocation**: within the insert transaction it finds
  the current max sequence for the year (via `forecastid.ParseSeq` over existing ids) and
  formats the next id with `forecastid.Format`, avoiding races.
- An `internal/adapters/persistence/db` (or `internal/db`) package opens the connection
  (modernc, WAL, pragmas), runs goose, and provides `*sql.DB` to repositories.

### 3.4 Ports

Port interfaces live in `internal/domain/ports`; repositories satisfy them so
`domain/services` (later phases) depend only on interfaces, never on sqlc or `database/sql`.

---

## 4. Adopt tool (`cmd/adopt`)

A standalone one-off binary, separate from `espigol`, run once at cutover. It contains its
own thin reader for the **old Java schema** and writes through the **new** persistence
layer. After cutover it can be retired; in Phase 2 it is built and tested against the real
Java DB.

**Invocation:** `adopt --from <legacy.db> --to <espigol.db>` (`--to` defaults to
`$ESPIGOL_HOME/espigol.db`). Refuses to run if `--to` exists unless `--force` is given.

**Steps (single transaction; rollback on any error):**

1. Create the destination DB; run `goose.Up` to build the new schema.
2. Open the legacy DB read-only.
3. Seed `section` rows `oliva` ("Secció d'oliva", order 1) and `ramaderia`
   ("Secció de ramaderia", order 2), `active=true`.
4. Partners: copy each; convert `olive_section`/`livestock_section` → `partner_section`
   rows; carry `board_member`.
5. Taxonomy + windows: copy `expense_type`, `expense_subtype`, `submission_window`; limits
   NUMERIC → canonical TEXT money.
6. Forecasts: copy each; money NUMERIC (int or REAL) → exact TEXT scale-2 (asserting no
   precision loss); `scope` Catalan string → `(scope_kind, section_code)`
   (`Comú`→COMMON, `Soci`→PARTNER, `Secció d'oliva`→SECTION+`oliva`,
   `Secció de ramaderia`→SECTION+`ramaderia`); `CPYYnnn` ids kept verbatim.
7. Reports + audit: copy `report` (snapshot_json + pdf BLOB) and `audit_event` rows.
8. Write one `AuditEvent` of kind `MIGRATION` recording per-table counts.

**Validation:** after load, verify per-table row-count parity with the source; fail loudly
on mismatch. **Not idempotent** (re-running needs a fresh `--to`) — documented in `--help`.
This is the only place float→exact conversion happens; the running app sees TEXT money only.

---

## 5. Testing (TDD)

- **Domain unit tests** (pure): `Money` arithmetic/rounding/compare; `ExpenseScope`
  invariant; `ExpenseForecast` year/plannedDate invariant and copy methods; `forecastid`
  Format/ParseSeq round-trip including the `999→A00→Z99` overflow and out-of-range error.
- **Persistence integration tests** (real SQLite temp file via `t.TempDir()`, goose-
  migrated, modernc): each repository round-trips domain → DB → domain exactly —
  especially `Money` TEXT (`"1322.22"`), scope kind+section, per-year composite-key
  taxonomy, and `ForecastRepository` next-id allocation (insert into 2026 → `CP26036`).
- **Adopt-tool test** (high-value): run `adopt` against a copy of the frozen real Java DB
  and assert: 8 partners with correct `partner_section` rows; 2 sections; 35 forecasts with
  exact TEXT money (incl. former-REAL `1322.22` / `638.74`); correct scope_kind/section_code
  mapping; 1 report (pdf + json intact); 61 audit events + 1 new MIGRATION event; per-table
  row-count parity.

**Test fixture:** the adopt test needs a copy of the real Java DB
(`espigol-java/.local/espigol.db`). The Phase-2 plan copies a frozen snapshot into this
repo's gitignored `testdata/` at implementation time; the test skips gracefully if the
fixture is absent (mirroring how espigol-java guards on `private/`), so CI stays green.

---

## 6. Scope

**In Phase 2:** the complete domain model; the goose schema + sqlc setup; mappers; all
repositories (CRUD/queries needed now); the `forecastid` helper; the DB open/migrate
plumbing; the `cmd/adopt` tool — all tested.

**Not in Phase 2:** allocation algorithm (Phase 3); window-close orchestration (Phase 4);
report rendering (Phase 5); server/auth (Phase 6); TUI (Phase 7). Repositories expose the
methods those phases will call but contain no business logic.

---

## 7. File structure (new in Phase 2)

```
db/
├── migrations/00001_init.sql        # goose schema (sqlc reads this)
└── queries/*.sql                    # sqlc query sources
sqlc.yaml
internal/
├── domain/
│   ├── model/                       # Money, Partner, Section, PartnerSection,
│   │                                #   ExpenseForecast, ExpenseScope, ExpenseType,
│   │                                #   ExpenseSubtype, SubmissionWindow, Report,
│   │                                #   AuditEvent, BoardAuthorization, enums
│   ├── forecastid/                  # CPYYnnn Format / ParseSeq
│   └── ports/                       # repository + Clock interfaces
└── adapters/persistence/
    ├── db/                          # open conn (modernc, WAL, pragmas) + goose runner
    ├── sqlc/                        # generated DBOs + query methods
    ├── mapper/                      # sqlc rows <-> domain
    └── *_repository.go              # repositories implementing ports
cmd/adopt/main.go                    # one-off legacy-DB adopt tool
testdata/                            # frozen legacy DB snapshot (gitignored)
```

---

## 8. References

- Overview design: `docs/superpowers/specs/2026-06-26-espigol-go-overview-design.md`.
- Java schema: `espigol-java/src/main/resources/db/migration/V2__schema.sql`.
- Java domain records: `espigol-java/src/main/java/coop/estellencs/espigol/domain/model/`.
- Real inherited DB (adopt source / fixture): `espigol-java/.local/espigol.db`.
- Domain rules (later phases): `espigol-java/private/espigol-cmd-spec.md`.
