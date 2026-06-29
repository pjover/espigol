# Espígol (Go) — Phase 5: Reports (PDF + Markdown) — Design

**Status:** Approved for implementation · **Date:** 2026-06-29

Phase 5 of the Espígol Go rewrite. Real PDF and Markdown rendering of the computed
`ReportData` snapshot, replacing Phase 4's no-op `ReportRenderer`. Reuses espigol-cmd's
refined maroto layout, rebuilt on `report.ReportData`, with a shared renderer-agnostic
structure so PDF and MD have identical sections and tables. Parent:
`docs/superpowers/specs/2026-06-26-espigol-go-overview-design.md` (§6).

Authoritative layout reference: espigol-cmd
`internal/domain/services/reports/` (`report_pdf.go`, `custom_table_sub_report.go`,
`expense_forecast_report_service.go`). Golden output (numbers reference only):
`espigol-java/private/report-examples/Previsions de despeses 2026.{pdf,md}`.

---

## 1. Decisions (resolved during brainstorming)

| Decision | Choice |
|---|---|
| maroto version | **v1 (v0.33.0)** — reuse the espigol-cmd layout. Verified: builds on Go 1.26, **CGO-free**. |
| Render/export split | **`ReportRenderer` (maroto PDF) → BLOB** (used by Close, in the tx, unchanged); a separate **`ReportExporter`** writes files out-of-tx (PDF BLOB → file, MD rendered → file). |
| PDF ↔ MD structure | **One shared renderer-agnostic `Block` layout**, built once from `ReportData`; PDF and MD both render the same blocks → identical sections/tables. |
| Markdown source | New code (espigol-cmd has no MD generator). MD mirrors the **PDF structure** (not the golden MD layout). The golden MD is a numbers reference only. |
| Final summary | A final **`Resum`** section (per-category summary: Import disponible / Comú / Seccions / Socis / Total / Import remanent, Previst + Aprovat) appended after both categories' detail. |

---

## 2. Architecture

All in `internal/adapters/report` (package `report`, beside the existing no-op renderer).

