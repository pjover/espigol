# Subsidy reconciliation — Phase 2: the assignment algorithm

**Status:** design, awaiting implementation.
**Source analysis:** `private/espigol-subsidy-reconciliation-spec.md` (Part B.2 of the "Ajuts 2025"
workbook description).
**Prior phase:** `docs/superpowers/specs/2026-07-08-subsidy-reconciliation-phase1-design.md`
(entities, persistence, ingestion — now live with 2025 data).
**This spec covers Phase 2 only** — the pure computation that turns imported grants + invoices
into a per-forecast assigned subsidy (`Subvenció assignada`). No persistence, no UI, no report
rendering — those are Phase 3.

## Goal

Phase 1 stores what was granted (`Concession`) and what was invoiced (`Invoice` + payments +
`ForecastInvoice` links). It does **not** answer the three questions the workbook exists to answer,
per Part B.4:

1. Did each `ExpenseForecast` receive a subsidy?
2. Which invoices document it, and what share?
3. How much subsidy is *assigned* to it?

Phase 2 provides the deterministic function that computes those answers over the whole year — an
in-memory `ReconciliationData` snapshot tree that Phase 3 will render to PDF/MD/HTML and store on
a snapshot row. Phase 2 itself has no writes, no TUI, no persistence.

## Decomposition context

- **Phase 1 (done):** entities + persistence + ingestion (JSON + TUI CRUD).
- **Phase 2 (this spec):** the algorithm — pure computation returning a nested snapshot.
- **Phase 3:** report rendering + snapshot persistence + TUI trigger.

Chosen decomposition: **horizontal, math-first**. Phase 2 is a library. Phase 3 owns the
snapshot-row schema, the trigger key in the TUI, and the renderers.

## Decisions (from brainstorming)

- **Deliverable:** a pure-computation library. `Compute(ReconciliationInput) → ReconciliationData`
  as a domain service, plus a thin `ReconciliationService.Compute(ctx, year)` orchestrator that
  fetches through ports and delegates. No persistence, no TUI, no snapshot row.
- **Payment gating:** all-or-nothing at fully-paid. An invoice contributes to `Executed` iff
  `Σ payments ≥ netAmount − 0.01`. Partially-paid or unpaid invoices contribute 0 to `Executed`
  and instead accumulate as `Pending` on the forecast.
