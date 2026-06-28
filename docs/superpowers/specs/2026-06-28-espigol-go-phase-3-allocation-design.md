# Espígol (Go) — Phase 3: Allocation Algorithm — Design

**Status:** Approved for implementation · **Date:** 2026-06-28

Phase 3 of the Espígol Go rewrite. A pure domain service that computes the full
`ReportData` from a year's forecasts, faithfully porting espigol-java's
`AllocationService` + `FairShareAllocator`, generalized from 2 hardcoded sections to N
data-driven sections. Parent design:
`docs/superpowers/specs/2026-06-26-espigol-go-overview-design.md` (§5.2). Phase 2 (domain &
persistence) is merged and provides `Money`, the entities, and the sections model.

The authoritative business rules are espigol-java's `domain/services/AllocationService` and
`FairShareAllocator` (golden-validated) and `private/espigol-cmd-spec.md` §8. This phase
reproduces them exactly; the only intentional change is the N-section generalization.

---

## 1. Scope & boundary

**In Phase 3** (all pure domain — no DB, no I/O, no serialization, no rendering):

- `internal/domain/model/report/` — the `ReportData` value-struct tree, with the
  section-specific parts generalized to N sections.
- `internal/domain/services/allocation.go` — `AllocationService.Compute(AllocationInput)
  (ReportData, error)`, the single entry point.
- `internal/domain/services/fairshare.go` — the iterative fair-share distribution
  (`internal`-private helper).
- Synthetic unit tests for the algorithm's edges + a committed, fully-anonymized
  golden-value test.

**Not in Phase 3:**
- Window state machine, open/close orchestration, persisting `approvedAmount`/`approvedOn`,
  JSON serialization of `ReportData`, the `Report` row, audit events → **Phase 4**.
- PDF/Markdown/HTML rendering → **Phases 5/6**.
- Loading inputs from repositories → the caller (Phase 4) does this; Phase 3 only defines
  the `AllocationInput` shape and consumes Phase-2 domain types.

`Compute` produces the complete computed `ReportData` (the same structure the golden MD
reflects); Phase 4 consumes it verbatim.

---

## 2. Architecture

```
internal/domain/
├── model/report/          # ReportData + sub-structs (immutable computed value types)
└── services/
    ├── allocation.go       # AllocationService.Compute(AllocationInput) (ReportData, error)
    └── fairshare.go        # fair-share distribution (unexported to the package)
```

- Pure functions over Phase-2 domain types (`Money`, `ExpenseForecast`, `Partner`,
  `Section`, `PartnerSection`, `ExpenseCategory`). Imports only
  `internal/domain/model`. No `database/sql`, no adapters, no ports.
- `Compute` takes an `AllocationInput` struct (not a long parameter list); Phase 4
  assembles it from repositories.
- The report structs are computed *outputs* assembled in one place, so they are plain
  structs with exported fields (not user-validated entities with unexported fields +
  constructors). Money fields are the Phase-2 `Money` type.

### 2.1 `AllocationInput`

```
AllocationInput{
    Year             int
    Forecasts        []model.ExpenseForecast      // the year's ENABLED forecasts (caller filters)
    Partners         []model.Partner
    Sections         []model.Section              // ACTIVE sections, in display order
    Memberships      []model.PartnerSection       // partner↔section (for the producer warning)
    SubtypeCategory  map[string]model.ExpenseCategory  // subtypeCode → CURRENT/INVESTMENT
    CurrentLimit     model.Money
    InvestmentLimit  model.Money
}
```

---

## 3. `ReportData` model (`internal/domain/model/report`)

Mirrors the Java `ReportData` tree, with `SectionDetail`/`WarningData` generalized to N
sections.

- **`ReportData`** — `Year int`, `HasNegativeRemainder bool`, `Categories []CategoryReportData`
  (CURRENT then INVESTMENT).
- **`CategoryReportData`** — `Category model.ExpenseCategory`, `Common CommonData`,
  `Sections SectionsData`, `Warning *WarningData` (nil unless that category's sections
  remainder < 0).
- **`CommonData`** — `Available, Total, Remainder model.Money`, `Items []DetailItem`.
- **`SectionsData`** — `Available, Total, Remainder model.Money`,
  `SectionDetails []SectionDetail`, `Partners PartnersData`.
