# Subsidy reconciliation — Phase 3: the report output

**Status:** design, awaiting implementation.
**Source analysis:** `private/espigol-subsidy-reconciliation-spec.md` (Part B.3 of the "Ajuts 2025" workbook description).
**Prior phases:**
- Phase 1 (merged) — `docs/superpowers/specs/2026-07-08-subsidy-reconciliation-phase1-design.md`: entities, persistence, ingestion.
- Phase 2 (merged) — `docs/superpowers/specs/2026-07-09-subsidy-reconciliation-phase2-design.md`: the pure `services.ComputeReconciliation(ReconciliationInput) → ReconciliationData` algorithm.

**This spec covers Phase 3 only** — persist the computed snapshot, render it to PDF + Markdown, and expose a TUI trigger. No server/socis-facing HTML view.

## Goal

Phase 2 stops at a computed `ReconciliationData` snapshot in memory. Phase 3 makes that snapshot real: writes it to a `reconciliation_snapshot` row, renders it to a PDF and a Markdown file on disk (both landing in `$ESPIGOL_HOME/reports/`), and gives the admin a single TUI keystroke to regenerate it as the year's invoices roll in. It closes the workbook's Part B.3 loop: the admin now sees per-forecast `Subvenció assignada` in a document they can hand to the inspector, plus the group / subtype / category roll-ups that the workbook's `Executat!Resum` and `Desviacions` sheets used to show.

## Decomposition context

- **Phase 1 (done):** entities + persistence + ingestion.
- **Phase 2 (done):** the algorithm (pure computation returning `ReconciliationData`).
- **Phase 3 (this spec):** the report — snapshot row + PDF + Markdown + TUI trigger. No server view.

Chosen decomposition: **horizontal, output-first**. Web view (`/reconciliation/{year}` for socis) is intentionally deferred — this spec bakes zero infrastructure for it. If it happens later, the persisted `snapshot_json` + existing `Block` layout give a clean starting point.

## Decisions (from brainstorming)

- **Full parity with the forecast-report pattern** — same seam (`Report` aggregate + `ReportRepository` port + `ReportRenderer` + `ReportExporter` + TUI key on `[7] Admin`). Different domain, different table, different renderer file — same shape.
- **Overwrite the latest row, no history.** One row per year in `reconciliation_snapshot`, keyed by `year`. Regeneration = upsert. Diverges from `Report`'s `superseded_at` history model because reconciliation is regenerated many times per year and there is no "close" moment worth preserving history against.
- **No server view.** No handler, no template, no HTML renderer. The persisted `snapshot_json` is enough to add one later without schema change.
- **Full 4-level breakdown in the report** — Category → Subtype → Concession → Forecast, with the category NetDeviation and per-subtype/per-concession/per-forecast roll-ups the workbook already shows. Per-forecast rows include the invoice list inline.
- **English identifiers, Catalan display strings.** Status enum values from Phase 2 (`StatusFullyJustified`, …) map to Catalan labels only in the layout builder.
- **All monetary formatting via the existing report helpers** — same currency format as forecast reports (`1.234,56 €`, period thousands, comma decimal, symbol after with a space, always two decimals).

## Architecture

Mirrors the forecast-report seam that has been in the codebase since Phase 5:

```
db/migrations/00004_reconciliation_snapshot.sql   new goose migration
db/queries/reconciliation_snapshot.sql            new sqlc queries (Upsert, GetByYear)

internal/domain/model/
  reconciliation_snapshot.go                       new aggregate (year, generatedAt, snapshotJSON, pdf)
  reconciliation_snapshot_test.go                  constructor + accessor tests

internal/domain/ports/ports.go                     add ReconciliationSnapshotRepository interface
                                                   + ReconciliationRenderer interface
                                                   + ReconciliationExporter interface
                                                   add ReconciliationSnapshots to RepoSet

internal/adapters/persistence/
  reconciliation_snapshot_repository.go            new repo (Save = upsert, FindByYear)
  reconciliation_snapshot_repository_test.go       round-trip + upsert tests
  mapper/reconciliation_snapshot.go                sqlc row ↔ model
  sqlc/reconciliation_snapshot.sql.go              regenerated

internal/adapters/report/
  reconciliation_layout.go                          buildReconciliationLayout(ReconciliationData) → []Block
  reconciliation_layout_test.go                     block-structure tests
  reconciliation_pdf_renderer.go                    ReconciliationPDFRenderer (uses existing renderDocument)
  reconciliation_pdf_renderer_test.go               smoke test (%PDF- prefix, non-empty)
  reconciliation_markdown_renderer.go               ReconciliationMarkdownRenderer
  reconciliation_markdown_renderer_test.go          contains title + status label + Money format
  reconciliation_exporter.go                        ReconciliationExporter (Export + ExportData → paths)

internal/application/
  snapshot.go                                       extend with ReconciliationSnapshotTo/FromJSON
  reconciliation_service.go                         add GenerateReport(ctx, year) + LatestSnapshot(ctx, year)
  reconciliation_service_test.go                    GenerateReport happy-path + overwrite + golden e2e

internal/adapters/tui/
  panel_admin.go                                    add "g" key + generateReconciliationCmd + result msg
  panel_admin_test.go                               test that "g" fires the command
  deps.go                                           add ReconciliationExporter

internal/wire/wire.go                               instantiate + inject renderers + repo + exporter
```

