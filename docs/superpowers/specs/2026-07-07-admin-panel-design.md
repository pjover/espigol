# Espígol — Admin Panel (import + backup/restore) — Design

**Status:** Approved for implementation · **Date:** 2026-07-07

Repurpose the sixth TUI panel from **Informes** (report-only) into a broader **Admin** panel that
generates the year report, bulk-imports forecasts from a JSON file, and backs up / restores the
SQLite database. All actions operate on the selected-year context that the rest of the TUI already
uses (the sidebar `Any: N`).

---

## 1. Decisions

| Decision | Choice |
|---|---|
| Panel identity | `reportsPanel` → `adminPanel`; title `Informes` → `Admin` (sidebar shows `[6] Admin`) |
| Report key | `r` → **`f`** (same behaviour: report for the selected year, any state) |
| Import key | **`i`** — import `Home/import/<year>-forecasts.json` for the selected year |
| Backup key | **`b`** — `VACUUM INTO Home/backups/espigol-<ts>.db` |
| Restore key | **`r`** — pick a backup → stage → apply on next launch |
| Target year | Selected-year context for every action; **import requires the year's window OPEN** |
| Import re-run | **Replace-all**: delete the year's existing forecasts, then insert the file's (idempotent) |
| Import atomicity | One `WithinTx`; any validation failure rolls back and leaves existing forecasts untouched |
| Missing references | Partner / subtype / section (and file-year mismatch, non-OPEN window) → error, nothing written |
| Scope tokens (JSON) | Canonical model values `COMMON` / `SECTION` / `PARTNER` |
| Backup mechanism | SQLite `VACUUM INTO` (WAL-safe, pure-Go, no external tools) |
| Backup retention | Keep all (prune-to-last-N deferred) |
| Restore model | Staged, applied on restart; current DB auto-backed-up first |
| Restore selection | Pick from a list of `backups/` files (newest first) |
| Restore choke point | `db.Open` applies a pending restore before opening (covers `espigol` and `--server`) |
| Audit | No audit rows for backup/restore (a restore wipes them); import audits per created/deleted forecast |
| Context list | Admin panel keeps the "Anys amb informe desat" list |

---

## 2. Panel

`internal/adapters/tui/panel_reports.go` → `panel_admin.go`; type `reportsPanel` → `adminPanel`;
constructor `NewReportsPanel` → `NewAdminPanel` (updated in `internal/wire/wire.go`). `Title()`
returns `"Admin"`.

### 2.1 Keybindings

| Key | Action | Notes |
|---|---|---|
| `f` | Generate report for selected year | Rebind of today's `r`: `findWindowStateCmd` → `generateReportCmd` (CLOSED → stored export, DRAFT/OPEN → live preview) |
| `i` | Import forecasts | `importer.Load(...)` → `Forecasts.AdminImport(...)`; requires OPEN window |
| `b` | Backup now | `Backuper.Backup(ctx)` |
| `r` | Restore | Opens backup selector modal → `Backuper.StageRestore(path)` |

### 2.2 Body & detail

- **Body (`View`)** — unchanged from today: `Any seleccionat: N` and the `Anys amb informe desat`
  context list.
- **Detail (`Detail`)** — shows the last action's outcome, driven by a small result union held on
  the panel: report paths, an import summary (`Importats 12 (esborrats 40)`), a backup path, a
  restore-staged notice (`Es restaurarà en reiniciar l'aplicació.`), or an error via `errDetail`.
- **`Actions()`** — `{f, genera informe}`, `{i, importa previsions}`, `{b, còpia de seguretat}`,
  `{r, restaura}`.

### 2.3 Restore selector modal

`r` builds a modal from `Backuper.ListBackups()` (newest first). A minimal single-column
list-select (numbered entries or up/down highlight, reusing the existing modal/keybinding
conventions). On pick + confirm → `StageRestore(chosen.Path)`; on `esc` → close. Empty list →
inline "cap còpia de seguretat" and no modal.

