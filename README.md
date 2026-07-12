# espigol

TUI and server for managing the annual subsidy budget of the Cooperativa d'Estellencs, a small agricultural cooperative on the island of Mallorca.

- `espigol` launches the admin TUI.
- `espigol --server` launches the socis web server.

Configuration and data live in `~/.config/espigol` by default, overridable with `$ESPIGOL_HOME`.

`$ESPIGOL_HOME/config.yaml` example (all keys are optional — defaults shown):

```yaml
business:
  name: "Cooperativa d'Estellencs"

server:
  port: 8080

# Relative to $ESPIGOL_HOME.  Omit to use the defaults below.
output:
  dir: "reports"      # → $ESPIGOL_HOME/reports
backup:
  dir: "backups"      # → $ESPIGOL_HOME/backups
logo:
  path: "logo.png"    # → $ESPIGOL_HOME/logo.png

oauth:
  client_id: ""
  client_secret: ""
  redirect_url: ""

admin:
  email: "admin@espigol"
```

All keys can be overridden with `ESPIGOL_<KEY>` environment variables
(e.g. `ESPIGOL_SERVER_PORT=9090`, `ESPIGOL_ADMIN_EMAIL=admin@example.org`).

See `docs/superpowers/specs/` for the design and `docs/superpowers/plans/` for the phased implementation plans.

## Using the admin TUI

Launch with `espigol` (typically over SSH on the VPS, against the live database).

- `1`–`7` — jump directly to a panel.
- `q` / `Ctrl+C` — quit.
- Most panels operate on the year selected in the **`[1] Anys`** panel; switching the selected year there updates what every other panel shows.

The sidebar lists seven panels:

