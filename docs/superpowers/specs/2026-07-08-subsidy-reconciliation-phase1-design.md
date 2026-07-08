# Subsidy reconciliation — Phase 1: data model, persistence & ingestion

**Status:** design, awaiting implementation.
**Source analysis:** `private/espigol-subsidy-reconciliation-spec.md` (the "Ajuts 2025" workbook,
described in espigol terms — read it for the full business context and the Part B target).
**This spec covers Phase 1 only** — the new reconciliation entities, their persistence, and how
the grant/invoice data gets *in*. The reconciliation algorithm and the report output are separate
later phases (see Decomposition).

## Goal

espigol today stops at **Aprovat** (`ExpenseForecast.ApprovedAmount`). It has no model of what the
funder (Consorci Serra de Tramuntana) actually *granted*, nor of the real *invoices* that justify
the spend. Phase 1 introduces that missing data layer and two ways to populate it, using 2025 (now
a `CLOSED` window) as the concrete driving case. No reconciliation *math* and no report *output* are
built in this phase — only the entities, persistence, ingestion, and their validation.

## Decomposition (the whole effort, for context)

The source spec is three separable subsystems, each its own spec → plan → build cycle:

1. **Phase 1 (this spec)** — new entities + persistence + data ingestion (JSON import *and* TUI
   editing). Success: 2025's grant + 59 invoices + join links stored, round-tripped, and validated.
2. **Phase 2** — the reconciliation *algorithm* (`min(Granted, Executed)` capping, share proration
   with largest-remainder, payment-gating, category-net deviations), validated against the appendix
   cross-checks in the source spec.
3. **Phase 3** — the reconciliation *report* output: a `ReportData`-style tree feeding the existing
   snapshot → PDF·MD·HTML pipeline, plus the per-group / per-subtype / per-category roll-ups.

Chosen decomposition: **horizontal, model-first.** This spec is Phase 1.

## Decisions (from brainstorming)

- **Naming:** program in English; Catalan only for outputs/UI (matches the existing `boardMember`
  vs "Consell Rector" convention). `Grant`/`Group` are SQL reserved words, so the granted-subsidy
  entity is `Concession` and its id field is `GroupCode`.
- **Ingestion:** *both* JSON import (bulk, replace-all) *and* TUI editing panels, in this phase.
- **No window-state gate:** reconciliation data is an independent, year-keyed overlay, editable
  regardless of `DRAFT`/`OPEN`/`CLOSED`. (Deliberately unlike the forecast importer, which is gated
  to `OPEN`; the grant/invoices are inherently post-close.)
- **`RepartimentKey` / `SplitKey` deferred (YAGNI):** per-link euro shares are entered directly on
  `forecast_invoice.amount`; espigol does not recompute splits from physical quantities in Phase 1.
- **Payments carry amounts** (an invoice can be paid in several partial payments). The *gating rule*
  — an invoice justifies its spend **all-or-nothing, only once fully paid** (`Σ payments ==
  net_amount`) — is a Phase 2 concern; Phase 1 only stores the payment rows faithfully.

## Terminology map (source Catalan → English code/DB → Catalan UI label)

| Source (Catalan) | Go / DB (English)                     | Catalan UI label            |
|------------------|---------------------------------------|-----------------------------|
| `Concessió`      | `Concession` / `concession`           | "Concessió"                 |
| `Grup` (`A6-02`) | `GroupCode` / `group_code`            | "Grup"                      |
| `ConcessioForecast` | `ConcessionForecast` / `concession_forecast` | —                    |
| `Factura`        | `Invoice` / `invoice`                 | "Factura"                   |
| `factura_payment`| `InvoicePayment` / `invoice_payment`  | —                           |
| `ForecastInvoice`| `ForecastInvoice` / `forecast_invoice`| —                           |
| `Demanat`        | `RequestedTotal`                      | "Demanat"                   |
| `Concedit`       | `GrantedAmount`                       | "Concedit"                  |
| `Executat`       | `ExecutedAmount`                      | "Executat"                  |
| `Subvenció assignada` | `AssignedSubsidy` (Phase 2)      | "Subvenció assignada"       |
| `Desviació`      | `Deviation` (Phase 2)                 | "Desviació"                 |
| `RepartimentKey` | `SplitKey` (deferred)                 | —                           |

