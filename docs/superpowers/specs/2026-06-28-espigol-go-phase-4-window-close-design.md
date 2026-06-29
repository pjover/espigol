# Espígol (Go) — Phase 4: Window Close & Report Snapshot — Design

**Status:** Approved for implementation · **Date:** 2026-06-28

Phase 4 of the Espígol Go rewrite. The application/orchestration layer that drives the
window lifecycle and ties Phase-2 persistence to the Phase-3 allocation algorithm. Parent:
`docs/superpowers/specs/2026-06-26-espigol-go-overview-design.md` (§4, §5.3, §6). Phases
1–3 are merged.

Authoritative reference: `espigol-java` `application/WindowClosingService.java` +
`ReportSnapshotSerializer.java` (which implemented `close` only; create/open/amend come from
overview §6).

---

## 1. Decisions (resolved during brainstorming)

| Decision | Choice |
|---|---|
| Phase-4 scope | **Full window lifecycle**: `CreateYear`, `Open`, `Close`, `Amend`. |
| ReportData → JSON | **`MarshalJSON`/`UnmarshalJSON` on `model.Money`** (canonical decimal string); snapshot = `json.Marshal(reportData)`. |
| `report.pdf` before Phase 5 | **`ReportRenderer` port + a no-op renderer** returning `[]byte{}`; Phase 5 swaps in the real maroto/MD renderer. Empty blob satisfies `BLOB NOT NULL` — no migration. |
| Atomic close transactions | **`TxManager` port** — `WithinTx(ctx, func(RepoSet) error)`; every op runs in one tx-scoped closure. |

---

## 2. Architecture

A new application layer orchestrating persistence + allocation, depending only on ports
(no `database/sql` in the app layer; fully testable).

```
internal/
├── application/
│   ├── window_service.go    # WindowService: CreateYear/Open/Close/Amend
│   ├── snapshot.go          # ReportData <-> JSON helpers
│   └── errors.go            # typed errors for callers (TUI/server) to localize
├── domain/
│   ├── model/money.go       # + MarshalJSON / UnmarshalJSON
│   └── ports/               # + ReportRenderer, TxManager, RepoSet
└── adapters/
    ├── persistence/txmanager.go        # TxManager impl + RepoSet over sqlc WithTx
    └── report/noop_renderer.go         # no-op ReportRenderer (Phase 5 replaces)
```

### 2.1 New ports (`internal/domain/ports`)

- `ReportRenderer interface { Render(rd report.ReportData, generatedAt time.Time) ([]byte, error) }`
- `TxManager interface { WithinTx(ctx context.Context, fn func(RepoSet) error) error }`
- `RepoSet` — a struct bundling the **tx-scoped** repositories handed to the closure:
  ```
  RepoSet{
      Partners PartnerRepository
      Forecasts ForecastRepository
      Windows  WindowRepository
      Taxonomy TaxonomyRepository
      Sections SectionRepository
      Reports  ReportRepository
      Audit    AuditLog
  }
  ```

The persistence adapter implements `WithinTx`: begin a tx on the shared `*sql.DB`, build
each repository over `sqlc.New(conn).WithTx(tx)` into a `RepoSet`, run `fn`, then commit on
nil error or roll back on error/panic. **Every WindowService operation runs inside one
`WithinTx` closure**, so reads and writes are atomic and consistent.

### 2.2 `Money` JSON (`model/money.go`)

- `MarshalJSON` → the quoted canonical fixed-scale string (`"31900.00"`).
- `UnmarshalJSON` → parse that string via `MoneyFromString`.

This makes `json.Marshal(report.ReportData)` work and round-trip losslessly. `encoding/json`
is stdlib; the domain stays free of DB/adapter dependencies.

### 2.3 `WindowService`