- **Compensation scope:** per-Concessió-group cap of `min(Granted, Executed)`, then per-forecast
  proration by share of paid Executed with largest-remainder cent-closing. Cross-group
  compensation within a category is *reported* via `NetDeviation` (the workbook's `Desviacions!K`)
  but does **not** change any per-forecast `Assigned`. Inspector-negotiated compensation happens
  outside espigol.
- **Category grouping for deviations:** sum all subtypes belonging to the same
  `ExpenseCategory` (CURRENT/INVESTMENT), regardless of specific subtype pairings the workbook
  happened to use. This is more general than the workbook's ad-hoc pairing and matches the funder
  rule ("spend may be re-attributed within a category").
- **Program in English, Catalan in UI.** Status enum values are English identifiers
  (`StatusFullyJustified`, `StatusPaymentPending`, …); Phase 3 will map them to Catalan labels
  when rendering.
- **All money is `model.Money`** (scale 2, HALF_UP). Every proration uses `TimesRatio` with a
  largest-remainder pass. No `float64`.

## Architecture

Mirrors the existing `AllocationService` / `FairShareAllocator` pattern (Phase 3 of the original
espigol build): pure math in `internal/domain/services`, application orchestrator in
`internal/application`.

```
internal/domain/services/
  reconciliation.go        Compute(ReconciliationInput) → ReconciliationData
  reconciliation_test.go   Unit tests: one per invariant

internal/application/
  reconciliation_service.go   (existing) + new method:
                              Compute(ctx, year) → ReconciliationData
  reconciliation_service_test.go
                              (existing) + one integration test using newReconWorld helper
```

No new migrations, no new sqlc queries, no new repositories, no wire changes, no TUI changes.
All reads flow through existing ports: `ForecastRepo.ListByYear`, `ConcessionRepo` (via existing
`ListConcessions` / `ListConcessionLinks`), `InvoiceRepo.ListByYear`, `TaxonomyRepo.ListSubtypes`,
`PartnerRepo.FindByID`. `Compute` runs inside a single `TxManager.WithinTx` (read-only, but keeps
snapshot consistency).

## Algorithm

Pure function over an in-memory input. Per year. Only forecasts with `Enabled == true` participate;
disabled forecasts are skipped entirely (they contribute nothing to `Executed`, don't appear in the
output, and their links are ignored).

### 1. Executed per forecast

For each enabled `ExpenseForecast` `i`:

- `Executed_i` = `Σ ForecastInvoice.Amount` over links whose invoice is *fully paid*
  (`Σ InvoicePayment.Amount ≥ Invoice.NetAmount − 0.01`).
- `Pending_i`  = `Σ ForecastInvoice.Amount` over links whose invoice is *not* fully paid.

An invoice's paid-status is computed once per invoice and reused.

### 2. Per Concessió group

For each `Concession` `g`, with member forecasts `F_g` (from `ConcessionForecast`):

- `Executed_g = Σ Executed_i for i in F_g`
- `Base_g = min(Granted_g, Executed_g)` — the justifiable base (per-group cap).
- If `Executed_g > 0`:
  `Assigned_i = Base_g × (Executed_i / Executed_g)` for each `i ∈ F_g`, with a largest-remainder
  pass so `Σ Assigned_i = Base_g` exactly (no rounding leak). Reuses the pattern from
  `internal/domain/services/allocation.go:176-180`.
- If `Executed_g == 0`: `Assigned_i = 0` for all `i ∈ F_g` (regardless of `Granted_g > 0`). The
  forecast's status becomes `StatusNoInvoice` or `StatusPaymentPending`.

### 3. Roll-ups

Computed in the same pass:

- **Per Concessió:** `Requested, Granted, Executed, Assigned, Difference = Granted − Executed`.
- **Per Subtype:** sum over its concessions. `Deviation_subtype = Granted − Executed` (positive =
  under-spent, negative = over-spent).
- **Per Category** (CURRENT/INVESTMENT): sum over its subtypes.
  `NetDeviation_category = Σ Deviation_subtype`. This is the workbook's `Desviacions!K` figure.
  Reporting only — does not change any `Assigned_i`.

### 4. Per-forecast status

Assigned as the algorithm walks each forecast. Precedence — the first matching row wins, so
per-forecast issues (no invoice, pending payment, individual over-execution) surface before
group-level ones:

| Precedence | Status                     | Condition                                                        |
|-----------:|----------------------------|------------------------------------------------------------------|
| 1          | `StatusNoInvoice`          | Forecast has zero links (paid *and* unpaid).                     |
| 2          | `StatusPaymentPending`     | Forecast has unpaid links (`Pending_i > 0`), possibly some paid. |
| 3          | `StatusOverExecuted`       | Forecast's paid `Executed_i > GrossAmount_i`.                    |
| 4          | `StatusPartiallyJustified` | Group `Executed_g < Granted_g` (grant only partly covered).      |
| 5          | `StatusFullyJustified`     | Group `Executed_g ≥ Granted_g` (full grant used).                |

Rationale: the per-forecast row in the Phase 3 report should show what's actionable for *that*
forecast (missing invoice, awaiting payment, spent more than planned) rather than a group-level
condition that appears anyway on the concession's own row.

## Return type

Colocated in `internal/domain/services/reconciliation.go`. Fields are plain (no interfaces, no
pointers except where a value is legitimately optional) so `json.Marshal` produces a clean
snapshot for Phase 3 to persist as-is.

```go
type ReconciliationData struct {
    Year       int
    Categories []CategoryReconciliation
}

type CategoryReconciliation struct {
    Category     model.ExpenseCategory // CURRENT | INVESTMENT
    Requested    model.Money
    Granted      model.Money
    Executed     model.Money
    Assigned     model.Money
    NetDeviation model.Money           // Σ subtype.Deviation
    Subtypes     []SubtypeReconciliation
}

type SubtypeReconciliation struct {
    Code, Label string
    Requested, Granted, Executed, Assigned model.Money
    Deviation   model.Money            // Granted − Executed (raw, before category-net)
    Concessions []ConcessionReconciliation
}

type ConcessionReconciliation struct {
    GroupCode, Concept string
    Requested, Granted, Executed, Assigned model.Money
    Difference  model.Money            // Granted − Executed
    Forecasts   []ForecastReconciliation
}

type ForecastReconciliation struct {
    ForecastID     string
    PartnerID      int
    Concept        string
    GrossAmount    model.Money         // ExpenseForecast.GrossAmount ("Previst")
    ApprovedAmount model.Money         // ExpenseForecast.ApprovedAmount ("Aprovat")
    Executed       model.Money         // Σ paid link amounts
    Pending        model.Money         // Σ unpaid link amounts
    Assigned       model.Money         // Subvenció assignada — the answer
    Status         ForecastReconStatus
    Invoices       []InvoiceContribution
}

type InvoiceContribution struct {
    InvoiceID    int64
    Issuer       string
    Number       string
    IssueDate    time.Time
    LinkedAmount model.Money           // this forecast's slice of the invoice
    FullyPaid    bool                  // Σ payments ≥ netAmount − 0.01
    PaidOn       *time.Time            // latest payment date if fully paid; else nil
}

type ForecastReconStatus int

const (
    StatusFullyJustified ForecastReconStatus = iota
    StatusPartiallyJustified
    StatusOverExecuted
    StatusPaymentPending
    StatusNoInvoice
)
```

**Empty categories are omitted.** If a year has only INVESTMENT concessions, `Categories` has one
element, not two. Same rule for empty subtypes / empty concessions.

**Ordering:**

- `Categories` in enum order (CURRENT first, then INVESTMENT).
- `Subtypes` alphabetical by `Code` within a category.
- `Concessions` by `GroupCode` within a subtype.
- `Forecasts` by `ForecastID` within a concession.
- `Invoices` on a forecast by `IssueDate` then `Number`.

## Input assembly (application layer)

`ReconciliationService.Compute(ctx, year)` runs one `WithinTx` block:

```go
func (s *ReconciliationService) Compute(ctx context.Context, year int) (services.ReconciliationData, error) {
    var out services.ReconciliationData
    err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
        forecasts,   _ := r.Forecasts.ListByYear(ctx, year)
        concessions, _ := r.Concessions.ListByYear(ctx, year)
        links,       _ := r.Concessions.ListLinksByYear(ctx, year)
        invoices,    _ := r.Invoices.ListByYear(ctx, year)  // includes payments + links (aggregate)
        subtypes,    _ := r.Taxonomy.ListSubtypes(ctx, year)
        types,       _ := r.Taxonomy.ListTypes(ctx, year)
        partners     := loadPartners(ctx, r, forecasts)     // by unique partner id

        in := services.ReconciliationInput{
            Year: year, Forecasts: forecasts, Concessions: concessions, Links: links,
            Invoices: invoices, Subtypes: subtypes, Types: types, Partners: partners,
        }
        out, err = services.Compute(in)
        return err
    })
    return out, err
}
```

Read-only. All error handling is standard sentinel wrap + rollback.

## Testing

Two layers, no live-DB coupling.

### Domain unit tests — `services/reconciliation_test.go`

One test per invariant, in-memory `ReconciliationInput`:

1. **Largest-remainder cent-closing.** Group `Granted=100.00` with two forecasts, `Executed=33.33`
   each. Assert `Σ Assigned = 66.66` (or `33.33 + 33.34` after remainder pass) and equals
   `min(Granted, Σ Executed)` exactly.
2. **Per-group cap on over-execution.** `Granted=100`, forecasts summing `Executed=150` → group's
   `Assigned=100`, forecasts prorated proportionally.
3. **Per-group cap on under-execution.** `Granted=100`, `Executed=60` → `Assigned=60`, status
   `StatusPartiallyJustified`.
4. **Payment-gate exclusion.** Invoice with `Σ payments < netAmount` contributes 0 to `Executed`
   and its full amount to `Pending`.
5. **No-invoice forecast.** Forecast with zero links → `Executed=0`, `Pending=0`, status
   `StatusNoInvoice`.
6. **Payment-pending forecast.** Forecast with one paid + one unpaid link → non-zero `Executed`
   and non-zero `Pending`, status `StatusPaymentPending`.
7. **Over-executed forecast, group capped.** Forecast's `Executed > GrossAmount`, group still
   capped by grant → status `StatusOverExecuted`.
8. **Category-net deviation.** One CURRENT category with subtype `a4` over-spent by 461 and
   subtype `a6` under-spent by 879 → `CategoryReconciliation.NetDeviation = 418`.
9. **Empty-category omission.** Input with only INVESTMENT concessions → `Categories` has one
   element.
10. **Enabled-forecast filter.** Forecast with `Enabled == false` is skipped entirely, even if
    it has links.

### Application integration test — `reconciliation_service_test.go`

One golden test. Skips with `t.Skip` if `private/export-reconciliation.json` is absent.

- Seed with `newReconWorld` helper (already exists): 2025 window, partners 1/2/4/5/6/7/8/9,
  taxonomy (a2/a3/a4/a6/b1/b2 + their types + categories), section "oliva", and 38 forecasts
  CP25001..CP25038 with concepts and gross amounts matching the workbook.
- Load `private/export-reconciliation.json` via `importer.LoadReconciliation` and run
  `AdminImport`.
- Call `Compute(ctx, 2025)`.
- Assertions against workbook figures (`private/espigol-subsidy-reconciliation-spec.md` Appendix):
  - Per-subtype `Executed`: `a2=5989.00`, `a3=0.00`, `a4=1381.11`, `a6=18672.09`, `b1=52752.80`,
    `b2=1460.00`. Grand total `Executed=80255.00`.
  - `A6-02 Adob orgànic` (CP25006 + CP25007) is `StatusFullyJustified`, `Assigned=13880.00`,
    per-forecast split proportional to `Executed`.
  - `B2-01 Arreglar marges` (CP25027): `Granted=1766.12`, `Executed=1460.00`, `Assigned=1460.00`,
    status `StatusPartiallyJustified` (workbook cross-check).
  - Every forecast in the output has a status set (no zero value).
  - CURRENT category `NetDeviation` matches the workbook's category-net figure.

## Out of scope (Phase 2)

- Report rendering (PDF/MD/HTML) → **Phase 3**.
- Persistence of the snapshot (a new `ReconciliationSnapshot` row analogous to `Report`) →
  **Phase 3**.
- Any TUI change — no key to trigger `Compute` yet → **Phase 3** wires the trigger.
- `SplitKey` / `RepartimentKey` (physical-quantity split ratios from the workbook's `Repartiment
  adob i fitos` / `Menjar animals` tabs) — currently the split is embedded in `ForecastInvoice`
  link amounts at import time; formalising the split-ratio entity is deferred beyond Phase 3.
- Inspector-negotiated compensation overrides. Phase 2 surfaces `NetDeviation` for the admin to
  see; any manual override lives outside this system for now.
- Server / socis-facing surface — TUI/admin flows only.