## 1. Schema — new migration `db/migrations/00003_reconciliation.sql`

All monetary columns are `TEXT` holding `Money` (scale-2, HALF_UP), consistent with the existing
schema. All tables are year-keyed; none reference window state.

```sql
CREATE TABLE concession (
    year            INTEGER NOT NULL,
    group_code      TEXT NOT NULL,              -- e.g. 'A6-02' (= source 'Grup')
    subtype_code    TEXT NOT NULL,
    concept         TEXT NOT NULL,
    requested_total TEXT NOT NULL,              -- Demanat
    granted_amount  TEXT NOT NULL,              -- Concedit
    PRIMARY KEY (year, group_code),
    FOREIGN KEY (year, subtype_code) REFERENCES expense_subtype(year, code)
);

CREATE TABLE concession_forecast (              -- bundle membership; one concession per forecast
    year        INTEGER NOT NULL,
    forecast_id TEXT NOT NULL,
    group_code  TEXT NOT NULL,
    PRIMARY KEY (year, forecast_id),
    FOREIGN KEY (year, group_code) REFERENCES concession(year, group_code),
    FOREIGN KEY (forecast_id) REFERENCES expense_forecast(id)
);

CREATE TABLE invoice (
    id         INTEGER PRIMARY KEY,
    year       INTEGER NOT NULL,
    issuer     TEXT NOT NULL,                   -- Proveïdor
    nif        TEXT NOT NULL,
    number     TEXT NOT NULL,                   -- supplier invoice number
    issue_date TEXT NOT NULL,                   -- Data factura
    net_amount TEXT NOT NULL,                   -- Import (sense IVA)
    file_path  TEXT,                            -- Arxiu (scanned pdf)
    notes      TEXT,
    UNIQUE (year, nif, number),
    FOREIGN KEY (year) REFERENCES submission_window(year)
);

CREATE TABLE invoice_payment (                  -- 0 rows = unpaid; N rows = split payments
    id         INTEGER PRIMARY KEY,
    invoice_id INTEGER NOT NULL,
    paid_on    TEXT NOT NULL,                   -- Data transfer.
    amount     TEXT NOT NULL,                   -- this payment's amount
    FOREIGN KEY (invoice_id) REFERENCES invoice(id) ON DELETE CASCADE
);

CREATE TABLE forecast_invoice (                 -- the M–N reconciliation truth
    forecast_id TEXT NOT NULL,
    invoice_id  INTEGER NOT NULL,
    amount      TEXT NOT NULL,                  -- this forecast's share of the invoice
    PRIMARY KEY (forecast_id, invoice_id),
    FOREIGN KEY (forecast_id) REFERENCES expense_forecast(id),
    FOREIGN KEY (invoice_id) REFERENCES invoice(id) ON DELETE CASCADE
);

CREATE INDEX idx_forecast_invoice_invoice ON forecast_invoice(invoice_id);
CREATE INDEX idx_invoice_payment_invoice ON invoice_payment(invoice_id);
CREATE INDEX idx_concession_forecast_group ON concession_forecast(year, group_code);
```

Modeling notes:
- **`concession_forecast` PK `(year, forecast_id)`** enforces *one concession per forecast* (each
  `CPYYnnn` sits under exactly one `Grup`, per the appendix). Kept as a separate join rather than a
  `group_code` column on `expense_forecast`, so reconciliation never leaks into the core forecast
  table.
