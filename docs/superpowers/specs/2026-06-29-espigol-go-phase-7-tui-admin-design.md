# Espígol (Go) — Phase 7: TUI (admin) — Design

**Status:** Approved for implementation · **Date:** 2026-06-29

Final phase of the Espígol Go rewrite. The admin-only terminal UI: a lazygit-style Bubble
Tea application driving per-entity admin application services, plus the deferred wiring of
the real PDF renderer into window close. Parent:
`docs/superpowers/specs/2026-06-26-espigol-go-overview-design.md` (§7 TUI).

**Admin-only, local.** The TUI runs locally (over SSH on the VPS) with **no auth**; the
actor is recorded in the audit log as the administrator. It covers everything *not* on the
socis web server (Phase 6): partner/section/taxonomy/board-authorization management, window
lifecycle, admin forecast CRUD impersonating any partner, and report generation. It calls
the same domain services as the server — just a different driving adapter.

Layout/UX reference: lazygit (visual/interaction parity, rebuilt — not a code port).
Report layout reference: the Phase-5 renderers.

---

## 1. Decisions (resolved during brainstorming)

| Decision | Choice |
|---|---|
| Phase scope | **One phase / one PR** covering both the admin application services and the TUI adapter. |
| Admin services | **Per-entity application services** (PartnerService, SectionService, TaxonomyService, BoardAuthorizationService, ReportService) + admin methods on ForecastService; reuse WindowService. |
| Admin forecast editing | Allowed while the year's window is **DRAFT or OPEN**, not CLOSED; bypasses soci ownership/scope authz; logged as admin. |
| Report `r` action | **Live preview for DRAFT/OPEN** (compute + render, nothing stored); **stored snapshot export for CLOSED**. Plus wire the real PDFRenderer into Close. |
| TUI structure | **Modular `Panel` interface**: one file per entity panel + a root Model owning focus/header/status/modals. |
| TUI testing | **Headless `Update`/`View` unit tests** (state transitions, key actions, View substrings); no golden-file snapshots. Services carry heavy logic coverage. |
| TUI libs | Charm stack: `bubbletea`+`lipgloss` (present) + **`bubbles`** (new): `list`, `table`, `textinput`, `viewport`, `key`, `help`. |

---

## 2. Architecture

```
internal/
├── application/                 # NEW per-entity admin services (over ports.TxManager)
│   ├── partner_service.go       # partners + section memberships CRUD
│   ├── section_service.go       # sections CRUD (add vinya, etc.)
│   ├── taxonomy_service.go      # per-year types/subtypes CRUD (DRAFT only)
│   ├── board_auth_service.go    # board authorizations grant/revoke
│   ├── report_service.go        # Preview(year)->live ReportData; Latest(year)->stored Report
│   ├── forecast_service.go      # + AdminCreate/AdminUpdate/AdminDelete
│   ├── window_service.go        # reuse (CreateYear/Open/Close/Amend)
│   └── errors.go                # extend
├── adapters/
│   ├── report/exporter.go       # + ExportData(rd, generatedAt, outputDir) for live preview
│   └── tui/                     # modular Bubble Tea app (replaces the Phase-1 stub)
│       ├── app.go               # root Model: focus, header (year ctx), status/help, modals
│       ├── panel.go             # Panel interface + Action type
│       ├── styles.go            # lipgloss styles incl. state-as-color
│       ├── form.go confirm.go   # modal forms + confirmation modal
│       └── panel_{years,partners,sections,taxonomy,forecasts,reports}.go
├── config/config.go             # + Admin.Email
└── wire/wire.go                 # + TUI(cfg) assembling services + real PDFRenderer
```

- **TUI is a driving adapter:** depends on `application` services + the `report` adapter +
  `config`; never imports repositories or `database/sql`.
- **`wire.TUI(cfg) (*tui.App, error)`** opens the DB, builds repos + `TxManager`, constructs
  `WindowService` with the **real `report.PDFRenderer`** (business name + logo from config),
  all per-entity services, `ReportService`, the `report.ReportExporter`, and the root TUI
  model. `cmd/espigol` default branch calls `wire.TUI(cfg)` then runs it (replacing the stub
  `tui.Run`).
- **Layering:** `application` → `ports`/`model` + stdlib only. `tui` → `application` +
  `report` + `config` + Charm libs + stdlib. `report` stays domain + stdlib. CGO-free.

---

## 3. Admin application services

Each is small, mirrors the existing `WindowService`/`ForecastService`, runs every mutation
in one `TxManager.WithinTx`, and writes an `AuditEvent` with `actorEmail = cfg.Admin.Email`
(via a shared `adminAudit` helper analogous to `forecastAudit`). Typed errors extend
`application/errors.go`.