---

## 3. Forecast import

### 3.1 File format — `Home/import/<year>-forecasts.json`

```json
{
  "year": 2025,
  "forecasts": [
    { "partnerId": 7, "scope": "COMMON",  "sectionCode": "",      "subtypeCode": "a1", "concept": "Assegurança collita", "description": "", "grossAmount": "2880.00",  "plannedDate": "2025-06-15" },
    { "partnerId": 1, "scope": "SECTION", "sectionCode": "oliva", "subtypeCode": "a1", "concept": "Poda",               "description": "", "grossAmount": "1200.00",  "plannedDate": "2025-03-01" },
    { "partnerId": 3, "scope": "PARTNER", "sectionCode": "",      "subtypeCode": "b1", "concept": "Tractor",            "description": "", "grossAmount": "31900.00", "plannedDate": "2025-09-01" }
  ]
}
```

Field rules:

- `year` (top-level) must equal the selected year — guards against importing the wrong file.
- `partnerId` — integer soci number; the partner must exist.
- `scope` — one of `COMMON` / `SECTION` / `PARTNER`. `sectionCode` is required **iff** `SECTION`,
  and must be empty otherwise.
- `subtypeCode` — must exist for that year (subtypes are year-scoped; this implies its type exists).
- `grossAmount` — decimal string parsed by `model.MoneyFromString` (e.g. `"2880.00"`).
- `plannedDate` — `YYYY-MM-DD`.
- No `id`, no approved fields. Imported forecasts are always fresh: approved = 0, approvedOn = nil,
  enabled = true, addedOn = now (same as `AdminCreate`).

### 3.2 Parser — `internal/adapters/importer` (new)

Pure, filesystem-only, no DB:

```go
func Load(path string, year int) ([]application.ForecastImportEntry, error)
```

Reads the file, unmarshals, checks top-level `year == year`, parses money/date/scope into
domain-typed entries. Keeps JSON and string formats out of the application layer. Errors are
row-referenced, e.g. `forecast[3]: invalid grossAmount "abc"`, `forecast[3]: SECTION scope requires
sectionCode`, `file year 2024 does not match selected year 2025`, missing file → clear message.

`application.ForecastImportEntry` (new):

```go
type ForecastImportEntry struct {
    PartnerID   int
    Scope       model.ScopeKind
    SectionCode string
    SubtypeCode string
    Concept     string
    Description string
    GrossAmount model.Money
    PlannedDate time.Time
}
```

### 3.3 Service — `ForecastService.AdminImport`

```go
func (s *ForecastService) AdminImport(ctx context.Context, actorEmail string, year int,
    entries []ForecastImportEntry) (ImportResult, error)

type ImportResult struct { Deleted, Created int }
```

Inside one `WithinTx(func(r ports.RepoSet) error { ... })`:

1. Load the window for `year`; require `WindowOpen` → else `ErrWindowNotOpen`.
2. Validate every entry's references: `r.Partners.FindByID`, subtype present in
   `r.Taxonomy.ListSubtypes(year)`, section present in `r.Sections.List()` when `SECTION`. First
   miss → row-referenced error; nothing written.
3. Replace-all: `r.Forecasts.ListByYear(year)` → `r.Forecasts.Delete(id)` each; then
   `r.Forecasts.Create(f)` each new entry (allocates fresh `CP<yy>nnn` ids). Build each via
   `model.NewUnsavedExpenseForecast(partner, ...)` with approved = 0 / approvedOn = nil /
   enabled = true / addedOn = now.
4. Audit: one `FORECAST_DELETED` per removed + one `FORECAST_CREATED` per added (existing kinds via
   the `forecastAuditEmail` helper). Return `ImportResult{Deleted, Created}`.

Because it is a single transaction, a validation failure rolls back and the year's existing
forecasts are untouched.

### 3.4 Config

