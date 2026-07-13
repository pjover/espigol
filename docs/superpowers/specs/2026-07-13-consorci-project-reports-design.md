# Consorci project reports (Markdown) — design

**Date:** 2026-07-13
**Status:** Approved (pending spec review)

## Context

Each year the cooperative applies to the Consorci Serra de Tramuntana subsidy
programme by submitting two documents, currently produced by hand:

- **E — Projecte d'actuació**: a narrative describing the cooperative, its
  objectives and the concrete activities it seeks funding for, grouped by
  expense apartat (subtype). Reference: `private/E-Projecte d_actuació 2025.pdf`.
- **F1 — Pressupost del projecte d'actuació**: the budget for those activities —
  a summary table by tipus/apartat and detailed breakdowns per concept.
  Reference: `private/F1-Pressupost del projecte d_actuació 2025.pdf`.

We want Espígol to generate both as **Markdown skeletons** from the year's
forecast data, so the administrator edits them (especially the narrative) and
submits them to the Consorci. Crucially, every line must carry the forecast
**CP code(s)** so the Consorci can later track each invoice/expense against the
initiative it belongs to.

These are distinct from the existing "Previsions de despeses" (allocation) and
"Conciliació d'ajuts" (reconciliation) reports; they serve the *application*
phase and are produced from live forecast data in any window state.

## Decisions (from brainstorming)

- **CP granularity**: group forecasts by concept name within an apartat; sum the
  amounts; **list all contributing CP codes** on the line (a line may carry
  several CPs, e.g. `Adob orgànic — CP25006, CP25007 — 13.880,00 €`).
- **Report 1 (Projecte)**: full skeleton — editable placeholders for the intro
  and every Objectius apartat, plus a **fully populated Activitats** section.
  Activity lines read `Concept (CP…)` with **no amount** (amounts live in the
  budget).
- **Report 2 (Pressupost)**: budget tables mirroring the reference (Resum per
  tipus + per-concept desglossament for corrents and inversions), with a **CP
  column** and summed amounts.
- **Trigger**: one new **Admin key `p`** ("documents Consorci") generating both
  `.md` files for the **selected year** from live data, shown in the result
  modal. Footer becomes `h · i · j · k · p · c · r`.
- **Filenames** (in the reports output dir): `Projecte d'actuació <year>.md` and
  `Pressupost del projecte d'actuació <year>.md`.

## Scope / data rules

- Source: **enabled** forecasts for the selected year (`ForecastService.ListByYear`,
  filtered by `Enabled()`), **all scopes** (common/section/partner) summed
  together — the budget is the total requested amount.
- Taxonomy: `TaxonomyService.ListTypes` / `ListSubtypes` for the year give the
  apartat labels and the tipus (A=CURRENT / B=INVESTMENT) each subtype belongs to.
- **Ordering**: tipus CURRENT (A) then INVESTMENT (B); apartats by subtype code
  (`a2, a3, a4, a6, b1, b2`); concepts alphabetical within an apartat; CP codes
  sorted ascending within a concept line.
- **Label formatting**: apartat = `a2` → `a.2. <subtype label>`; tipus =
  `A. <type label>` (e.g. `A. Despeses corrents`). Only apartats/tipus that have
  forecasts appear.
- Money uses the app's standard format `1.234,56 €` (`formatEuro`). Never
  `float64`; sums via `model.Money`.

## Architecture

One shared computation feeds two renderers (same pattern as the reconciliation
report):

```
ForecastService.ListByYear + TaxonomyService.List{Types,Subtypes}
        │  (application: ProjecteService.Compute)
        ▼
services.ComputeProjecte(forecasts, types, subtypes) -> services.ProjecteData   (pure domain)
        │
        ├── report.ProjecteActuacioRenderer.Render(data) -> []byte   (narrative skeleton)
        └── report.PressupostRenderer.Render(data)        -> []byte   (budget tables)
        │  (report.ProjecteExporter.Export -> writes both .md, returns paths)
        ▼
TUI Admin 'p' -> generateProjecteCmd -> info modal with the two paths
```

### `services.ProjecteData` (domain, pure)

Defined in `internal/domain/services/projecte.go`, mirroring how
`ReconciliationData` lives in the services package:

```go
type ProjecteData struct {
    Year   int
    Tipus  []TipusProjecte // ordered A then B
    Total  model.Money     // grand total
}
type TipusProjecte struct {
    Code, Label string       // "A", "Despeses corrents"
    Apartats    []ApartatProjecte
    Total       model.Money
}
type ApartatProjecte struct {
    Code, Label string       // "a2", "Activitats d'informació..."
    Concepts    []ConcepteProjecte
    Total       model.Money
}
type ConcepteProjecte struct {
    Name  string
    CPs   []string           // sorted forecast ids, e.g. ["CP25006","CP25007"]
    Total model.Money
}
```

`ComputeProjecte` groups enabled forecasts by subtype → concept, sums amounts,
collects+sorts CPs, resolves apartat/tipus labels+category from the taxonomy,
and orders everything as above. Empty apartats/tipus are omitted. It is I/O-free.