- **`forecast_invoice`** is the true M–N: one invoice split across many forecasts (many rows, same
  `invoice_id`) and one forecast across many invoices (many rows, same `forecast_id`). The source
  workbook's per-line `Grup` is derivable via the forecast's `concession_forecast` membership, so it
  is not stored redundantly on the invoice.
- **`invoice`** uses a surrogate `id` (invoices are referenced by two child tables and split across
  many links). `UNIQUE (year, nif, number)` guards against accidental duplicate imports of the same
  physical invoice.

## 2. Domain model & ports

- **`internal/domain/model/`** — new immutable structs following the existing `forecast.go` style
  (unexported fields, constructors that validate, copy-methods, `Money` for all amounts, no setters):
  - `Concession` (`year`, `groupCode`, `subtypeCode`, `concept`, `requestedTotal`, `grantedAmount`).
  - `ConcessionForecast` (`year`, `groupCode`, `forecastID`) — a plain link value.
  - `Invoice` (`id`, `year`, `issuer`, `nif`, `number`, `issueDate`, `netAmount`, `filePath`,
    `notes`, plus its `[]InvoicePayment` and `[]ForecastInvoice` when loaded as an aggregate).
  - `InvoicePayment` (`id`, `invoiceID`, `paidOn`, `amount`).
  - `ForecastInvoice` (`forecastID`, `invoiceID`, `amount`).
- **`internal/domain/ports/`** — repository interfaces the application depends on:
  - `ConcessionRepository`: list/find/save/delete concessions and their forecast memberships, all
    year-scoped; `ReplaceForYear` for the import.
  - `InvoiceRepository`: list/find/save/delete invoices with their payments and forecast-invoice
    links; `ReplaceForYear` for the import.
  - Both added to `ports.RepoSet` so they participate in the existing `TxManager.WithinTx` unit of
    work.

## 3. Persistence

- **`db/queries/reconciliation.sql`** — sqlc query sources for all five tables; `make sqlc-generate`
  regenerates `internal/adapters/persistence/sqlc`.
- **Mappers + repositories** in `internal/adapters/persistence/`, mirroring the existing
  `forecast_repository.go` / mapper split (translate mutable sqlc rows ↔ immutable domain structs at
  the boundary). Register in `ports_check.go`.
- **`internal/wire/wire.go`** — assemble the new repositories into the application services for the
  **TUI** driver. The server (socis) does not touch reconciliation in Phase 1.

## 4. Ingestion A — JSON import (replace-all)

- **`internal/application/reconciliation_import.go`** (+ an importer under
  `internal/adapters/importer/`), following `forecast_import.go`: parse → **validate everything** →
  **replace the year's reconciliation data in a single transaction**, rolling back untouched on any
  error. No window-state check.
- **Replace-all semantics:** importing a year clears and re-inserts *all* of that year's
  `concession`, `concession_forecast`, `invoice`, `invoice_payment`, and `forecast_invoice` rows.
  Re-import to update (e.g. as payment dates/amounts arrive).
- **File format** (one file per year, e.g. `import/reconciliation-2025.json`):