Depends only on `ports.TxManager`, `ports.ReportRenderer`, `ports.Clock`:
```
func NewWindowService(tx ports.TxManager, renderer ports.ReportRenderer, clock ports.Clock) *WindowService
func (s *WindowService) CreateYear(ctx context.Context, year int) (model.SubmissionWindow, error)
func (s *WindowService) Open(ctx context.Context, year int) error
func (s *WindowService) Close(ctx context.Context, year int) (model.Report, error)
func (s *WindowService) Amend(ctx context.Context, year int) (model.Report, error)
```

---

## 3. Operations & validation

`now := clock.Now()` at the top of each; each body is one `WithinTx` closure. Typed errors
live in `application/errors.go` (`ErrWrongState`, `ErrDeadlinePassed`,
`ErrIncompleteTaxonomy`, `ErrAnotherWindowOpen`, `ErrYearExists`, `ErrNoPriorYear`,
`ErrWindowNotFound`) so callers render Catalan messages.

### 3.1 CreateYear → DRAFT

- Error `ErrYearExists` if a window for `year` already exists.
- Find the most-recent prior year window; copy its `currentExpenseLimit` /
  `investmentExpenseLimit`; copy its taxonomy (`ListTypes`/`ListSubtypes` →
  `SaveType`/`SaveSubtype` for `year`). If no prior year exists → `ErrNoPriorYear`
  (won't happen post-adopt; canonical seeding is deferred — YAGNI).
- Default `deadline` = 31 Dec `year` 23:59:59 UTC (admin edits during DRAFT).
- Insert `SubmissionWindow{year, DRAFT, openedAt:nil, closedAt:nil, deadline, limits}`.
- No audit event (DRAFT setup; the `AuditKind` enum has no `WINDOW_CREATED`).

### 3.2 Open → DRAFT → OPEN

- Window must exist (`ErrWindowNotFound`) and be `DRAFT` (`ErrWrongState`).
- `deadline > now`, else `ErrDeadlinePassed`.
- Taxonomy has ≥1 `CURRENT` type **and** ≥1 `INVESTMENT` type, else
  `ErrIncompleteTaxonomy`.
- No other window is `OPEN` (`Windows.List`; DB partial unique index is the backstop), else
  `ErrAnotherWindowOpen`.
- `Windows.Save(window.WithState(OPEN).WithOpenedAt(now))`. Audit `WINDOW_OPENED`.
- Taxonomy "locking" while OPEN is enforced by the editing callers in Phase 7, not here.

### 3.3 Close → OPEN → CLOSED

Window must be `OPEN` (else `ErrWrongState`). Then:

1. Gather inputs via tx repos: enabled forecasts (`Forecasts.ListByYear(year)` filtered to
   `Enabled()`); `Partners.List()`; active `Sections.List()` + `Sections.ListMemberships()`;
   `subtypeCategory` (`map[subtypeCode]ExpenseCategory` from `Taxonomy.ListTypes/ListSubtypes`);
   limits from the window.
2. `rd := services.Compute(AllocationInput{…})` (Phase 3).
3. **Collect approved amounts** from every `DetailItem` in `rd` — common items, section
   items, and per-partner items — keyed by forecast id (common/section = gross, partner =
   prorated). Matches Java `collectApproved`.
4. For each enabled forecast in that map: if `approvedAmount` already equals the computed
   value **and** `approvedOn != nil`, skip; else
   `Forecasts.Save(f.WithApprovedAmount(v).WithApprovedOn(now))`. Count writes.
5. `snapshot := SnapshotToJSON(rd)` (`json.Marshal`).
6. `pdf, _ := renderer.Render(rd, now)` (no-op → `[]byte{}` in Phase 4).
7. `id := Reports.Insert(Report{year, generatedAt:now, snapshotJSON:snapshot, pdf, supersededAt:nil})`.
8. `Windows.Save(window.WithState(CLOSED).WithClosedAt(now))`.
9. `Audit.Append(WINDOW_CLOSED, "SubmissionWindow", year, now, {"reportId":id,"forecastsApproved":n})`.
10. Return the saved `Report`.

### 3.4 Amend (regenerate a CLOSED year)

Window must be `CLOSED` (else `ErrWrongState`). Same compute + collect + persist-approved +
snapshot + render as Close, but: before inserting, `Reports.MarkSuperseded(latest.id, now)`
(via `FindLatestByYear`); insert the new `Report`; **no window state change**; audit
`REPORT_GENERATED`. Returns the new `Report`.

### 3.5 Atomicity

Each operation is one `WithinTx` closure, so any failure (allocation error, save error,
renderer error) rolls back the whole operation — no half-closed window, no orphan report, no
partial approved-amount writes.

---

## 4. Persistence & port additions

- `SectionRepository.ListMemberships(ctx) ([]model.PartnerSection, error)` — list **all**
  partner-section rows (the allocation warning needs every membership). New sqlc query +
  method + round-trip test.
- `TxManager` impl + `RepoSet` (`internal/adapters/persistence/txmanager.go`).
- No-op `ReportRenderer` (`internal/adapters/report/noop_renderer.go`) returning `[]byte{}`.
- **No schema migration** — empty `[]byte` satisfies `pdf BLOB NOT NULL`.
- Reused as-is from Phase 2: `ForecastRepository.ListByYear/Save`, `ReportRepository.Insert/
  FindLatestByYear/MarkSuperseded`, `WindowRepository.List/FindByYear/Save`,
  `TaxonomyRepository.ListTypes/ListSubtypes/SaveType/SaveSubtype`, `AuditLog.Append`.

---

## 5. Testing (TDD)

- **Money JSON** unit test: `MarshalJSON`/`UnmarshalJSON` round-trip (incl. `"1322.22"` and
  a negative); and embedded in a `report.ReportData` `json.Marshal`/`Unmarshal` round-trip.
- **TxManager** test (real temp SQLite): commit persists; a returned error rolls back (no
  rows written); a panic rolls back.
- **WindowService** tests (real temp SQLite via the persistence `TxManager` + no-op renderer
  + a fake `Clock`):
  - `CreateYear`: copies prior-year taxonomy + limits, creates DRAFT; `ErrYearExists`;
    `ErrNoPriorYear`.
  - `Open`: happy path → OPEN + openedAt + `WINDOW_OPENED` audit; rejects non-DRAFT, past
    deadline, missing a category, another window OPEN.
  - `Close` over a small seeded scenario: forecasts get `approvedAmount`/`approvedOn`; a
    `Report` is inserted whose snapshot JSON deserializes to the expected numbers; window →
    CLOSED; `WINDOW_CLOSED` audit payload correct; rejects non-OPEN; skip-unchanged path
    doesn't rewrite.
  - `Amend` on a CLOSED year: prior report `supersededAt` set; new report is the latest
    non-superseded; approved amounts updated; window stays CLOSED; `REPORT_GENERATED` audit.
- Scenarios are small synthetic seeds (full golden-number validation lives in Phase 3);
  these assert orchestration wiring, persistence, and state transitions.

---

## 6. Scope

**In Phase 4:** `WindowService` (CreateYear/Open/Close/Amend); `TxManager`/`RepoSet`;
`ReportRenderer` port + no-op; `Money` JSON; snapshot helpers; `Sections.ListMemberships`.

**Not in Phase 4:** PDF/MD/HTML rendering (Phase 5/6); auto-close scheduler + notifications
(deferred per overview); taxonomy/partner/section *editing* services, the year picker, and
soci forecast CRUD (Phase 6/7 driving adapters).

---

## 7. References

- Overview: `docs/superpowers/specs/2026-06-26-espigol-go-overview-design.md` (§4, §5.3, §6).
- Phase 3: `docs/superpowers/specs/2026-06-28-espigol-go-phase-3-allocation-design.md`
  (`services.Compute`, `report.ReportData`).
- Java reference: `espigol-java` `application/WindowClosingService.java`,
  `application/ReportSnapshotSerializer.java`.