No new adapter *directory*. Everything reconciliation-report lives beside the existing forecast-report code in `internal/adapters/report/` and `internal/adapters/persistence/`.

## Data model

### `reconciliation_snapshot` table

```sql
CREATE TABLE reconciliation_snapshot (
    year          INTEGER PRIMARY KEY,
    generated_at  TEXT NOT NULL,          -- RFC 3339
    snapshot_json TEXT NOT NULL,          -- json.Marshal(services.ReconciliationData)
    pdf           BLOB NOT NULL,          -- rendered PDF bytes
    FOREIGN KEY (year) REFERENCES submission_window(year)
);
```

`year` is the primary key, enforcing one row per year. No `superseded_at`, no `id`.

### `ReconciliationSnapshot` aggregate

```go
type ReconciliationSnapshot struct {
    year         int
    generatedAt  time.Time
    snapshotJSON string
    pdf          []byte
}

func NewReconciliationSnapshot(year int, at time.Time, snapshotJSON string, pdf []byte) (ReconciliationSnapshot, error)
// Accessors: Year(), GeneratedAt(), SnapshotJSON(), Pdf()
```

Constructor rejects empty `snapshotJSON` and negative year (mirrors `NewReport`).

### `ReconciliationSnapshotRepository` port

```go
type ReconciliationSnapshotRepository interface {
    Save(ctx context.Context, s model.ReconciliationSnapshot) error
    FindByYear(ctx context.Context, year int) (model.ReconciliationSnapshot, bool, error)
}
```

`Save` is upsert (`INSERT ... ON CONFLICT(year) DO UPDATE SET generated_at=?, snapshot_json=?, pdf=?`). No `Delete`, no `MarkSuperseded`.

Added to `ports.RepoSet` as `ReconciliationSnapshots`.

### Snapshot serialization

Extend `internal/application/snapshot.go`:

```go
func ReconciliationSnapshotToJSON(rd services.ReconciliationData) (string, error)
func ReconciliationSnapshotFromJSON(s string) (services.ReconciliationData, error)
```

Plain `json.Marshal`/`Unmarshal`. `ReconciliationData` already has JSON tags from Phase 2.

## Application orchestration

Two new methods on `ReconciliationService`:

```go
// GenerateReport computes the year's reconciliation via ComputeReconciliation,
// serializes the snapshot, renders the PDF, and upserts the row. Returns the
// persisted aggregate. All I/O inside one TxManager.WithinTx.
func (s *ReconciliationService) GenerateReport(ctx context.Context, year int) (model.ReconciliationSnapshot, error)

// LatestSnapshot returns the stored snapshot for a year (empty if none).
func (s *ReconciliationService) LatestSnapshot(ctx context.Context, year int) (model.ReconciliationSnapshot, bool, error)
```

`GenerateReport` sequence:

1. Compute via `services.ComputeReconciliation(input)` (Phase 2, unchanged) — inside the tx.
2. Serialize via `application.ReconciliationSnapshotToJSON(rd)`.
3. Render PDF via injected `ports.ReconciliationRenderer.Render(rd, clock.Now())`.
4. Build the aggregate via `model.NewReconciliationSnapshot(year, at, json, pdf)`.
5. `Save` via `r.ReconciliationSnapshots.Save(ctx, agg)`.
6. Return the aggregate.

No window-state gate: reconciliation is a year-keyed overlay editable in any state (Phase 1 rule); regenerating the report follows the same rule.

Constructor of `ReconciliationService` gains two dependencies:
- `clock ports.Clock` — matches how `ForecastService` gets its clock (`clock.System{}` in wire.go).
- `renderer ports.ReconciliationRenderer` — the PDF renderer.