- **`PartnerService`** — `Create`/`Update` (name, surname, VAT, email, mobile, `PartnerType`,
  RIA number, board flag), enable/disable, `SetSectionMemberships(partnerID, []model.PartnerSection)`.
  Validates id/email uniqueness and a valid `PartnerType`. Partner *types* remain the existing
  `PartnerType` enum (no separate type CRUD — YAGNI).
- **`SectionService`** — `Create`/`Update` (code, label, active, order). Rejects duplicate
  codes; rejects disabling a section still referenced by forecasts in a non-closed year.
- **`TaxonomyService`** — `CreateType`/`UpdateType`/`DeleteType`,
  `CreateSubtype`/`UpdateSubtype`/`DeleteSubtype` for a year, **only while that year's window
  is `DRAFT`** (`ErrTaxonomyLocked` otherwise). Subtype must reference an existing type;
  delete rejected if referenced by a forecast.
- **`BoardAuthorizationService`** — `Grant(partnerID, scopeKind, sectionCode)` /
  `Revoke(...)`. Validates the partner is a board member and (for SECTION) the section exists;
  rejects duplicate grants.
- **`ForecastService` admin methods** — `AdminCreate(ctx, actorEmail string, partnerID int, in ForecastInput) (model.ExpenseForecast, error)`,
  `AdminUpdate(ctx, actorEmail, id string, in ForecastInput) error`,
  `AdminDelete(ctx, actorEmail, id string) error`. Impersonate any partner; allow any scope
  (PARTNER/COMMON/SECTION) — **bypass** ownership/scope authz; enforce the year's window is
  **DRAFT or OPEN** (`ErrWindowNotEditable` on CLOSED); reuse existing subtype/section-for-year
  validation. Soci-facing methods are unchanged. Audit actor = `actorEmail`.
- **`ReportService`** — `Preview(ctx, year) (report.ReportData, error)` computes the live
  allocation for DRAFT/OPEN (reusing the same compute path as `Close`, refactored into a
  shared helper so preview and close cannot diverge); `Latest(ctx, year) (model.Report, bool, error)`
  returns the stored snapshot for CLOSED. No file I/O (the TUI orchestrates export).

`WindowService` is reused unchanged for CreateYear/Open/Close/Amend; only its injected
renderer changes (§5).

---

## 4. TUI (lazygit-style, modular)

### 4.1 Root model & layout
`app.go` holds an ordered `[]Panel`, the focused index, the **year context** (selected year,
shown in the top bar), a status/keybinding line (`bubbles/help`), and a modal overlay stack.
`Update` routes global keys (panel switch, quit, help) itself and delegates the rest to the
focused panel; modals, when open, capture input. `View` composes with `lipgloss`: a left
column of stacked panel headers + the focused panel's list, a main panel with the selected
item's detail, the top year bar, and the bottom keybinding line.

### 4.2 Panel interface
`panel.go`:
```
type Action struct { Key string; Label string }
type Panel interface {
    Title() string
    Update(msg tea.Msg) (Panel, tea.Cmd)   // panel-local keys + data refresh
    View(width, height int) string          // the panel's list
    Detail() string                          // main-panel detail for the selected item
    Actions() []Action                       // single-letter actions shown in the status line
}
```
Six implementations (left column order = overview §7):
- **Anys** (windows) — `n` new year, `o` open, `c` close (confirm), `r` report, `a` amend.
- **Socis** (partners) — `n/e/d`, `m` edit section memberships, `b` toggle board member.
- **Seccions** — `n/e/d` (add `vinya`).
- **Tipus i subtipus** (taxonomy for the year-context) — `n/e/d`, **disabled unless the year
  is DRAFT** (actions hidden + a notice).
- **Previsions** (forecasts for the year-context, all partners) — `n/e/d` via the admin
  forecast methods (the form includes a partner selector + scope selector).
- **Informes** (reports) — `r` generate/export, shows the last written file paths.

### 4.3 Navigation, keymaps, modals
`↑/↓`,`j/k` move within a panel; `Tab`/`Shift+Tab` (+ left/right) switch panels; `Enter`
drills into detail; single-letter actions per the focused panel; `?` help overlay; `q` quit.
Destructive/irreversible actions (`d` delete, `c` close) require a **confirmation modal**
(`confirm.go`). Create/edit open a **form modal** (`form.go`) built from `bubbles/textinput`
fields plus simple selectors for enums/scope/partner/subtype; submit calls the matching
service and refreshes the panel; validation errors render inline (Catalan).