- **`SectionDetail`** — `SectionCode string`, `Label string` (replacing Java's
  `ExpenseScope name`), `Items []DetailItem`, `Total model.Money`. One per active section
  that has forecasts for the category; empty sections omitted.
- **`WarningData`** (generalized) — `Category model.ExpenseCategory`,
  `Rows []SectionWarning`.
- **`SectionWarning`** — `SectionCode string`, `Label string`, `Producers int`,
  `Allowed, Requested, Adjustment model.Money`. One row per active section (replacing
  Java's hardcoded olive*/livestock* fields).
- **`PartnersData`** — `SubtypeTotals []SubtypeTotal`, `GrandTotal model.Money`,
  `HasExcess bool`, `FinalRemainder model.Money`, `Allocations []PartnerAllocation`,
  `PartnerDetails []PartnerDetail`.
- **`PartnerAllocation`** — `PartnerID int`, `PartnerName string`, `Requested, Allocated
  model.Money`.
- **`PartnerDetail`** — `Name string`, `Items []DetailItem`, `Total model.Money`,
  `IsCapped bool`, `MaxAuthorized model.Money`.
- **`DetailItem`** — `CpCode string` (the forecast id), `Concept, Description string`,
  `RequestedAmount, ApprovedAmount model.Money`.
- **`SubtypeTotal`** — `SubtypeCode string`, `Amount model.Money`.

**Ordering (matches Java):** detail items by concept; partner allocations and partner
details by name; subtype totals by code; sections by display order.

**Approved-amount rule (matches Java):** Common- and section-scope items have
`approved = gross` (no proration). Only `Soci`-scope partner forecasts are prorated, and
only when the partner is capped.

---

## 4. The Compute algorithm

`Compute` runs the spec §8 waterfall per category (CURRENT, then INVESTMENT), porting the
Java reference. Partner display name format: `name + " (" + id + ")"` (matches Java;
anonymized in tests).

**Per category** (with `limit` = the category's limit):

1. Filter the input's enabled forecasts to this category via `SubtypeCategory[subtypeCode]`.
2. **Common** (`scopeKind == COMMON`): sort by concept; `total = Σ gross`;
   `remainder = limit − total`; items' approved = gross. → `CommonData`.
3. **Sections** (N, generalized): for each active section in display order, filter
   `scopeKind == SECTION && sectionCode == s.code`; skip if empty; sort by concept;
   section total = Σ gross; items' approved = gross. `sectionsTotal = Σ section totals`;
   `availableForSections = limit − commonTotal`; `sectionsRemainder = availableForSections
   − sectionsTotal`.
4. **Warning** (only if `sectionsRemainder < 0`): for each active section, count producer
   members — partners with `partnerType == Productor` AND a `PartnerSection` membership for
   that section. `denominator = Σ those counts` (a producer in two sections counts twice).
   Each section: `allowed = availableForSections × producers / denominator`
   (`denominator == 0 → allowed = 0`); `requested` = that section's total;
   `adjustment = requested − allowed`. → `WarningData{Category, Rows}`.
5. **Socis** (`scopeKind == PARTNER`): aggregate gross per partner; `grandTotal = Σ`;
   `hasExcess = grandTotal > sectionsRemainder`; run `FairShareAllocator` over
   `(sectionsRemainder, partnerTotals)`; build subtype totals (by code), per-partner
   allocations, and per-partner details (proration when capped). → `PartnersData`.

`ReportData.HasNegativeRemainder` = any category's `sectionsRemainder < 0`.

### 4.1 FairShareAllocator (ported verbatim)

Input: `(remainder Money, partnerTotals map[int]Money, partnerNames map[int]string)`.
Output: `(allocations []PartnerAllocation, finalRemainder Money)`.

- Empty partners → `([], remainder)`.
- If `Σrequested ≤ remainder`: everyone gets full request; `finalRemainder = remainder −
  Σrequested`.
- Else iterate (max 100): `mean = budgetLeft / nUnfixed` (HALF_UP scale-2); fix every
  unfixed partner whose allocation ≤ mean (subtract its allocation from `budgetLeft`); if
  none were newly fixed, cap all remaining unfixed at `mean` and stop; after each pass,
  break when `|remainder − Σallocated| < 0.01`.
- `finalRemainder = remainder − Σallocated`. Allocations sorted by name.
- **Non-positive pool:** when `remainder ≤ 0`, the iteration still caps everyone at the
  (≤ 0) mean — no clamping to zero (matches the reference).

### 4.2 Rounding (ported; supported by Phase-2 `Money`)

- `mean = budgetLeft.DividedBy(nUnfixed)` → HALF_UP scale-2.
- Per-item proration when capped: `ratio = allocated.Decimal() / requested.Decimal()` at
  shopspring's default division precision (16 digits, matching Java's `DECIMAL64`), then
  `approved = gross.TimesRatio(ratio)` (rounds to scale-2 HALF_UP). Uncapped or
  `requested == 0` → `approved = gross`.
- All magnitude comparisons via `Money.Cmp` (scale-2). Convergence threshold `0.01`;
  iteration cap `100`.

---

## 5. Testing

Two tiers, both always run in CI.

### 5.1 Synthetic unit tests (pure, hand-written)

- `FairShareAllocator`: no-excess (everyone full); excess with capping; all-above-mean
  (cap all at mean); non-positive pool (mean ≤ 0, no clamp); empty partners; single
  partner; convergence/iteration.
- Per-category mechanics: common remainder; N-section iteration + empty-section skip; the
  warning proportional split across ≥3 sections (incl. a producer in two sections counted
  twice, and `denominator == 0 → allowed 0`); per-item proration when capped
  (approved = gross × ratio, scale-2); subtype-total aggregation; ordering.

### 5.2 Anonymized golden test (committed, self-contained, always runs)

- A committed Go fixture holds the **2026 input with real numbers but anonymized text**:
  the 8 partners (real `id`, `partnerType`, section memberships; `name → "Soci <id>"`); the
  35 forecasts (real `partnerId`, `grossAmount`, `scopeKind`+`sectionCode`, `subtypeCode`,
  `year`/dates; `concept → "Concepte <cpCode>"`, `description → ""`); the active sections
  (`oliva`, `ramaderia`); the subtype→category map; and the limits (30000 / 70000).
- A committed **expected-values fixture** of the golden numbers from the MD (per-category
  Common/Sections/Socis totals and remainders, e.g. current Comú 2880,00 / Seccions
  27.111,00 / Total 29.991,00 / Remanent 9,00; investment Socis 23.498,96 / Remanent
  11.203,04; `hasNegativeRemainder`; key per-partner allocations).
- The test calls `Compute(input)` and asserts the computed `ReportData` **numbers** (Money
  via `.String()`) equal the expected fixture exactly. Only numbers/ids/scopes/subtypes are
  asserted; names/concepts are anonymized — no cooperative data in the repo.
- **Fixture source = the golden MD only** (not the legacy DB). The DB
  (`testdata/legacy-espigol.db`) has diverged from the golden MD — it was edited after the
  MD was produced (35 forecasts vs the MD's 28; differing amounts and scopes, e.g.
  `CP26028` is 12.596 in the DB but 13.187 in the MD). So `Compute(DB data)` would NOT
  match the golden numbers. Both the **input** (28 forecasts, 8 partners, 2 sections,
  limits) and the **expected** values are derived from the single self-consistent golden MD,
  which lists every forecast's CP code, scope/section grouping, per-partner grouping, and
  gross amount. Subtype codes are not shown in the MD and do not affect the asserted totals,
  so each forecast gets a placeholder subtype per category (`"a1"` for CURRENT, `"b1"` for
  INVESTMENT); the subtype→category map maps those two codes. The golden 2026 data has
  positive section remainders in both categories, so **no warning and no capping fire** in
  the golden test — those paths are covered by the synthetic unit tests (§5.1). The
  anonymized fixture values are embedded in the implementation plan (extracted from the MD
  at authoring time); only the anonymized fixture is committed.

---

## 6. References

- Overview design: `docs/superpowers/specs/2026-06-26-espigol-go-overview-design.md` (§5.2).
- Phase 2 design: `docs/superpowers/specs/2026-06-27-espigol-go-phase-2-domain-persistence-design.md`.
- Java reference (authoritative): `espigol-java/src/main/java/coop/estellencs/espigol/domain/services/AllocationService.java`, `FairShareAllocator.java`, and `domain/model/report/`.
- Domain rules: `espigol-java/private/espigol-cmd-spec.md` §8.
- Golden output: `espigol-java/private/report-examples/Previsions de despeses 2026.md`.
- Numeric/structural fixture source (gitignored): `testdata/legacy-espigol.db`.