- **maroto v1 scaffolding** — copied from espigol-cmd and adapted to render to **`[]byte`**
  (maroto's in-memory `Output()`, not `OutputFileAndClose`): a `ReportPdf` with logo +
  business-name header and a date footer.
- **Shared layout** (`layout.go`): a renderer-agnostic block model + `buildLayout`.
- **`PDFRenderer`** (`pdf_renderer.go`): implements `ports.ReportRenderer`
  (`Render(rd report.ReportData, generatedAt time.Time) ([]byte, error)`); maps blocks to
  maroto. Holds business name + logo path (from config).
- **`MarkdownRenderer`** (`markdown_renderer.go`): `Render(rd report.ReportData) []byte`;
  maps the same blocks to Markdown.
- **formatting** (`format.go`): `formatEuro(model.Money) string` → `"1.234,56 €"`; Catalan
  category labels (`Despesa corrent` / `Despesa d'inversió`).
- **`ReportExporter`** (`exporter.go`): writes the stored PDF BLOB + the rendered MD to the
  output dir.

**No change to `WindowService`/`Close`.** Phase 5 delivers these adapters + tests. Wiring
(swap the no-op for `PDFRenderer` in Close's construction; call `ReportExporter` after
close) lands with the TUI/server (Phase 6/7). The no-op renderer remains for the
`WindowService` tests until then.

### 2.1 Shared block model (`layout.go`)

```
type Block interface{}              // sealed: one of the below

type SectionTitle struct { Text string }

type Row struct {
    Cells []string                 // Money already formatted (e.g. "1.234,56 €")
    Bold  bool
    Red   bool
}
type Table struct {
    Title   string
    Headers []string
    Widths  []uint                 // maroto grid units (sum 12); MD ignores
    Rows    []Row
}

type PageBreak struct{}

func buildLayout(rd report.ReportData, generatedAt time.Time) []Block
```

`buildLayout` walks `ReportData` once and emits the ordered blocks (§3). Both renderers
consume the result, so structure/formatting are identical by construction.

---

## 3. Report structure (the block sequence)

`buildLayout` emits, **for each category** in `rd.Categories` (`Despesa corrent`, then
`Despesa d'inversió`), the espigol-cmd sub-report sequence sourced from `ReportData`:

1. **Common table** — from `cat.Common.Items` (`Àmbit | Concepte | Brut`); `Total comú` =
   `cat.Common.Total`.
2. **Sections table** — iterate `cat.Sections.SectionDetails` (each section's label, items,
   total); `Total seccions` = `cat.Sections.Total`.
3. **Remainder summary** — `Disponible any <year>`, `Total comú`, `Disponible per seccions`
   (= `cat.Sections.Available`), `Total seccions`, category total, `Remanent` (=
   `cat.Sections.Remainder`).
4. **Warning table** — only if `cat.Warning != nil`: one red row per `SectionWarning`
   (section label, producers, allowed, adjustment), `⚠ AVÍS` title.
5. **Partners table** — `cat.Sections.Partners.SubtypeTotals` (subtype → amount); `Total
   socis` = `GrandTotal`; if `HasExcess`, a per-partner adjustment table from `Allocations`
   (`Soci | Sol·licitat | Assignat`, capped rows red); else a `Remanent final` row (=
   `FinalRemainder`).
6. **`Detall per secció i soci`** section title, then **per-scope detail** tables (common +
   each section: `CP | Concepte | Brut`) and **per-partner detail** tables from
   `PartnerDetails` (`CP | Concepte | Brut`; an `Import màxim autoritzat` red row when
   `IsCapped`). CP code = `DetailItem.CpCode`.
- `PageBreak` between categories (not after the last).

Then a **final `Resum` section** (`SectionTitle "Resum"`), with one summary table per
category (`Despesa corrent`, `Despesa d'inversió`), columns **Concepte | Previst | Aprovat**,
bold rows:

- **Import disponible** = `Common.Available` (limit)
- **Comú** = `Common.Total`
- **Seccions** = `Sections.Total`
- **Socis** — Previst = `Partners.GrandTotal`; Aprovat = Σ `Partners.Allocations[].Allocated`
- **Total** = Comú + Seccions + Socis (per column)
- **Import remanent** = limit − Total (per column)

Comú/Seccions are approved = gross, so their Previst == Aprovat; only Socis can differ under
capping. All amounts via `formatEuro`; structural labels Catalan verbatim.

---

## 4. Renderers

### 4.1 PDFRenderer (maroto v1)

`Render(rd, generatedAt) ([]byte, error)`: title `Previsions de despeses <year>`, footer =
`generatedAt` formatted `02/01/2006`; build `buildLayout(rd, generatedAt)` and map each
block to maroto — `Table` → a `CustomTableSubReport` (using `Widths`, bold/red per row),
`SectionTitle` → heading, `PageBreak` → `AddPage`. Header shows the logo (if the file
exists) + business name (from config). Returns maroto `Output()` bytes (the BLOB).

### 4.2 MarkdownRenderer

`Render(rd) []byte`: `# Previsions de despeses <year>`, then map the same blocks —
`SectionTitle` → `##`, `Table` → its `Title` as `###` + a GitHub-style `| … |` table
(bold via `**…**`; the red flag is rendered as bold in MD), `PageBreak` → `---`. Identical
sections/tables/numbers to the PDF.

### 4.3 ReportExporter

`Export(rep model.Report, outputDir string) error` (no PDF re-render — reuses the BLOB):
1. Expand a leading `~` in `outputDir`; `os.MkdirAll(outputDir, 0o755)`.
2. Write `rep.Pdf()` → `<outputDir>/Previsions de despeses <rep.Year()>.pdf`.
3. Deserialize the snapshot **in the adapter itself** —
   `var rd report.ReportData; json.Unmarshal([]byte(rep.SnapshotJSON()), &rd)` (lossless,
   since `Money` has `UnmarshalJSON`) — then write `markdownRenderer.Render(rd)` →
   `<outputDir>/Previsions de despeses <rep.Year()>.md`.

The `report` adapter deserializes the snapshot directly (imports `encoding/json` + the
domain `report` package) rather than calling `application.SnapshotFromJSON`, so the adapter
layer does **not** depend on the application layer. The caller (TUI, Phase 7) fetches the
latest `Report` for a year (`ReportRepository.FindLatestByYear`) and passes it in;
`outputDir` comes from config.

---

## 5. Testing (TDD)

- **`formatEuro`** unit test: `1234.56→"1.234,56 €"`, `0→"0,00 €"`, `-9→"-9,00 €"`,
  `31900→"31.900,00 €"`, `1322.22→"1.322,22 €"`.
- **`buildLayout`** test: run `services.Compute` on the Phase-3 **anonymized golden 2026
  fixture**, build the layout, assert both categories present, the expected tables exist, and
  the final `Resum` tables carry the golden numbers EU-formatted (`2.880,00 €`,
  `27.111,00 €`, `23.498,96 €`, `11.203,04 €`, remanent `9,00 €`).
- **MarkdownRenderer** test: render that `ReportData`; assert the `Resum` heading, category
  headings, well-formed `| … |` tables, and the golden EU-formatted totals.
- **PDFRenderer** smoke test: render that `ReportData`; assert non-empty `[]byte`, no error,
  `%PDF` header prefix. (No byte-parity with the golden PDF — different maroto build/fonts.)
- **ReportExporter** test: build a `model.Report` (BLOB = bytes, snapshot = golden
  `ReportData` JSON), `Export` to a temp dir; assert both files exist, the `.pdf` bytes equal
  the BLOB exactly, the `.md` contains expected content.

The anonymized fixture keeps tests self-contained with no cooperative data; Phase 3 owns the
numeric validation.

---

## 6. Scope

**In Phase 5:** the shared `Block` layout + `buildLayout`; `PDFRenderer` (maroto v1,
implements `ports.ReportRenderer`); `MarkdownRenderer`; `formatEuro` + Catalan labels;
`ReportExporter`; the maroto scaffolding (from espigol-cmd) adapted to `[]byte`.

**Not in Phase 5:** wiring the real `PDFRenderer` into `Close` and calling `ReportExporter`
from the TUI (Phase 7); the HTML report view (Phase 6 server); auto-close/notifications
(deferred). The no-op renderer stays until Phase 7 wiring.

---

## 7. References

- Overview: `docs/superpowers/specs/2026-06-26-espigol-go-overview-design.md` (§6).
- Phase 3: `…/2026-06-28-espigol-go-phase-3-allocation-design.md` (`report.ReportData`).
- Phase 4: `…/2026-06-28-espigol-go-phase-4-window-close-design.md` (`ReportRenderer` port,
  `SnapshotFromJSON`, Close BLOB).
- Layout reference: espigol-cmd `internal/domain/services/reports/`.
- Numbers reference: `espigol-java/private/report-examples/Previsions de despeses 2026.{pdf,md}`.