| # | Panel | Purpose |
|---|-------|---------|
| 1 | [Anys](#1-anys-years) | Create years, open/close/reopen the submission window, select the active year |
| 2 | [Socis](#2-socis-partners) | Manage cooperative members |
| 3 | [Seccions](#3-seccions-sections) | Manage crop/activity sections (e.g. olive, vineyard) |
| 4 | [Taxonomia](#4-taxonomia-expense-types--subtypes) | Manage the year's expense type/subtype taxonomy |
| 5 | [Previsions](#5-previsions-forecasts) | Manage partners' forecasted expenses for the year |
| 6 | [Ajuts](#6-ajuts-subsidy-reconciliation) | Reconcile subsidy concessions and invoices against forecasts |
| 7 | [Admin](#7-admin) | Generate reports, import data, back up/restore the database |

### [1] Anys (years)

- `↑`/`↓` — select year.
- `n` — create a new year.
- `o` — open the year's submission window (or **reopen** it, if it's currently CLOSED).
- `c` — close the year's submission window (confirmation required).
- `e` — edit the year's deadline/limits. Only while the window is not CLOSED.
- `r` — generate the forecast report for the selected year (same as the Admin panel's `f`, see below).

A year's window moves through **DRAFT → OPEN → CLOSED**; only one window is OPEN at a time.

### [2] Socis (partners)

- `↑`/`↓` — select partner.
- `n` — new partner.
- `e` — edit the selected partner.
- `b` — toggle whether the selected partner is a board member ("junta").
- `m` — manage which sections the selected partner belongs to.

### [3] Seccions (sections)

- `n` — new section.
- `e` — edit the selected section.

### [4] Taxonomia (expense types & subtypes)

Defines the year's expense categories. Only editable while the year's window is **DRAFT** (locked once OPEN).

- `n` — new subtype.
- `t` — new type.
- `e` — edit the selected type/subtype.
- `d` — delete the selected type/subtype.

### [5] Previsions (forecasts)

Lists the selected year's forecasted expenses.

- `n` — new forecast.
- `e` — edit the selected forecast.
- `d` — delete the selected forecast.

Bulk import of forecasts from a JSON file is done from the **Admin** panel's `p` key — see [Importing forecasts](#importing-forecasts) below.

### [6] Ajuts (subsidy reconciliation)

The Ajuts panel manages concession requests and invoice reconciliation for the selected year. The panel
operates in two views (toggled by `tab`): **Concessions** (subsidy requests grouped by partner/activity) and
**Factures** (invoices linked to those concessions). All reconciliation data can be imported, created, edited,
and deleted without any window-state gate, independent of the forecast submission process.

Available keys:

- `tab` — toggle between Concessions and Factures views.
- `i` — import reconciliation data from a JSON file (see below).
- `n` — create a new concession or invoice (depends on active view).
- `e` — edit the selected concession or invoice.
- `d` — delete the selected concession or invoice.

#### Importing reconciliation data

`i` reads `~/.config/espigol/import/reconciliation-<year>.json` (e.g. `reconciliation-2025.json` for 2025).
The import is **replace-all**: every existing concession and invoice for that year is deleted and replaced by
the file's contents. All referenced forecast IDs (`forecastIds`) and subtypes (`subtypeCode`) must already
exist for the year, or the whole import is rejected and nothing changes.

Each concession groups a subsidy request with linked forecasts. Amounts are decimal strings. Linked forecasts
are comma-separated (`CP25008,CP25009`). Each invoice may have multiple payments (date-amount pairs) and links
to forecasts (forecast-ID + assigned amount pairs).

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
      "filePath": "f878.pdf",
      "notes": "Varies màquines",
      "payments": [
        { "paidOn": "2025-04-01", "amount": "1234.56" }
      ],
      "links": [
        { "forecastId": "CP25008", "amount": "500.00" },
        { "forecastId": "CP25009", "amount": "734.56" }
      ]
    }
  ]
}
```

#### TUI form conventions

When creating or editing concessions and invoices via `n` and `e`:

- **Previsions** (concession forecasts) — comma-separated CP-IDs (`CP25008,CP25009`).
- **Pagaments** (invoice payments) — semicolon-separated date-amount pairs (`2025-04-01:500.00;2025-04-15:734.56`).
- **Enllaços** (invoice forecast links) — semicolon-separated forecast-ID–amount pairs (`CP25008:500.00;CP25009:734.56`).

All amounts use decimal notation (e.g. `1234.56`).

#### Reconciliation computation

Once concessions and invoices are entered, espigol computes, per forecast, how much of the granted subsidy is
actually assigned based on linked, paid invoices — and assigns one of five statuses:

| Status (Catalan) | Meaning |
|---|---|
| Justificat | Fully justified: linked invoice amounts cover the forecast |
| Parcial | Partially justified: some but not all of the forecast is covered |
| Sobre-executat | Over-executed: linked amounts exceed the forecast |
| Pendent pagament | Linked invoices exist but haven't been fully paid yet |
| Sense factura | No invoice has been linked to the forecast yet |

This computation feeds the reconciliation report generated by the Admin panel's `g` key (below) — the
per-forecast status shown there is exactly this.

### [7] Admin

The Admin panel acts on the year selected in the sidebar and offers six keys:

- `f` — generate the forecast report for the selected year (PDF + Markdown into `~/.config/espigol/reports/`).
- `g` — generate the subsidy reconciliation report for the selected year (PDF + Markdown into
  `~/.config/espigol/reports/Conciliació ajuts <year>.{pdf,md}`). Unlike `f`, this has **no window-state
  gate** — it can be regenerated at any time, in any window state, and always reflects the current
  concessions/invoices data. Re-generating overwrites the previous report for that year (one stored snapshot
  per year).
- `p` — import forecasts (previsions) for the selected year from a JSON file (see below). Requires the window to be **OPEN**.
- `c` — import concessions and invoices for the selected year from a JSON file (see [Ajuts](#6-ajuts-subsidy-reconciliation)). No window-state gate.
- `b` — back up the database to `~/.config/espigol/backups/espigol-<timestamp>.db`.
- `r` — restore the database: pick a backup from the list; the current database is backed up first and the chosen one is applied on the next launch.

#### Importing forecasts

`p` reads `~/.config/espigol/import/<year>-forecasts.json` (e.g. `2025-forecasts.json` for 2025). The
year's submission window must be **OPEN**. The import is **replace-all**: every existing forecast for
that year is deleted and replaced by the file's contents. The referenced partner (`partnerId`), expense
subtype (`subtypeCode`), and — for `SECTION` scope — the section (`sectionCode`) must already exist, or
the whole import is rejected and nothing changes.

`scope` is one of `COMMON`, `SECTION`, or `PARTNER`; `sectionCode` is required only for `SECTION` (and
must be empty otherwise). `grossAmount` is a decimal string and `plannedDate` is `YYYY-MM-DD`. Imported
forecasts start unapproved.

```json
{
  "year": 2025,
  "forecasts": [
    { "partnerId": 7, "scope": "COMMON",  "sectionCode": "",      "subtypeCode": "a1", "concept": "Assegurança collita", "description": "", "grossAmount": "2880.00",  "plannedDate": "2025-06-15" },
    { "partnerId": 1, "scope": "SECTION", "sectionCode": "oliva", "subtypeCode": "a1", "concept": "Poda",                "description": "", "grossAmount": "1200.00",  "plannedDate": "2025-03-01" },
    { "partnerId": 3, "scope": "PARTNER", "sectionCode": "",      "subtypeCode": "b1", "concept": "Tractor",             "description": "", "grossAmount": "31900.00", "plannedDate": "2025-09-01" }
  ]
}
```

## Deployment

`make dist` cross-compiles static, CGO-free binaries for `linux/amd64`, `linux/arm64`,
and `darwin/arm64` into `dist/<os>-<arch>/espigol`. The typical update flow on a VPS is:

```bash
make dist
scp dist/linux-amd64/espigol root@your-vps:/usr/local/bin/espigol
ssh root@your-vps 'systemctl restart espigol-server'
```

`espigol --server` is meant to run under `systemd`, behind Caddy for TLS, with the
admin TUI run over SSH against the same database. See
[`docs/ops/DEPLOY.md`](docs/ops/DEPLOY.md) for the full provisioning runbook and
[`deployments/`](deployments/) for the systemd units, Caddy config, and backup script.