### 4.4 State-as-color (`styles.go`)
Centralized `lipgloss` styles: `DRAFT` grey/yellow, `OPEN` green, `CLOSED` blue; disabled
rows dimmed; capped/over-budget amounts red.

### 4.5 Refresh model
Panels load data via the services on init and re-query after any mutation (no shared mutable
cache), keeping the screen consistent with the DB. All user-facing text Catalan.

---

## 5. Report generation flow & Close wiring

- **Close wiring (deferred Phase-5 step):** `wire.TUI` constructs `WindowService` with the
  real `report.PDFRenderer` (config business name + logo path) instead of the no-op. `Close`
  logic is unchanged; it now stores a real PDF BLOB + snapshot.
- **`r` branches by the selected year's window state:**
  - **CLOSED** → `ReportService.Latest(year)` → `report.ReportExporter.Export(rep, cfg.OutputDir)`
    writes `Previsions de despeses <year>.pdf` (immutable BLOB) + `.md` (rendered from the
    stored snapshot). Frozen numbers preserved.
  - **DRAFT/OPEN** → `ReportService.Preview(year)` → `report.ReportExporter.ExportData(rd, now, cfg.OutputDir)`
    renders PDF (`PDFRenderer`) + MD (`MarkdownRenderer`) fresh and writes both files. A
    preview; nothing stored.
- **`outputDir`** = `cfg.OutputDir` (with `~` expansion, as `Export` already does). The TUI
  shows the written paths in the Informes detail / status line.
- **`ExportData(rd report.ReportData, generatedAt time.Time, outputDir string) error`** is the
  only new report-adapter code; `Export` (BLOB-based) is reused unchanged. File-writing stays
  in the report adapter; the TUI just selects the path by window state.

---

## 6. Testing (TDD)

- **Application services** (bulk coverage) — real SQLite via `TxManager`:
  - PartnerService incl. memberships and uniqueness; SectionService (duplicate-code +
    referenced-section guards); TaxonomyService (DRAFT-only gate, ≥1 CURRENT/≥1 INVESTMENT
    rule, delete-referenced guard); BoardAuthorizationService (board-only, section exists,
    duplicate guard); ForecastService admin methods (impersonation; DRAFT/OPEN allowed, CLOSED
    → `ErrWindowNotEditable`; audit actor = admin email); ReportService (`Preview` numbers vs
    the anonymized golden 2026 fixture; `Latest` for a closed year).
- **TUI** — headless `Update`/`View` unit tests: a key press opens the correct form/confirm;
  form submit calls the service and the list refreshes; panel focus/navigation transitions;
  state-as-color present in `View()` output; taxonomy actions disabled when the year isn't
  DRAFT. No golden-file snapshots.
- **Report adapter** — `ExportData` test: export to a temp dir; assert both files exist, the
  `.pdf` has the `%PDF` prefix, and the `.md` contains golden EU-formatted numbers.
- Full suite CGO-free; `go vet` clean.

---

## 7. Configuration, wiring, scope

- **Config:** add `Admin.Email` (env `ESPIGOL_ADMIN_EMAIL`, default `"admin@espigol"`) — the
  audit actor for TUI mutations.
- **Wiring:** `wire.TUI(cfg)` assembles services + real PDFRenderer + the root model;
  `cmd/espigol` default (non-`--server`) branch runs it. The Phase-1 stub `tui.Run` is
  replaced.
- **Scope — in:** the six per-entity admin services + ReportService; ForecastService admin
  methods; `report.ExportData`; real PDFRenderer wired into Close; the full modular Bubble Tea
  TUI (panels, navigation, forms, confirm modals, state-color, help); `wire.TUI` + `cmd`
  branch; the `bubbles` dependency; `Admin.Email` config.
- **Scope — out:** the socis web server (Phase 6, done); auto-close scheduler + email
  notifications (deferred); any TUI auth (local/SSH, none — actor is the admin); partner-type
  CRUD (stays the `PartnerType` enum).

---

## 8. References

- Overview: `docs/superpowers/specs/2026-06-26-espigol-go-overview-design.md` (§7).
- Phase 4: `application` `WindowService` (CreateYear/Open/Close/Amend), `TxManager`,
  `ReportRenderer` port. Phase 5: `report` `PDFRenderer`/`MarkdownRenderer`/`ReportExporter`,
  `buildLayout`. Phase 6: `application.ForecastService` + `internal/wire` pattern.
- Layout reference: lazygit. Numbers reference: the anonymized golden 2026 fixture (Phase 3).