## Renderers

### Block layout builder

`internal/adapters/report/reconciliation_layout.go`:

```go
// buildReconciliationLayout consumes a computed ReconciliationData and emits
// the shared Block sequence rendered by both the PDF and Markdown writers.
func buildReconciliationLayout(rd services.ReconciliationData) []Block
```

For each category in `rd.Categories`, in order:

1. `SectionTitle(categoryHeader(cat))` — e.g. `"Despeses corrents (a2, a3, a4, a6)"`.
2. Category-summary `Table` — headers `Subtipus | Demanat | Concedit | Executat | Assignat | Desviació`. One row per subtype; totals row at the bottom with `Desviació neta` = `cat.NetDeviation`.
3. For each subtype in `cat.Subtypes`:
   1. `SectionTitle(st.Code + " — " + st.Label)`.
   2. Concessions `Table` — headers `Grup | Concepte | Demanat | Concedit | Executat | Assignat | Diferència`. One row per concession + totals row.
   3. For each concession in `st.Concessions`:
      - `Table` titled `cn.Concept + " (Grup " + cn.GroupCode + ")"`.
      - Headers: `Prevision | Soci | Concepte | Previst | Executat | Assignat | Estat`.
      - One row per forecast (member of the group).
      - Follow-up rows (indented, `Row.Bold = false`) for each invoice on that forecast: cells `"↳ " + issuer + " " + number + " (" + date + ")"`, blank soci, blank concepte, `linkedAmount`, blank, `paid?"✓":"✗"`.
4. `PageBreak` between categories.

### Status labels

Small helper in the layout file:

```go
func statusLabel(s services.ForecastReconStatus) string {
    switch s {
    case services.StatusFullyJustified:    return "Justificat"
    case services.StatusPartiallyJustified:return "Parcial"
    case services.StatusOverExecuted:      return "Sobre-executat"
    case services.StatusPaymentPending:    return "Pendent pagament"
    case services.StatusNoInvoice:         return "Sense factura"
    }
    return ""
}
```

### PDF renderer

`internal/adapters/report/reconciliation_pdf_renderer.go`:

```go
type ReconciliationPDFRenderer struct {
    BusinessName string
    LogoPath     string
}

func (r ReconciliationPDFRenderer) Render(rd services.ReconciliationData, generatedAt time.Time) ([]byte, error) {
    title := fmt.Sprintf("Conciliació d'ajuts %d", rd.Year)
    footer := generatedAt.Format("02/01/2006")
    return renderDocument(title, footer, r.BusinessName, r.LogoPath, buildReconciliationLayout(rd))
}
```

Reuses `renderDocument` verbatim — no maroto changes.

### Markdown renderer

`internal/adapters/report/reconciliation_markdown_renderer.go`:

```go
type ReconciliationMarkdownRenderer struct{}

func (ReconciliationMarkdownRenderer) Render(rd services.ReconciliationData) []byte
```

Same structure as the forecast Markdown renderer: `# Conciliació d'ajuts <year>\n\n`, then walk `buildReconciliationLayout(rd)` writing `## `-level titles, tables via the existing `writeMarkdownTable` helper, and `---\n\n` for page breaks.

### Exporter

`internal/adapters/report/reconciliation_exporter.go`:

```go
type ReconciliationExporter struct {
    PDFRenderer ports.ReconciliationRenderer   // used by ExportData path
    MDRenderer  ReconciliationMarkdownRenderer // MD is deterministic, no clock
}

// Export writes the persisted PDF (from rec.Pdf()) + a freshly rendered MD to
// outputDir. Returns the written file paths.
func (e ReconciliationExporter) Export(rec model.ReconciliationSnapshot, outputDir string) ([]string, error)

// ExportData is the live path (unused in Phase 3 — no preview branch — but
// kept for symmetry with the forecast exporter).
func (e ReconciliationExporter) ExportData(rd services.ReconciliationData, at time.Time, outputDir string) ([]string, error)
```

Output paths (overwrite any prior file):
- `<outputDir>/Conciliació ajuts <year>.pdf`
- `<outputDir>/Conciliació ajuts <year>.md`

Wired into `Deps.ReconciliationExporter`.

## TUI

Extends `[7] Admin`. Current keys after Phase 2's rename: `f` (informe de previsions), `p` (importa previsions), `c` (importa concessions), `b` (còpia), `r` (restaura). `g` is unused.