Add `import/` to the `EnsureHome` tree (alongside `reports/`, `backups/`) and an `ImportDir`
(`filepath.Join(Home, "import")`) field on `Config`. The panel resolves the file as
`filepath.Join(Cfg.ImportDir, fmt.Sprintf("%d-forecasts.json", year))`.

---

## 4. Backup & restore

### 4.1 Adapter — `internal/adapters/persistence/backup` (new)

```go
type Backuper interface {
    Backup(ctx context.Context) (path string, err error) // VACUUM INTO backups/espigol-<ts>.db
    ListBackups() ([]BackupFile, error)                  // backups/, newest first
    StageRestore(srcPath string) error                   // safety-backup + copy → restore-pending.db
}

type BackupFile struct { Path string; Name string; ModTime time.Time; Size int64 }
```

- `Backup` runs `VACUUM INTO 'Home/backups/espigol-YYYYMMDD-HHMMSS.db'` on the live connection —
  a consistent, compact single-file copy, WAL-safe, no external tools. Returns the written path.
- `ListBackups` lists `espigol-*.db` files in `BackupDir`, newest first (by name/ModTime).
- `StageRestore(src)` first calls `Backup` (safety copy of the current DB), then copies `src` to
  `Home/restore-pending.db`.

Constructed with the `*sql.DB` handle, `BackupDir`, `Home`, and a clock; wired into `tui.Deps` as
`Backup Backuper`.

### 4.2 Startup swap — `db.ApplyPendingRestore`

Called at the top of `db.Open(path)`, before `sql.Open`, deriving paths from `path`'s directory
(keeps `db.Open` self-contained and covers both the TUI and `--server`):

1. If `<dir>/restore-pending.db` does not exist → no-op.
2. Otherwise: `os.Rename(pending, dbPath)` (replaces `espigol.db`), remove stale `espigol.db-wal`
   and `espigol.db-shm` sidecars, remove the pending marker.
3. `db.Open` then continues normally — `goose.Up` migrates a restored older-schema backup forward.

A backup taken from a newer binary than the one restoring is an accepted edge case (goose finds no
applicable migrations).

---

## 5. Wiring

- `internal/wire/wire.go` `TUI`: construct `backup.New(conn, cfg.BackupDir, cfg.Home, clock)` and add
  it to `tui.Deps`; swap `tui.NewReportsPanel` → `tui.NewAdminPanel`.
- `internal/adapters/persistence/db/db.go` `Open`: call `ApplyPendingRestore` first.
- `internal/config/config.go`: `EnsureHome` adds `import/`; `Config` gains `ImportDir`.

---

## 6. Tests

**Importer (`internal/adapters/importer`):** valid file → entries; missing file; year mismatch;
bad money / bad date / bad scope; `SECTION` without `sectionCode` and non-`SECTION` with one.

**`ForecastService.AdminImport`:** happy path (replace-all counts, idempotent re-run yields same
set); OPEN required (DRAFT/CLOSED → `ErrWindowNotOpen`); each missing reference (partner, subtype,
section) → error and no writes (existing forecasts preserved); audit rows appended.

**Backup adapter:** `Backup` produces an openable copy containing the seeded data; `ListBackups`
ordering; `StageRestore` writes both the safety backup and `restore-pending.db`.

**`db.ApplyPendingRestore`:** pending present → file swapped, WAL/SHM cleared, marker removed;
pending absent → no-op; **round-trip** — seed data X → backup → mutate to Y → stage → apply →
reopen → data is X.

**Panel (`panel_admin_test.go`):** update `TestReportsPanel_*` to `NewAdminPanel` + `f` key; `b`
invokes `Backup`; `r` opens the selector and `StageRestore` is called on confirm.

---

## 7. Out of scope

- Backup pruning / retention policies.
- Immediate in-session restore (no restart).
- Import of partners / sections / taxonomy / windows (those must pre-exist).
- Scheduling or automatic backups.