```json
{
  "year": 2025,
  "concessions": [
    {
      "groupCode": "A6-02",
      "subtypeCode": "a6",
      "concept": "Adob orgànic",
      "requestedTotal": "13880.00",
      "grantedAmount": "13880.00",
      "forecastIds": ["CP25008", "CP25009"]
    }
  ],
  "invoices": [
    {
      "issuer": "Jardines Campaner",
      "nif": "B12345678",
      "number": "F878",
      "issueDate": "2025-03-14",
      "netAmount": "1234.56",
      "filePath": "B1-01-F1-250314-varies-maquines-campaner.pdf",
      "notes": "Varies màquines",
      "payments": [
        { "paidOn": "2025-04-01", "amount": "1234.56" }
      ],
      "links": [
        { "forecastId": "CP25030", "amount": "500.00" },
        { "forecastId": "CP25032", "amount": "734.56" }
      ]
    }
  ]
}
```

  - Concession membership is expressed as `forecastIds` inside each concession (drives
    `concession_forecast`). Invoice→forecast shares are the `links` (drive `forecast_invoice`).
  - All amounts are strings parsed as `Money`; dates are ISO `YYYY-MM-DD`.
  - An admin-panel key triggers the import (mirroring the forecast importer's `i`).

## 5. Ingestion B — TUI editing panels

A new top-level **"Ajuts"** panel (Catalan UI), reachable from the admin panel, with two views and a
JSON-import action. Built on the existing Bubble Tea panel/keymap/style infrastructure; the
`tui-design` skill guides layout during implementation.

- **Concessions view** — table (`Grup`, `Concepte`, subtype, `Demanat`, `Concedit`, #forecasts);
  add / edit / delete a concession and edit its forecast membership (assign/unassign `CPYYnnn`s).
- **Factures view** — table (`Proveïdor`, `Núm.`, `Data`, `Import (sense IVA)`, paid status,
  #links); add / edit / delete an invoice, manage its `Pagaments` (date + amount rows), and manage
  its per-forecast `Enllaços` (forecast + share `Import`).
- **Import action** — a key that runs the JSON import for the selected year.

Paid-status display derives from payments: *no pagat* (0 rows), *parcial* (`Σ < net`), *pagat*
(`Σ == net`).

## 6. Validation & integrity

Applies to **both** the JSON import and TUI saves.

- **Hard (fail the import / block the save):**
  - the `year` has a `submission_window`;
  - every `subtypeCode` exists in `expense_subtype(year, code)`;
  - every referenced `forecastId` (in concession membership and invoice links) exists in
    `expense_forecast` for that `year`;
  - `groupCode` unique within the year; each forecast in ≤1 concession (enforced by the PK);
  - `(year, nif, number)` unique per invoice;
  - all amounts and dates parse.
- **Soft (surfaced as warnings, non-fatal — the workbook's `OK?` columns):**
  - `granted_amount ≤ requested_total` per concession;
  - `requested_total ≈ Σ GrossAmount` of the concession's member forecasts (`Demanat` vs `Σ Previst`,
    within a `0,01 €` tolerance, matching the workbook's `ABS(diff) < 0.01` `OK?` test);
  - `Σ forecast_invoice.amount` for an invoice `≤ net_amount`;
  - `Σ invoice_payment.amount ≤ net_amount`.

## 7. Testing

- Repository round-trip tests (save → load equality) for concessions+membership and
  invoices+payments+links; mapper tests at the boundary.
- Import tests: happy path, each hard-validation failure rolls back with the year's prior data
  untouched, and soft-check warnings are reported without aborting.
- **2025 fixture load** (data-level only): importing the real 2025 reconciliation JSON yields the
  appendix totals — `Concedit = 84.096,91 €`, 59 invoices — and the bundle memberships
  (`A6-02 = CP25008 + CP25009`, `A6-04 = CP25015 + CP25016 + CP25017`, the `F878 → B1-01/03/10/13`
  and `FD-39521 → B1-08/09` one-invoice-many-forecasts cases). No allocation math is asserted here.

## 8. Out of scope (Phase 1)

- The reconciliation **algorithm** — `ExecutedAmount` per forecast, `min(Granted, Executed)` cap,
  share proration with largest-remainder, category-net deviations → **Phase 2**.
- **Payment-gating logic** — the data model stores payments with amounts; the rule that an invoice
  justifies its spend **all-or-nothing only once fully paid** (`Σ payments == net_amount`) is applied
  in **Phase 2**.
- Report **output** — `AssignedSubsidy`, the `ReportData`-style tree, PDF/MD/HTML, and the per-group
  / per-subtype / per-category roll-ups → **Phase 3**.
- `SplitKey` (physical-quantity split ratios) — deferred; shares are entered directly.
- Any server/socis-facing surface — TUI/admin only in Phase 1.
