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

## Admin panel

The admin TUI's `[7] Admin` panel acts on the year selected in the sidebar and offers five keys:

- `f` — generate the report for the selected year (PDF + Markdown into `~/.config/espigol/reports/`).
- `p` — import forecasts (previsions) for the selected year from a JSON file (see below). Requires the window to be **OPEN**.
- `c` — import concessions and invoices for the selected year from a JSON file (see [Subsidy reconciliation (Ajuts)](#subsidy-reconciliation-ajuts)). No window-state gate.
- `b` — back up the database to `~/.config/espigol/backups/espigol-<timestamp>.db`.
- `r` — restore the database: pick a backup from the list; the current database is backed up first and the chosen one is applied on the next launch.

### Importing forecasts

`i` reads `~/.config/espigol/import/<year>-forecasts.json` (e.g. `2025-forecasts.json` for 2025). The
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

### Subsidy reconciliation (Ajuts)

The `[6] Ajuts` panel manages concession requests and invoice reconciliation for the selected year. The panel
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

#### Current scope

This is **Phase 1** of the subsidy reconciliation feature. It manages concession and invoice data entry only.
The assignment algorithm (computing actual subsidies per forecast) and final report generation are planned for
later phases. Soft validation checks (e.g. payment sums, linked forecast totals) are shown as warnings during
import but do not reject the import.

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