New key `g` — "genera informe de conciliació":

- `handleKey` gains `case "g": return p, generateReconciliationCmd(p.deps, p.year)`.
- `generateReconciliationCmd(deps, year)` calls `deps.Reconciliation.GenerateReport(ctx, year)` → `deps.ReconciliationExporter.Export(rec, deps.Cfg.OutputDir)` → returns `reconciliationGeneratedMsg{year, paths, err}`.
- `Update` handles the new message: on error → `adminResult{err}`; on success → `adminResult{text: "Informe de conciliació generat:\n  " + paths}`.
- `Actions()` gains `{Key: "g", Label: "genera informe de conciliació"}` after the `f` entry.
- `Detail()` dim hint gets `· g: conciliació` appended.

No window-state gate. Regenerating an existing year silently overwrites.

## Wire

`internal/wire/wire.go`:

- Instantiate renderers using existing config fields:
  - `pdf := report.ReconciliationPDFRenderer{BusinessName: cfg.BusinessName, LogoPath: cfg.LogoPath}`.
  - `md := report.ReconciliationMarkdownRenderer{}`.
  - `exp := report.NewReconciliationExporter(pdf, md)`.
- Instantiate repo: `persistence.NewReconciliationSnapshotRepository(q)`, add to `RepoSet.ReconciliationSnapshots`.
- Extend `ReconciliationService` constructor to accept `clock` and the renderer; pass them from wire.
- `Deps.ReconciliationExporter = exp`.

No config changes — `BusinessName`, `LogoPath`, `OutputDir` already exist for the forecast report and are reused.

## Testing

Six layers, no new fixtures beyond the existing `private/export-*.json`.

1. **Aggregate constructor** (`reconciliation_snapshot_test.go`) — happy path + rejects empty `snapshotJSON` + rejects negative year.

2. **Persistence round-trip** (`reconciliation_snapshot_repository_test.go`) — `Save` + `FindByYear` returns identical bytes; second `Save` for the same year overwrites (raw `SELECT count(*) = 1`); `FindByYear` for unknown year returns `(_, false, nil)`.

3. **Layout structure** (`reconciliation_layout_test.go`) — hand-built minimal `ReconciliationData` (one CURRENT category, one subtype, one concession, two forecasts, one paid + one unpaid invoice). Assert block sequence: SectionTitle → summary Table → subtype SectionTitle → concessions Table → per-concession Table → PageBreak. Assert specific header cells and total rows exist. Do NOT pin every string — check structure + key labels only.

4. **Renderer smoke tests** — PDF returns non-empty bytes starting with `%PDF-`; MD contains title `# Conciliació d'ajuts 2025`, a status label (`Justificat`), and one Money-formatted value.

5. **Application `GenerateReport`** — extend `reconciliation_service_test.go`:
   - Happy path: seed via existing `newReconWorld`, call `GenerateReport(ctx, 2025)`, assert non-empty `SnapshotJSON` + non-empty `Pdf`, `LatestSnapshot` returns the same aggregate.
   - Overwrite: call twice; assert only one row exists (via a direct sqlc `count` query), second `GeneratedAt >= first`.

6. **Golden 2025 end-to-end** — extend `TestReconciliation2025Fixture_ComputeMatchesWorkbook` to additionally call `GenerateReport(ctx, 2025)` after the `Compute` assertions. Assert:
   - `LatestSnapshot(ctx, 2025)` returns the persisted row.
   - The row's `SnapshotJSON` deserializes via `ReconciliationSnapshotFromJSON` back to `services.ReconciliationData` with the same per-subtype totals as the fresh `Compute`.
   - Pdf length > 0 and starts with `%PDF-`.
   - Skips with `t.Skip` when `private/export-*.json` is absent.

## Out of scope (Phase 3)

- **Server-side HTML view.** No `/reconciliation/{year}` handler, no template, no `HTMLRenderer`. Deferred to a possible Phase 4 or forever.
- **Snapshot history / `superseded_at`.** Overwrite semantics only. Adding history later is a schema-migration + repo change if needed.
- **PDF-content assertions** beyond magic bytes. Maroto internals aren't ours to pin.
- **Cross-language output.** Report language is Catalan (matches the rest of espigol).
- **`SplitKey` / `RepartimentKey`** — still deferred per Phase 1 §8.
- **Per-partner filtered exports.** A soci sees the same PDF as everyone else if they get one; the report is not personalised. Personalised views would live in a future server phase.