### Renderers (`internal/adapters/report`)

- `projecte_actuacio_renderer.go` — emits Report 1: `# Projecte d'actuació <year>`,
  intro placeholder, `## Objectius` with a placeholder subheading per apartat,
  `## Activitats` with a subheading per apartat and one `- Concept (CP…)` bullet
  per concept, then the signature block. No amounts.
- `pressupost_renderer.go` — emits Report 2: `# Pressupost…`, a short fixed intro
  sentence, `## Resum per tipus de despesa` (Tipus | Apartat | Brut, with bold
  per-tipus totals and a grand total), then `## Desglossament per conceptes de
  despeses corrents` and `## Desglossament per conceptes d'inversions`. Each
  desglossament has one table per apartat (Concepte | CP | Brut, bold apartat
  total) and ends with a bold section total (`Total general` for that tipus),
  matching the reference. The two desglossament sections are omitted if their
  tipus has no forecasts.

Both reuse `formatEuro` from `format.go`. Pure functions of `ProjecteData`.

### Exporter / application / wiring

- `report.ProjecteExporter{}` — `Export(data ProjecteData, outputDir string) ([]string, error)`
  writes `Projecte d'actuació <year>.md` and `Pressupost del projecte d'actuació
  <year>.md`, returns their paths (mirrors `ReconciliationExporter`).
- `application.ProjecteService.Compute(ctx, year) (services.ProjecteData, error)`
  — reads forecasts + taxonomy within a tx and calls `ComputeProjecte`.
- `wire.TUI` adds `ProjecteService` + `ProjecteExporter` to `tui.Deps`.
- `panel_admin.go`: new `case "p"` → `generateProjecteCmd(deps, year)` → compute,
  export, `resultModalCmd(text, nil)`; add the `{Key:"p", Label:"documents Consorci"}`
  action and the Detail() hint.

## Report layouts (concrete, from 2025 data)

### Report 1 — `Projecte d'actuació 2025.md`

```markdown
# Projecte d'actuació 2025

_[Introducció: convocatòria, BOIB, descripció de la cooperativa i activitats sol·licitades.]_

## Objectius

### a.2. Activitats d'informació i promoció de productes agraris
_[Objectius d'aquest apartat]_
…(one placeholder heading per apartat that has forecasts)…

## Activitats

### a.6. Despeses de fertilitzants, productes d'alimentació animal
- Adob foliar (CP25005)
- Adob orgànic (CP25006, CP25007)
- Alfals (CP25012)
…

_Estellencs, en data de la signatura._

_Pere Jover Casasnovas, President_
```

### Report 2 — `Pressupost del projecte d'actuació 2025.md`

```markdown
# Pressupost del projecte d'actuació 2025

El pressupost de les actuacions per a les quals es demana subvenció és el següent:

## Resum per tipus de despesa

| Tipus | Apartat | Brut |
| --- | --- | ---: |
| A. Despeses corrents | a.2. Activitats d'informació i promoció… | 8.189,00 € |
|  | a.6. Despeses de fertilitzants… | 19.551,73 € |
| **Total A. Despeses corrents** |  | **29.860,73 €** |
| B. Despeses d'inversió | b.1. Despeses d'adquisició de maquinària… | 52.470,06 € |
|  | b.2. Manteniment i restauració… | 3.270,60 € |
| **Total B. Despeses d'inversió** |  | **55.740,66 €** |
| **Total general** |  | **85.601,39 €** |

## Desglossament per conceptes de despeses corrents

### a.6. Despeses de fertilitzants, productes d'alimentació animal
| Concepte | CP | Brut |
| --- | --- | ---: |
| Adob foliar | CP25005 | 1.488,73 € |
| Adob orgànic | CP25006, CP25007 | 13.880,00 € |
| **Total a.6.** |  | **19.551,73 €** |

## Desglossament per conceptes d'inversions
…(one table per B apartat)…
```

## Testing

- **Domain** (`projecte_test.go`): fixture of a few forecasts spanning
  subtypes/concepts (incl. two forecasts sharing a concept) → assert grouping,
  summed amounts, sorted CP lists, apartat/tipus ordering, and totals.
- **Renderers**: assert each document contains the expected headings, CP codes,
  and totals (style of `reconciliation_markdown_renderer_test.go`).
- **Exporter + Admin**: pressing `p` writes both files with the correct names and
  opens the info modal (style of the `i`/reconciliation admin test).
- **End-to-end (manual)**: generate against a scratch copy of the 2025 DB and
  confirm the budget totals match the reference PDFs — corrents **29.860,73 €**,
  inversió **55.740,66 €**, general **85.601,39 €**.

## Out of scope

- No PDF output (Markdown only, by request).
- No new persistence/snapshot; reports are generated live from current forecasts.
- The intro and Objectius prose are not generated — they remain editable
  placeholders the administrator fills each year.
- The reference E-Projecte's thematic split of a6 into "Alimentació animal" +
  "Fertilitzants" is not reproduced; grouping is strictly by apartat (subtype),
  and the administrator re-splits prose if desired.
```
