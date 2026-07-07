# espigol

TUI and server for managing the annual subsidy budget of the Cooperativa d'Estellencs, a small agricultural cooperative on the island of Mallorca.

- `espigol` launches the admin TUI.
- `espigol --server` launches the socis web server.

Configuration and data live in `~/.config/espigol` by default, overridable with `$ESPIGOL_HOME`.

See `docs/superpowers/specs/` for the design and `docs/superpowers/plans/` for the phased implementation plans.

## Admin panel

The admin TUI's `[6] Admin` panel acts on the year selected in the sidebar and offers four keys:

- `f` — generate the report for the selected year (PDF + Markdown into `~/.config/espigol/reports/`).
- `i` — import forecasts for the selected year from a JSON file (see below).
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
