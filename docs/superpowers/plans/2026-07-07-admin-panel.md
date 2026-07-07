# Admin Panel (import + backup/restore) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn TUI panel `[6] Informes` into `[6] Admin` that generates the year report (`f`), bulk-imports forecasts from JSON (`i`), backs up the SQLite DB (`b`), and restores it (`r`).

**Architecture:** A new `importer` adapter parses `Home/import/<year>-forecasts.json` into domain-typed entries; `ForecastService.AdminImport` replaces a year's forecasts in one transaction (requires an OPEN window, validates references). A new `backup` adapter uses SQLite `VACUUM INTO` for snapshots and stages restores as `restore-pending.db`, which `db.Open` swaps in on next launch. The reports panel is renamed to the admin panel and gains the four keybindings.

**Tech Stack:** Go, Bubble Tea + lipgloss (TUI), modernc.org/sqlite, goose migrations, hexagonal architecture (domain `model`/`ports`, `application` services, `adapters`).

## Global Constraints

- Language of all user-facing TUI strings: **Catalan**.
- Import target is the **selected-year context**; import requires that year's window state to be **OPEN** (`application.ErrWindowNotOpen`), not merely editable.
- Import is **replace-all** and **atomic**: run inside one `TxManager.WithinTx`; any validation failure rolls back and leaves existing forecasts untouched.
- JSON `scope` tokens are the canonical model values: `COMMON` / `SECTION` / `PARTNER`. `sectionCode` is required **iff** `SECTION`.
- Imported forecasts are always fresh: `approvedAmount = ZeroMoney()`, `approvedOn = nil`, `enabled = true`, `addedOn = now`.
- Backup uses `VACUUM INTO` (no external tools). Backups are kept (no pruning). Filename: `espigol-YYYYMMDD-HHMMSS.db`.
- Restore is staged (`Home/restore-pending.db`) and applied on next process start inside `db.Open`; the current DB is safety-backed-up first.
- Keybindings on the Admin panel: `f` report, `i` import, `b` backup, `r` restore.
- Follow existing patterns: services return sentinel errors from `internal/application/errors.go`; audit via the `forecastAuditEmail` helper; commit after each task.

---

### Task 1: Config — `import/` directory and `ImportDir` field

**Files:**
- Modify: `internal/config/config.go` (Config struct, `EnsureHome`, `Load`)
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Config.ImportDir string` (= `filepath.Join(Home, "import")`); `EnsureHome` creates `<home>/import`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/config/config_test.go`:

```go
func TestEnsureHome_CreatesImportDir(t *testing.T) {
	home := filepath.Join(t.TempDir(), "espigol")
	if err := EnsureHome(home); err != nil {
		t.Fatalf("EnsureHome: %v", err)
	}
	fi, err := os.Stat(filepath.Join(home, "import"))
	if err != nil || !fi.IsDir() {
		t.Fatalf("expected import/ dir, err=%v", err)
	}
}

func TestLoad_SetsImportDir(t *testing.T) {
	home := t.TempDir()
	cfg, err := Load(home)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ImportDir != filepath.Join(home, "import") {
		t.Errorf("ImportDir = %q, want %q", cfg.ImportDir, filepath.Join(home, "import"))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run 'ImportDir' -v`
Expected: FAIL — `cfg.ImportDir` undefined (compile error).

- [ ] **Step 3: Add the `ImportDir` field**

In `internal/config/config.go`, add to the `Config` struct after `BackupDir string`:

```go
	BackupDir    string
	ImportDir    string
```

- [ ] **Step 4: Create `import/` in `EnsureHome`**

In `EnsureHome`, extend the directory slice:

```go
	for _, dir := range []string{
		home,
		filepath.Join(home, "reports"),
		filepath.Join(home, "backups"),
		filepath.Join(home, "import"),
	} {
```

- [ ] **Step 5: Set `ImportDir` in `Load`**

In `Load`, add to the `cfg := &Config{...}` literal after `BackupDir: v.GetString("backup.dir"),`:

```go
		BackupDir:    v.GetString("backup.dir"),
		ImportDir:    filepath.Join(home, "import"),
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS (all config tests).

- [ ] **Step 7: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add import/ dir and ImportDir field"
```

---

### Task 2: `db.ApplyPendingRestore` and call it in `db.Open`

**Files:**
- Modify: `internal/adapters/persistence/db/db.go`
- Test: `internal/adapters/persistence/db/db_test.go`

**Interfaces:**
- Produces: `func db.ApplyPendingRestore(dbPath string) error` — if `<dir>/restore-pending.db` exists, renames it over `dbPath`, removes `dbPath-wal`/`dbPath-shm`, and deletes the marker; no-op otherwise. Called at the top of `db.Open`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/adapters/persistence/db/db_test.go`:

```go
func TestApplyPendingRestore_SwapsFileAndClearsSidecars(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "espigol.db")
	if err := os.WriteFile(dbPath, []byte("OLD"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath+"-wal", []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath+"-shm", []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	pending := filepath.Join(dir, "restore-pending.db")
	if err := os.WriteFile(pending, []byte("NEW"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ApplyPendingRestore(dbPath); err != nil {
		t.Fatalf("ApplyPendingRestore: %v", err)
	}

	got, _ := os.ReadFile(dbPath)
	if string(got) != "NEW" {
		t.Errorf("db content = %q, want NEW", got)
	}
	if _, err := os.Stat(pending); !os.IsNotExist(err) {
		t.Errorf("pending marker should be gone, err=%v", err)
	}
	if _, err := os.Stat(dbPath + "-wal"); !os.IsNotExist(err) {
		t.Errorf("-wal sidecar should be removed")
	}
	if _, err := os.Stat(dbPath + "-shm"); !os.IsNotExist(err) {
		t.Errorf("-shm sidecar should be removed")
	}
}

func TestApplyPendingRestore_NoPendingIsNoOp(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "espigol.db")
	if err := os.WriteFile(dbPath, []byte("KEEP"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ApplyPendingRestore(dbPath); err != nil {
		t.Fatalf("ApplyPendingRestore: %v", err)
	}
	got, _ := os.ReadFile(dbPath)
	if string(got) != "KEEP" {
		t.Errorf("db content = %q, want KEEP", got)
	}
}
```

Ensure the test file imports `os` and `path/filepath` (add to its import block if missing).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapters/persistence/db/ -run ApplyPendingRestore -v`
Expected: FAIL — `ApplyPendingRestore` undefined.

- [ ] **Step 3: Implement `ApplyPendingRestore` and call it in `Open`**

In `internal/adapters/persistence/db/db.go`, add `os` and `path/filepath` to the import block, then add the function and call it first in `Open`:

```go
// Open opens the database at path ... (existing doc comment unchanged)
func Open(path string) (*sql.DB, error) {
	if err := ApplyPendingRestore(path); err != nil {
		return nil, err
	}
	dsn := fmt.Sprintf(
		"file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)",
		path,
	)
	// ... rest of Open unchanged ...
}

// ApplyPendingRestore swaps a staged restore into place before the database is
// opened. If <dir>/restore-pending.db exists it replaces the database file,
// removes the stale -wal/-shm sidecars (which belong to the old database), and
// deletes the marker. It is a no-op when no restore is pending. Running here,
// the single open choke point, means both the TUI and --server apply a
// pending restore on their next start.
func ApplyPendingRestore(dbPath string) error {
	pending := filepath.Join(filepath.Dir(dbPath), "restore-pending.db")
	if _, err := os.Stat(pending); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("checking pending restore: %w", err)
	}
	if err := os.Rename(pending, dbPath); err != nil {
		return fmt.Errorf("applying pending restore: %w", err)
	}
	for _, sidecar := range []string{dbPath + "-wal", dbPath + "-shm"} {
		if err := os.Remove(sidecar); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing %s: %w", sidecar, err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapters/persistence/db/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/persistence/db/db.go internal/adapters/persistence/db/db_test.go
git commit -m "feat(db): apply staged restore-pending.db on Open"
```

---

### Task 3: `ForecastService.AdminImport` + `ForecastImportEntry` + `ImportResult`

**Files:**
- Create: `internal/application/forecast_import.go`
- Test: `internal/application/forecast_import_test.go`

**Interfaces:**
- Consumes: `ForecastService` (`s.tx`, `s.clock`), `ports.RepoSet` (`Windows.FindByYear`, `Taxonomy.ListSubtypes`, `Sections.List`, `Partners.FindByID`, `Forecasts.ListByYear/Delete/Create`), `forecastAuditEmail`, `model.NewScope`, `model.NewUnsavedExpenseForecast`, `ErrWindowNotFound`, `ErrWindowNotOpen`.
- Produces:
  - `type ForecastImportEntry struct { PartnerID int; Scope model.ScopeKind; SectionCode string; SubtypeCode string; Concept string; Description string; GrossAmount model.Money; PlannedDate time.Time }`
  - `type ImportResult struct { Deleted int; Created int }`
  - `func (s *ForecastService) AdminImport(ctx context.Context, actorEmail string, year int, entries []ForecastImportEntry) (ImportResult, error)`

- [ ] **Step 1: Write the failing tests**

Create `internal/application/forecast_import_test.go`:

```go
package application_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

func impNow() time.Time { return time.Date(2025, 6, 1, 9, 0, 0, 0, time.UTC) }

// seedImportYear seeds a window (given state) for 2025 with type A / subtype a1,
// section "oliva", and partners 1 and 7. Returns the open conn and queries.
func seedImportYear(t *testing.T, state model.WindowState) (*application.ForecastService, *persistence.ForecastRepository, func(context.Context) []model.ExpenseForecast) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "imp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	q := sqlc.New(conn)
	ctx := context.Background()

	var openedAt, closedAt *time.Time
	if state == model.WindowOpen || state == model.WindowClosed {
		o := impNow()
		openedAt = &o
	}
	if state == model.WindowClosed {
		c := impNow()
		closedAt = &c
	}
	w, _ := model.NewSubmissionWindow(2025, state, openedAt, closedAt,
		time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	if err := persistence.NewWindowRepository(q).Save(ctx, w); err != nil {
		t.Fatal(err)
	}
	tax := persistence.NewTaxonomyRepository(q)
	ta, _ := model.NewExpenseType(2025, "A", "[a]", model.CategoryCurrent)
	_ = tax.SaveType(ctx, ta)
	sa, _ := model.NewExpenseSubtype(2025, "a1", "[a1]", "A")
	_ = tax.SaveSubtype(ctx, sa)
	sec, _ := model.NewSection("oliva", "Secció d'oliva", true, 1)
	_ = persistence.NewSectionRepository(q).Save(ctx, sec)
	pr := persistence.NewPartnerRepository(q)
	for _, id := range []int{1, 7} {
		p, _ := model.NewPartner(id, "Soci", "", "", "s@e.test", "", model.Productor, 0, impNow(), false)
		_ = pr.Save(ctx, p)
	}

	fr := persistence.NewForecastRepository(conn, q)
	svc := application.NewForecastService(persistence.NewTxManager(conn), fixedClock{t: impNow()})
	list := func(ctx context.Context) []model.ExpenseForecast {
		out, err := fr.ListByYear(ctx, 2025)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	return svc, fr, list
}

func commonEntry(partnerID int, gross string) application.ForecastImportEntry {
	amt, _ := model.MoneyFromString(gross)
	return application.ForecastImportEntry{
		PartnerID:   partnerID,
		Scope:       model.ScopeCommon,
		SubtypeCode: "a1",
		Concept:     "Concepte",
		GrossAmount: amt,
		PlannedDate: time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
	}
}

func TestAdminImport_ReplacesAllForYear(t *testing.T) {
	svc, fr, list := seedImportYear(t, model.WindowOpen)
	ctx := context.Background()

	// Pre-existing forecast that import must remove.
	amt, _ := model.MoneyFromString("999.00")
	pre, _ := model.NewUnsavedExpenseForecast(mustPartner(t, 1), "Vell", "", amt,
		model.ZeroMoney(), nil, time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC), 2025, "a1",
		model.NewCommonScope(), impNow(), true)
	if _, err := fr.Create(ctx, pre); err != nil {
		t.Fatal(err)
	}

	entries := []application.ForecastImportEntry{commonEntry(7, "2880.00"), commonEntry(1, "1200.00")}
	res, err := svc.AdminImport(ctx, "admin@espigol", 2025, entries)
	if err != nil {
		t.Fatalf("AdminImport: %v", err)
	}
	if res.Deleted != 1 || res.Created != 2 {
		t.Errorf("result = %+v, want {Deleted:1 Created:2}", res)
	}
	if got := len(list(ctx)); got != 2 {
		t.Errorf("forecasts after import = %d, want 2", got)
	}

	// Idempotent: re-running yields the same set (2), not 4.
	res2, err := svc.AdminImport(ctx, "admin@espigol", 2025, entries)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Deleted != 2 || res2.Created != 2 || len(list(ctx)) != 2 {
		t.Errorf("re-run not idempotent: res=%+v count=%d", res2, len(list(ctx)))
	}
}

func TestAdminImport_RequiresOpenWindow(t *testing.T) {
	svc, _, _ := seedImportYear(t, model.WindowDraft)
	_, err := svc.AdminImport(context.Background(), "admin@espigol", 2025,
		[]application.ForecastImportEntry{commonEntry(1, "100.00")})
	if !errors.Is(err, application.ErrWindowNotOpen) {
		t.Errorf("want ErrWindowNotOpen, got %v", err)
	}
}

func TestAdminImport_MissingPartnerRollsBack(t *testing.T) {
	svc, fr, list := seedImportYear(t, model.WindowOpen)
	ctx := context.Background()
	amt, _ := model.MoneyFromString("50.00")
	pre, _ := model.NewUnsavedExpenseForecast(mustPartner(t, 1), "Vell", "", amt,
		model.ZeroMoney(), nil, time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC), 2025, "a1",
		model.NewCommonScope(), impNow(), true)
	if _, err := fr.Create(ctx, pre); err != nil {
		t.Fatal(err)
	}

	entries := []application.ForecastImportEntry{commonEntry(99, "100.00")}
	if _, err := svc.AdminImport(ctx, "admin@espigol", 2025, entries); err == nil {
		t.Fatal("expected error for missing partner 99")
	}
	// Roll back: the pre-existing forecast must survive.
	if got := len(list(ctx)); got != 1 {
		t.Errorf("forecasts after failed import = %d, want 1 (unchanged)", got)
	}
}

// mustPartner builds a valid Partner value for constructing test forecasts.
func mustPartner(t *testing.T, id int) model.Partner {
	t.Helper()
	p, err := model.NewPartner(id, "Soci", "", "", "s@e.test", "", model.Productor, 0, impNow(), false)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
```

Note: `fixedClock` already exists in the `application_test` package (used by other service tests). If the compiler reports it undefined, define a minimal one in this file: `type fixedClock struct{ t time.Time }; func (c fixedClock) Now() time.Time { return c.t }` — but check first, it is almost certainly already there.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/application/ -run AdminImport -v`
Expected: FAIL — `AdminImport` / `ForecastImportEntry` / `ImportResult` undefined.

- [ ] **Step 3: Implement `forecast_import.go`**

Create `internal/application/forecast_import.go`:

```go
package application

import (
	"context"
	"fmt"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/ports"
)

// ForecastImportEntry is one parsed row from an import file, already converted
// to domain-typed values by the importer adapter. AdminImport consumes these.
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

// ImportResult reports how many forecasts a replace-all import removed and added.
type ImportResult struct {
	Deleted int
	Created int
}

// AdminImport replaces every forecast for year with entries, in one
// transaction. The year's window must be OPEN. Every entry's partner, subtype
// (year-scoped) and section (when SECTION-scoped) must already exist, otherwise
// the whole import rolls back and the year's existing forecasts are untouched.
// Imported forecasts are fresh: approved = 0, approvedOn = nil, enabled = true.
func (s *ForecastService) AdminImport(ctx context.Context, actorEmail string, year int,
	entries []ForecastImportEntry) (ImportResult, error) {
	now := s.clock.Now()
	var result ImportResult
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		w, ok, err := r.Windows.FindByYear(ctx, year)
		if err != nil {
			return err
		}
		if !ok {
			return ErrWindowNotFound
		}
		if w.State() != model.WindowOpen {
			return ErrWindowNotOpen
		}

		subtypes, err := r.Taxonomy.ListSubtypes(ctx, year)
		if err != nil {
			return err
		}
		subtypeCodes := make(map[string]bool, len(subtypes))
		for _, st := range subtypes {
			subtypeCodes[st.Code()] = true
		}
		sections, err := r.Sections.List(ctx)
		if err != nil {
			return err
		}
		sectionCodes := make(map[string]bool, len(sections))
		for _, sec := range sections {
			sectionCodes[sec.Code()] = true
		}

		// Validate and build every forecast before mutating anything.
		built := make([]model.ExpenseForecast, 0, len(entries))
		for i, e := range entries {
			partner, ok, err := r.Partners.FindByID(ctx, e.PartnerID)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("forecast[%d]: partner %d not found", i, e.PartnerID)
			}
			if !subtypeCodes[e.SubtypeCode] {
				return fmt.Errorf("forecast[%d]: subtype %q not found for year %d", i, e.SubtypeCode, year)
			}
			if e.Scope == model.ScopeSection && !sectionCodes[e.SectionCode] {
				return fmt.Errorf("forecast[%d]: section %q not found", i, e.SectionCode)
			}
			scope, err := model.NewScope(e.Scope, e.SectionCode)
			if err != nil {
				return fmt.Errorf("forecast[%d]: %w", i, err)
			}
			f, err := model.NewUnsavedExpenseForecast(partner, e.Concept, e.Description,
				e.GrossAmount, model.ZeroMoney(), nil, e.PlannedDate, year, e.SubtypeCode, scope, now, true)
			if err != nil {
				return fmt.Errorf("forecast[%d]: %w", i, err)
			}
			built = append(built, f)
		}

		// Replace-all: delete the year's existing forecasts, then insert.
		existing, err := r.Forecasts.ListByYear(ctx, year)
		if err != nil {
			return err
		}
		for _, old := range existing {
			if err := r.Forecasts.Delete(ctx, old.ID()); err != nil {
				return err
			}
			if err := forecastAuditEmail(ctx, r, actorEmail, model.AuditForecastDeleted, old.ID(), now); err != nil {
				return err
			}
			result.Deleted++
		}
		for _, f := range built {
			saved, err := r.Forecasts.Create(ctx, f)
			if err != nil {
				return err
			}
			if err := forecastAuditEmail(ctx, r, actorEmail, model.AuditForecastCreated, saved.ID(), now); err != nil {
				return err
			}
			result.Created++
		}
		return nil
	})
	if err != nil {
		return ImportResult{}, err
	}
	return result, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/application/ -run AdminImport -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/application/forecast_import.go internal/application/forecast_import_test.go
git commit -m "feat(application): AdminImport replace-all forecast import"
```

---

### Task 4: `importer.Load` adapter

**Files:**
- Create: `internal/adapters/importer/importer.go`
- Test: `internal/adapters/importer/importer_test.go`

**Interfaces:**
- Consumes: `application.ForecastImportEntry`, `model.ParseScopeKind`, `model.NewScope`, `model.MoneyFromString`.
- Produces: `func importer.Load(path string, year int) ([]application.ForecastImportEntry, error)`.

- [ ] **Step 1: Write the failing tests**

Create `internal/adapters/importer/importer_test.go`:

```go
package importer_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pjover/espigol/internal/adapters/importer"
	"github.com/pjover/espigol/internal/domain/model"
)

func writeFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "2025-forecasts.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const validBody = `{
  "year": 2025,
  "forecasts": [
    { "partnerId": 7, "scope": "COMMON",  "sectionCode": "",      "subtypeCode": "a1", "concept": "Assegurança", "description": "", "grossAmount": "2880.00", "plannedDate": "2025-06-15" },
    { "partnerId": 1, "scope": "SECTION", "sectionCode": "oliva", "subtypeCode": "a1", "concept": "Poda",       "description": "", "grossAmount": "1200.00", "plannedDate": "2025-03-01" }
  ]
}`

func TestLoad_Valid(t *testing.T) {
	entries, err := importer.Load(writeFile(t, validBody), 2025)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len = %d, want 2", len(entries))
	}
	if entries[0].Scope != model.ScopeCommon || entries[0].PartnerID != 7 {
		t.Errorf("entry0 = %+v", entries[0])
	}
	if entries[1].Scope != model.ScopeSection || entries[1].SectionCode != "oliva" {
		t.Errorf("entry1 = %+v", entries[1])
	}
	if entries[0].GrossAmount.String() != "2880.00" {
		t.Errorf("gross = %s, want 2880.00", entries[0].GrossAmount.String())
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := importer.Load(filepath.Join(t.TempDir(), "nope.json"), 2025)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_YearMismatch(t *testing.T) {
	_, err := importer.Load(writeFile(t, validBody), 2024)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("want year-mismatch error, got %v", err)
	}
}

func TestLoad_BadFields(t *testing.T) {
	cases := map[string]string{
		"bad money": `{"year":2025,"forecasts":[{"partnerId":1,"scope":"COMMON","subtypeCode":"a1","concept":"x","grossAmount":"abc","plannedDate":"2025-06-15"}]}`,
		"bad date":  `{"year":2025,"forecasts":[{"partnerId":1,"scope":"COMMON","subtypeCode":"a1","concept":"x","grossAmount":"10.00","plannedDate":"nope"}]}`,
		"bad scope": `{"year":2025,"forecasts":[{"partnerId":1,"scope":"WAT","subtypeCode":"a1","concept":"x","grossAmount":"10.00","plannedDate":"2025-06-15"}]}`,
		"section no code": `{"year":2025,"forecasts":[{"partnerId":1,"scope":"SECTION","sectionCode":"","subtypeCode":"a1","concept":"x","grossAmount":"10.00","plannedDate":"2025-06-15"}]}`,
		"common with code": `{"year":2025,"forecasts":[{"partnerId":1,"scope":"COMMON","sectionCode":"oliva","subtypeCode":"a1","concept":"x","grossAmount":"10.00","plannedDate":"2025-06-15"}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := importer.Load(writeFile(t, body), 2025); err == nil {
				t.Errorf("expected error for %s", name)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapters/importer/ -v`
Expected: FAIL — package/`Load` undefined.

- [ ] **Step 3: Implement `importer.go`**

Create `internal/adapters/importer/importer.go`:

```go
// Package importer reads forecast import files (Home/import/<year>-forecasts.json)
// into application.ForecastImportEntry values. It performs format and scope
// consistency validation only; referential integrity (partner/subtype/section
// existence and the OPEN-window rule) is enforced by ForecastService.AdminImport.
package importer

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

type fileDoc struct {
	Year      int         `json:"year"`
	Forecasts []fileEntry `json:"forecasts"`
}

type fileEntry struct {
	PartnerID   int    `json:"partnerId"`
	Scope       string `json:"scope"`
	SectionCode string `json:"sectionCode"`
	SubtypeCode string `json:"subtypeCode"`
	Concept     string `json:"concept"`
	Description string `json:"description"`
	GrossAmount string `json:"grossAmount"`
	PlannedDate string `json:"plannedDate"`
}

// Load reads and parses the import file at path, requiring its top-level year to
// equal year. Errors are row-referenced (forecast[i]: ...).
func Load(path string, year int) ([]application.ForecastImportEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading import file: %w", err)
	}
	var doc fileDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing import file: %w", err)
	}
	if doc.Year != year {
		return nil, fmt.Errorf("file year %d does not match selected year %d", doc.Year, year)
	}
	entries := make([]application.ForecastImportEntry, 0, len(doc.Forecasts))
	for i, fe := range doc.Forecasts {
		scope, err := model.ParseScopeKind(fe.Scope)
		if err != nil {
			return nil, fmt.Errorf("forecast[%d]: %w", i, err)
		}
		// Validate scope/sectionCode consistency now for a row-referenced error.
		if _, err := model.NewScope(scope, fe.SectionCode); err != nil {
			return nil, fmt.Errorf("forecast[%d]: %w", i, err)
		}
		gross, err := model.MoneyFromString(fe.GrossAmount)
		if err != nil {
			return nil, fmt.Errorf("forecast[%d]: invalid grossAmount %q: %w", i, fe.GrossAmount, err)
		}
		planned, err := time.Parse("2006-01-02", fe.PlannedDate)
		if err != nil {
			return nil, fmt.Errorf("forecast[%d]: invalid plannedDate %q: %w", i, fe.PlannedDate, err)
		}
		entries = append(entries, application.ForecastImportEntry{
			PartnerID:   fe.PartnerID,
			Scope:       scope,
			SectionCode: fe.SectionCode,
			SubtypeCode: fe.SubtypeCode,
			Concept:     fe.Concept,
			Description: fe.Description,
			GrossAmount: gross,
			PlannedDate: planned,
		})
	}
	return entries, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapters/importer/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/importer/
git commit -m "feat(importer): parse forecast import JSON files"
```

---

### Task 5: `backup` adapter (`Backup` / `ListBackups` / `StageRestore`)

**Files:**
- Create: `internal/adapters/persistence/backup/backup.go`
- Test: `internal/adapters/persistence/backup/backup_test.go`

**Interfaces:**
- Consumes: a live `*sql.DB` (for `VACUUM INTO`), `dbPath`, `backupDir`, a `Clock`.
- Produces:
  - `type Clock interface{ Now() time.Time }`
  - `type BackupFile struct { Path string; Name string; ModTime time.Time; Size int64 }`
  - `type Backuper interface { Backup(ctx context.Context) (string, error); ListBackups() ([]BackupFile, error); StageRestore(srcPath string) error }`
  - `func New(db *sql.DB, dbPath, backupDir string, clock Clock) *Service` (implements `Backuper`)

- [ ] **Step 1: Write the failing tests**

Create `internal/adapters/persistence/backup/backup_test.go`:

```go
package backup_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence/backup"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
)

type fakeClock struct{ t time.Time }

func (c fakeClock) Now() time.Time { return c.t }

func newSvc(t *testing.T) (*backup.Service, string, string) {
	t.Helper()
	home := t.TempDir()
	dbPath := filepath.Join(home, "espigol.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	backupDir := filepath.Join(home, "backups")
	svc := backup.New(conn, dbPath, backupDir, fakeClock{t: time.Date(2025, 6, 1, 10, 20, 30, 0, time.UTC)})
	return svc, dbPath, backupDir
}

func TestBackup_ProducesOpenableCopy(t *testing.T) {
	svc, _, backupDir := newSvc(t)
	path, err := svc.Backup(context.Background())
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if filepath.Dir(path) != backupDir {
		t.Errorf("backup path %q not in backupDir %q", path, backupDir)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
	// The copy is a valid, migrated database.
	conn, err := db.Open(path)
	if err != nil {
		t.Fatalf("opening backup copy: %v", err)
	}
	conn.Close()
}

func TestListBackups_NewestFirst(t *testing.T) {
	svc, _, backupDir := newSvc(t)
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"espigol-20250601-090000.db", "espigol-20250602-090000.db", "notabackup.txt"} {
		if err := os.WriteFile(filepath.Join(backupDir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files, err := svc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("len = %d, want 2 (txt ignored)", len(files))
	}
	if files[0].Name != "espigol-20250602-090000.db" {
		t.Errorf("newest first failed: %q", files[0].Name)
	}
}

func TestStageRestore_WritesPendingAndSafetyBackup(t *testing.T) {
	svc, dbPath, backupDir := newSvc(t)
	src := filepath.Join(t.TempDir(), "chosen.db")
	if err := os.WriteFile(src, []byte("RESTORE-ME"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := svc.StageRestore(src); err != nil {
		t.Fatalf("StageRestore: %v", err)
	}
	pending := filepath.Join(filepath.Dir(dbPath), "restore-pending.db")
	got, err := os.ReadFile(pending)
	if err != nil || string(got) != "RESTORE-ME" {
		t.Fatalf("pending content = %q err=%v", got, err)
	}
	entries, _ := os.ReadDir(backupDir)
	if len(entries) == 0 {
		t.Error("expected a safety backup to be created before staging")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapters/persistence/backup/ -v`
Expected: FAIL — package/`New` undefined.

- [ ] **Step 3: Implement `backup.go`**

Create `internal/adapters/persistence/backup/backup.go`:

```go
// Package backup creates and stages restores of the espigol SQLite database
// for the admin TUI. Backups use VACUUM INTO — a consistent, compact
// single-file copy that is safe on the live WAL database and needs no external
// tools. Restores are staged as <db dir>/restore-pending.db and applied on the
// next process start by db.ApplyPendingRestore.
package backup

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Clock returns the current time. It mirrors ports.Clock but is redeclared here
// so this adapter does not depend on the domain.
type Clock interface{ Now() time.Time }

// BackupFile describes one backup on disk.
type BackupFile struct {
	Path    string
	Name    string
	ModTime time.Time
	Size    int64
}

// Backuper is the behaviour the TUI Admin panel consumes.
type Backuper interface {
	Backup(ctx context.Context) (string, error)
	ListBackups() ([]BackupFile, error)
	StageRestore(srcPath string) error
}

// Service implements Backuper against a live *sql.DB and the on-disk layout.
type Service struct {
	db        *sql.DB
	dbPath    string
	backupDir string
	clock     Clock
}

// New builds a backup Service. db is the live connection to dbPath; backupDir is
// where snapshots are written.
func New(db *sql.DB, dbPath, backupDir string, clock Clock) *Service {
	return &Service{db: db, dbPath: dbPath, backupDir: backupDir, clock: clock}
}

// Backup writes a consistent snapshot to backups/espigol-YYYYMMDD-HHMMSS.db and
// returns its path.
func (s *Service) Backup(ctx context.Context) (string, error) {
	if err := os.MkdirAll(s.backupDir, 0o700); err != nil {
		return "", fmt.Errorf("creating backup dir: %w", err)
	}
	name := fmt.Sprintf("espigol-%s.db", s.clock.Now().Format("20060102-150405"))
	dest := filepath.Join(s.backupDir, name)
	// VACUUM INTO takes a SQL string literal for the destination path.
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO "+sqlQuote(dest)); err != nil {
		return "", fmt.Errorf("vacuum into %s: %w", dest, err)
	}
	return dest, nil
}

// ListBackups returns the espigol-*.db files in backupDir, newest first. Because
// the filenames are timestamped, lexical-descending order is newest-first.
func (s *Service) ListBackups() ([]BackupFile, error) {
	entries, err := os.ReadDir(s.backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading backup dir: %w", err)
	}
	var files []BackupFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "espigol-") || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		files = append(files, BackupFile{
			Path:    filepath.Join(s.backupDir, e.Name()),
			Name:    e.Name(),
			ModTime: info.ModTime(),
			Size:    info.Size(),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name > files[j].Name })
	return files, nil
}

// StageRestore first takes a safety backup of the current database, then copies
// srcPath to <db dir>/restore-pending.db, which db.ApplyPendingRestore swaps in
// on the next start.
func (s *Service) StageRestore(srcPath string) error {
	if _, err := s.Backup(context.Background()); err != nil {
		return fmt.Errorf("safety backup before restore: %w", err)
	}
	pending := filepath.Join(filepath.Dir(s.dbPath), "restore-pending.db")
	if err := copyFile(srcPath, pending); err != nil {
		return fmt.Errorf("staging restore: %w", err)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// sqlQuote wraps s in single quotes for a SQL string literal, doubling any
// embedded single quotes.
func sqlQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapters/persistence/backup/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/persistence/backup/
git commit -m "feat(backup): VACUUM INTO snapshots and staged restore"
```

---

### Task 6: Rename reports panel to Admin panel; rebind report to `f`; wire `Backup`

**Files:**
- Rename: `internal/adapters/tui/panel_reports.go` → `internal/adapters/tui/panel_admin.go`
- Modify: `internal/adapters/tui/deps.go` (add `Backup` field)
- Modify: `internal/wire/wire.go` (construct backup service; `NewAdminPanel`)
- Modify: `internal/adapters/tui/panel_admin_test.go` (`TestReportsPanel_*` → `NewAdminPanel` + `f`; extend `testDeps`)
- Modify: `internal/adapters/tui/panel_basic_test.go` (`testDeps` adds `Backup`, `Home`, `BackupDir`, `ImportDir`)

**Interfaces:**
- Consumes: `backup.Backuper` (Task 5).
- Produces:
  - `tui.Deps.Backup backup.Backuper`
  - `func tui.NewAdminPanel(deps Deps) Panel`
  - `type adminResult struct { text string; err error }` and panel field `result *adminResult` (later tasks set it).

- [ ] **Step 1: Rename the file and update `deps.go` / `wire.go` (compile-first refactor)**

```bash
git mv internal/adapters/tui/panel_reports.go internal/adapters/tui/panel_admin.go
```

In `internal/adapters/tui/deps.go`, add the import and field:

```go
import (
	"github.com/pjover/espigol/internal/adapters/persistence/backup"
	"github.com/pjover/espigol/internal/adapters/report"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/config"
)

type Deps struct {
	Partners  *application.PartnerService
	Sections  *application.SectionService
	Taxonomy  *application.TaxonomyService
	BoardAuth *application.BoardAuthorizationService
	Forecasts *application.ForecastService
	Windows   *application.WindowService
	Reports   *application.ReportService
	Exporter  report.ReportExporter
	Backup    backup.Backuper
	Cfg       *config.Config
}
```

In `internal/wire/wire.go` `TUI`, add the import `"github.com/pjover/espigol/internal/adapters/persistence/backup"`, construct the service, add it to `deps`, and switch the panel constructor:

```go
	deps := tui.Deps{
		Partners:  application.NewPartnerService(txm, clock, cfg.Admin.Email),
		Sections:  application.NewSectionService(txm, clock, cfg.Admin.Email),
		Taxonomy:  application.NewTaxonomyService(txm, clock, cfg.Admin.Email),
		BoardAuth: application.NewBoardAuthorizationService(txm, clock, cfg.Admin.Email),
		Forecasts: application.NewForecastService(txm, clock),
		Windows:   application.NewWindowService(txm, pdf, clock),
		Reports:   application.NewReportService(txm),
		Exporter:  reportadapter.NewReportExporter(pdf),
		Backup:    backup.New(conn, cfg.DBPath, cfg.BackupDir, clock),
		Cfg:       cfg,
	}

	panels := []tui.Panel{
		tui.NewYearsPanel(deps),
		tui.NewPartnersPanel(deps),
		tui.NewSectionsPanel(deps),
		tui.NewTaxonomyPanel(deps),
		tui.NewForecastsPanel(deps),
		tui.NewAdminPanel(deps),
	}
```

- [ ] **Step 2: Rewrite `panel_admin.go` (rename type, `f` key, result union)**

Replace the contents of `internal/adapters/tui/panel_admin.go` with:

```go
package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pjover/espigol/internal/domain/model"
)

// adminPanel is the "Admin" panel (formerly "Informes"). It operates on the
// selected-year context and offers: f generate report, i import forecasts,
// b backup the database, r restore it. It also lists which years have a stored
// Report, for context.
type adminPanel struct {
	deps  Deps
	year  int
	state model.WindowState

	years    []int // years with a stored report, ascending
	yearsErr error // error from loading the years-with-reports list

	result *adminResult
}

// adminResult holds the outcome of the last f/i/b/r action, rendered by Detail.
type adminResult struct {
	text string
	err  error
}

// NewAdminPanel builds the Admin panel.
func NewAdminPanel(deps Deps) Panel {
	return adminPanel{deps: deps}
}

func (p adminPanel) Title() string { return "Admin" }

// reportYearsLoadedMsg carries the result of listing which years have a stored
// Report (used only for the "years with reports" context list).
type reportYearsLoadedMsg struct {
	years []int
	err   error
}

func (p adminPanel) loadYearsCmd() tea.Cmd {
	return func() tea.Msg {
		windows, err := p.deps.Windows.List(context.Background())
		if err != nil {
			return reportYearsLoadedMsg{err: err}
		}
		var years []int
		for _, w := range windows {
			if w.State() == model.WindowClosed {
				if _, ok, err := p.deps.Reports.Latest(context.Background(), w.Year()); err == nil && ok {
					years = append(years, w.Year())
				}
			}
		}
		sort.Ints(years)
		return reportYearsLoadedMsg{years: years}
	}
}

// findWindowStateCmd resolves the selected year's window state so the report
// action knows whether to Export (CLOSED) or ExportData (DRAFT/OPEN).
func (p adminPanel) findWindowStateCmd(year int) tea.Cmd {
	return func() tea.Msg {
		windows, err := p.deps.Windows.List(context.Background())
		if err != nil {
			return reportDoneMsg{year: year, err: err}
		}
		for _, w := range windows {
			if w.Year() == year {
				return windowStateMsg{year: year, state: w.State(), found: true}
			}
		}
		return windowStateMsg{year: year, found: false}
	}
}

// windowStateMsg carries the selected year's window state, fetched before
// generating the report.
type windowStateMsg struct {
	year  int
	state model.WindowState
	found bool
}

func (p adminPanel) Update(msg tea.Msg) (Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case panelInitMsg:
		return p, p.loadYearsCmd()

	case yearSelectedMsg:
		p.year = msg.Year
		return p, p.loadYearsCmd()

	case reportYearsLoadedMsg:
		if msg.err != nil {
			p.yearsErr = msg.err
		} else {
			p.yearsErr = nil
			p.years = msg.years
		}
		return p, nil

	case windowStateMsg:
		if msg.year != p.year {
			return p, nil
		}
		if !msg.found {
			p.result = &adminResult{err: fmt.Errorf("cap any %d trobat", p.year)}
			return p, nil
		}
		p.state = msg.state
		return p, generateReportCmd(p.deps, p.year, p.state)

	case reportDoneMsg:
		if msg.year != p.year {
			return p, nil
		}
		if msg.err != nil {
			p.result = &adminResult{err: msg.err}
		} else if len(msg.paths) == 0 {
			p.result = &adminResult{text: "Informe generat (cap fitxer)."}
		} else {
			p.result = &adminResult{text: "Informe generat:\n  " + strings.Join(msg.paths, "\n  ")}
		}
		return p, p.loadYearsCmd()

	case tea.KeyMsg:
		return p.handleKey(msg)
	}
	return p, nil
}

func (p adminPanel) handleKey(msg tea.KeyMsg) (Panel, tea.Cmd) {
	switch msg.String() {
	case "f":
		return p, p.findWindowStateCmd(p.year)
	}
	return p, nil
}

func (p adminPanel) View(width, height int) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Any seleccionat: %d\n\n", p.year))

	if len(p.years) == 0 {
		b.WriteString(dimStyle.Render("(cap any amb informe desat)"))
	} else {
		b.WriteString("Anys amb informe desat: ")
		parts := make([]string, len(p.years))
		for i, y := range p.years {
			parts[i] = fmt.Sprintf("%d", y)
		}
		b.WriteString(strings.Join(parts, ", "))
	}
	return b.String()
}

func (p adminPanel) Detail() string {
	if p.result != nil {
		if p.result.err != nil {
			return errDetail(p.result.err)
		}
		return p.result.text
	}
	if p.yearsErr != nil {
		return errDetail(p.yearsErr)
	}
	return dimStyle.Render("f: informe · i: importa · b: còpia · r: restaura")
}

func (p adminPanel) Actions() []Action {
	return []Action{
		{Key: "f", Label: "genera informe"},
	}
}
```

- [ ] **Step 3: Update `testDeps` to provide `Backup` and home dirs**

In `internal/adapters/tui/panel_basic_test.go`, update `testDeps`. Add imports `"github.com/pjover/espigol/internal/adapters/persistence/backup"` and (if not present) `"time"`, then:

```go
func testDeps(t *testing.T) (Deps, *sqlc.Queries) {
	t.Helper()
	home := t.TempDir()
	dbPath := filepath.Join(home, "panels.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	q := sqlc.New(conn)
	txm := persistence.NewTxManager(conn)
	clock := pbFixedClock{t: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)}

	exporter := appreport.NewReportExporter(appreport.PDFRenderer{BusinessName: "Test"})

	deps := Deps{
		Partners:  application.NewPartnerService(txm, clock, testAdminEmail),
		Sections:  application.NewSectionService(txm, clock, testAdminEmail),
		Taxonomy:  application.NewTaxonomyService(txm, clock, testAdminEmail),
		Forecasts: application.NewForecastService(txm, clock),
		Windows:   application.NewWindowService(txm, appreport.NoopRenderer{}, clock),
		Reports:   application.NewReportService(txm),
		Exporter:  exporter,
		Backup:    backup.New(conn, dbPath, filepath.Join(home, "backups"), clock),
		Cfg: &config.Config{
			Home:      home,
			DBPath:    dbPath,
			OutputDir: filepath.Join(home, "reports"),
			BackupDir: filepath.Join(home, "backups"),
			ImportDir: filepath.Join(home, "import"),
			Admin:     struct{ Email string }{Email: testAdminEmail},
		},
	}
	return deps, q
}
```

Note: `pbFixedClock` must satisfy `backup.Clock` (`Now() time.Time`) — it already has `Now()`, so it works. Create `filepath.Join(home,"reports")` is not required for these tests; the report tests write into `OutputDir` which the exporter creates.

- [ ] **Step 4: Update the reports-panel tests to Admin panel + `f`**

In `internal/adapters/tui/panel_admin_test.go`, replace every `NewReportsPanel(deps)` with `NewAdminPanel(deps)` and every `p.Update(pKey("r"))` (report trigger) with `p.Update(pKey("f"))`. Rename the two functions `TestReportsPanel_ClosedYear_GeneratesViaExportAndFilesExist` and `TestReportsPanel_OpenYear_GeneratesViaExportDataAndFilesExist` to `TestAdminPanel_ClosedYear_...` / `TestAdminPanel_OpenYear_...` for clarity (optional but recommended).

- [ ] **Step 5: Build and run the TUI + wire packages**

Run: `go build ./... && go test ./internal/adapters/tui/ ./internal/wire/ -v`
Expected: PASS. (The renamed report tests exercise the `f` key path.)

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/tui/ internal/wire/wire.go
git commit -m "refactor(tui): rename Informes panel to Admin, report on f key"
```

---

### Task 7: Import key `i`

**Files:**
- Modify: `internal/adapters/tui/panel_admin.go` (add `i` handler, `importForecastsCmd`, `forecastsImportedMsg`, Actions entry)
- Test: `internal/adapters/tui/panel_admin_test.go` (new `TestAdminPanel_Import_*`)

**Interfaces:**
- Consumes: `importer.Load` (Task 4), `Forecasts.AdminImport` (Task 3), `Cfg.ImportDir`, `Cfg.Admin.Email`.
- Produces: `type forecastsImportedMsg struct { year int; result application.ImportResult; err error }`; `func importForecastsCmd(deps Deps, year int) tea.Cmd`.

- [ ] **Step 1: Write the failing test**

Add to `internal/adapters/tui/panel_admin_test.go` (add imports `"encoding/json"` if convenient, or write the file body as a raw string):

```go
func TestAdminPanel_Import_CreatesForecasts(t *testing.T) {
	deps, q := testDeps(t)
	ctx := context.Background()

	// Seed an OPEN 2025 year with taxonomy a1 and partner 7.
	seedWindow(t, q, 2025, model.WindowOpen)
	tax := persistence.NewTaxonomyRepository(q)
	ta, _ := model.NewExpenseType(2025, "A", "[a]", model.CategoryCurrent)
	_ = tax.SaveType(ctx, ta)
	sa, _ := model.NewExpenseSubtype(2025, "a1", "[a1]", "A")
	_ = tax.SaveSubtype(ctx, sa)
	p7, _ := model.NewPartner(7, "Soci", "", "", "s7@e.test", "", model.Productor, 0,
		time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), false)
	_ = persistence.NewPartnerRepository(q).Save(ctx, p7)

	// Write the import file into Cfg.ImportDir.
	if err := os.MkdirAll(deps.Cfg.ImportDir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := `{"year":2025,"forecasts":[
	  {"partnerId":7,"scope":"COMMON","subtypeCode":"a1","concept":"Assegurança","grossAmount":"2880.00","plannedDate":"2025-06-15"},
	  {"partnerId":7,"scope":"COMMON","subtypeCode":"a1","concept":"Segona","grossAmount":"100.00","plannedDate":"2025-07-01"}
	]}`
	if err := os.WriteFile(filepath.Join(deps.Cfg.ImportDir, "2025-forecasts.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	p := NewAdminPanel(deps)
	p, _ = p.Update(yearSelectedMsg{Year: 2025})
	_, cmd := p.Update(pKey("i"))
	msg := runCmd(t, cmd).(forecastsImportedMsg)
	if msg.err != nil {
		t.Fatalf("import error: %v", msg.err)
	}
	if msg.result.Created != 2 {
		t.Errorf("Created = %d, want 2", msg.result.Created)
	}
	p, _ = p.Update(msg)
	if got := p.Detail(); !strings.Contains(got, "Importats 2") {
		t.Errorf("Detail = %q, want it to mention Importats 2", got)
	}
}

func TestAdminPanel_Import_ClosedYearSurfacesError(t *testing.T) {
	deps, q := testDeps(t)
	seedWindow(t, q, 2025, model.WindowDraft) // not OPEN
	if err := os.MkdirAll(deps.Cfg.ImportDir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := `{"year":2025,"forecasts":[{"partnerId":7,"scope":"COMMON","subtypeCode":"a1","concept":"x","grossAmount":"1.00","plannedDate":"2025-06-15"}]}`
	if err := os.WriteFile(filepath.Join(deps.Cfg.ImportDir, "2025-forecasts.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	p := NewAdminPanel(deps)
	p, _ = p.Update(yearSelectedMsg{Year: 2025})
	_, cmd := p.Update(pKey("i"))
	msg := runCmd(t, cmd).(forecastsImportedMsg)
	if msg.err == nil {
		t.Fatal("expected error importing into a non-OPEN year")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapters/tui/ -run AdminPanel_Import -v`
Expected: FAIL — `forecastsImportedMsg` / `importForecastsCmd` undefined; `i` key does nothing.

- [ ] **Step 3: Implement the `i` handler and import command**

In `internal/adapters/tui/panel_admin.go`, add `"path/filepath"`, `"github.com/pjover/espigol/internal/adapters/importer"`, and `"github.com/pjover/espigol/internal/application"` to the imports. Add the message type and command near the other message types:

```go
// forecastsImportedMsg carries the outcome of importForecastsCmd.
type forecastsImportedMsg struct {
	year   int
	result application.ImportResult
	err    error
}

// importForecastsCmd loads Home/import/<year>-forecasts.json and replaces the
// year's forecasts via AdminImport (which requires an OPEN window).
func importForecastsCmd(deps Deps, year int) tea.Cmd {
	return func() tea.Msg {
		path := filepath.Join(deps.Cfg.ImportDir, fmt.Sprintf("%d-forecasts.json", year))
		entries, err := importer.Load(path, year)
		if err != nil {
			return forecastsImportedMsg{year: year, err: err}
		}
		adminEmail := ""
		if deps.Cfg != nil {
			adminEmail = deps.Cfg.Admin.Email
		}
		res, err := deps.Forecasts.AdminImport(context.Background(), adminEmail, year, entries)
		return forecastsImportedMsg{year: year, result: res, err: err}
	}
}
```

Add the `i` case to `handleKey`:

```go
func (p adminPanel) handleKey(msg tea.KeyMsg) (Panel, tea.Cmd) {
	switch msg.String() {
	case "f":
		return p, p.findWindowStateCmd(p.year)
	case "i":
		return p, importForecastsCmd(p.deps, p.year)
	}
	return p, nil
}
```

Add the `forecastsImportedMsg` case to `Update` (next to `reportDoneMsg`):

```go
	case forecastsImportedMsg:
		if msg.year != p.year {
			return p, nil
		}
		if msg.err != nil {
			p.result = &adminResult{err: msg.err}
		} else {
			p.result = &adminResult{text: fmt.Sprintf("Importats %d (esborrats %d)", msg.result.Created, msg.result.Deleted)}
		}
		return p, p.loadYearsCmd()
```

Add the import action to `Actions()`:

```go
func (p adminPanel) Actions() []Action {
	return []Action{
		{Key: "f", Label: "genera informe"},
		{Key: "i", Label: "importa previsions"},
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapters/tui/ -run AdminPanel_Import -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/tui/panel_admin.go internal/adapters/tui/panel_admin_test.go
git commit -m "feat(tui): Admin panel forecast import (i)"
```

---

### Task 8: Backup key `b`

**Files:**
- Modify: `internal/adapters/tui/panel_admin.go` (add `b` handler, `backupCmd`, `backupDoneMsg`, Actions entry)
- Test: `internal/adapters/tui/panel_admin_test.go` (new `TestAdminPanel_Backup_*`)

**Interfaces:**
- Consumes: `Deps.Backup.Backup` (Task 5/6).
- Produces: `type backupDoneMsg struct { path string; err error }`; `func backupCmd(deps Deps) tea.Cmd`.

- [ ] **Step 1: Write the failing test**

Add to `internal/adapters/tui/panel_admin_test.go`:

```go
func TestAdminPanel_Backup_CreatesFileAndShowsPath(t *testing.T) {
	deps, _ := testDeps(t)
	p := NewAdminPanel(deps)
	_, cmd := p.Update(pKey("b"))
	msg := runCmd(t, cmd).(backupDoneMsg)
	if msg.err != nil {
		t.Fatalf("backup error: %v", msg.err)
	}
	if _, err := os.Stat(msg.path); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
	p, _ = p.Update(msg)
	if got := p.Detail(); !strings.Contains(got, msg.path) {
		t.Errorf("Detail = %q, want it to contain the backup path", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapters/tui/ -run AdminPanel_Backup -v`
Expected: FAIL — `backupDoneMsg` undefined; `b` key does nothing.

- [ ] **Step 3: Implement the `b` handler and backup command**

In `internal/adapters/tui/panel_admin.go` add:

```go
// backupDoneMsg carries the outcome of backupCmd.
type backupDoneMsg struct {
	path string
	err  error
}

func backupCmd(deps Deps) tea.Cmd {
	return func() tea.Msg {
		path, err := deps.Backup.Backup(context.Background())
		return backupDoneMsg{path: path, err: err}
	}
}
```

Add the `b` case to `handleKey`:

```go
	case "b":
		return p, backupCmd(p.deps)
```

Add the `backupDoneMsg` case to `Update`:

```go
	case backupDoneMsg:
		if msg.err != nil {
			p.result = &adminResult{err: msg.err}
		} else {
			p.result = &adminResult{text: "Còpia de seguretat creada:\n  " + msg.path}
		}
		return p, nil
```

Add the action:

```go
		{Key: "b", Label: "còpia de seguretat"},
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapters/tui/ -run AdminPanel_Backup -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/tui/panel_admin.go internal/adapters/tui/panel_admin_test.go
git commit -m "feat(tui): Admin panel database backup (b)"
```

---

### Task 9: Restore key `r` + backup selector modal

**Files:**
- Create: `internal/adapters/tui/backup_select_modal.go`
- Modify: `internal/adapters/tui/panel_admin.go` (`r` handler, `restoreStagedMsg` case, Actions entry)
- Test: `internal/adapters/tui/backup_select_modal_test.go`; add `TestAdminPanel_Restore_EmptyList` to `panel_admin_test.go`

**Interfaces:**
- Consumes: `Deps.Backup.ListBackups`/`StageRestore` (Task 5), `openModalCmd`, `closeModalCmd`, `titleStyle`, `focusedPanelStyle`, `helpStyle`, `modalStyle`.
- Produces:
  - `type backupSelectModal struct { deps Deps; files []backup.BackupFile; cursor int }` implementing `tea.Model`
  - `func newBackupSelectModal(deps Deps, files []backup.BackupFile) backupSelectModal`
  - `type restoreStagedMsg struct { name string; err error }`; `func stageRestoreCmd(deps Deps, path string) tea.Cmd`

- [ ] **Step 1: Write the failing tests**

Create `internal/adapters/tui/backup_select_modal_test.go`:

```go
package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pjover/espigol/internal/adapters/persistence/backup"
)

func TestBackupSelectModal_EnterStagesRestore(t *testing.T) {
	deps, _ := testDeps(t)
	// Create a real backup to select.
	src, err := deps.Backup.Backup(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	files, err := deps.Backup.ListBackups()
	if err != nil || len(files) == 0 {
		t.Fatalf("ListBackups: %v (n=%d)", err, len(files))
	}

	m := newBackupSelectModal(deps, files)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command after Enter")
	}
	// The batch includes stageRestoreCmd (produces restoreStagedMsg) and
	// closeModalCmd; run the batch and find the restoreStagedMsg.
	msg := runCmd(t, cmd)
	staged := findRestoreStaged(t, msg)
	if staged.err != nil {
		t.Fatalf("stage error: %v", staged.err)
	}
	if staged.name != filepath.Base(src) {
		t.Errorf("staged name = %q, want %q", staged.name, filepath.Base(src))
	}
	pending := filepath.Join(deps.Cfg.Home, "restore-pending.db")
	if _, err := os.Stat(pending); err != nil {
		t.Errorf("restore-pending.db not written: %v", err)
	}
}

// findRestoreStaged extracts a restoreStagedMsg from a possibly-batched message.
func findRestoreStaged(t *testing.T, msg tea.Msg) restoreStagedMsg {
	t.Helper()
	switch m := msg.(type) {
	case restoreStagedMsg:
		return m
	case tea.BatchMsg:
		for _, c := range m {
			if c == nil {
				continue
			}
			if rs := findRestoreStaged(t, c()); rs.name != "" || rs.err != nil {
				return rs
			}
		}
	}
	return restoreStagedMsg{}
}
```

Add to `internal/adapters/tui/panel_admin_test.go`:

```go
func TestAdminPanel_Restore_EmptyListShowsNotice(t *testing.T) {
	deps, _ := testDeps(t) // no backups created
	p := NewAdminPanel(deps)
	p, _ = p.Update(pKey("r"))
	if got := p.Detail(); !strings.Contains(got, "cap còpia") {
		t.Errorf("Detail = %q, want it to mention 'cap còpia'", got)
	}
}
```

Note: `t.Context()` requires Go 1.24+. If the toolchain is older, replace with `context.Background()` and import `context` in the test file.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/adapters/tui/ -run 'BackupSelectModal|Restore' -v`
Expected: FAIL — `newBackupSelectModal` / `restoreStagedMsg` / `r` handler undefined.

- [ ] **Step 3: Implement the selector modal**

Create `internal/adapters/tui/backup_select_modal.go`:

```go
package tui

import (
	"context"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pjover/espigol/internal/adapters/persistence/backup"
)

// backupSelectModal lets the admin pick a backup file to restore. Enter stages
// the chosen file (StageRestore) and closes; Esc cancels. It follows the same
// modalClosedMsg/openModalCmd convention as confirmModal.
type backupSelectModal struct {
	deps   Deps
	files  []backup.BackupFile
	cursor int
}

func newBackupSelectModal(deps Deps, files []backup.BackupFile) backupSelectModal {
	return backupSelectModal{deps: deps, files: files}
}

// Init implements tea.Model.
func (m backupSelectModal) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m backupSelectModal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case "down", "j":
		if m.cursor < len(m.files)-1 {
			m.cursor++
		}
		return m, nil
	case "enter":
		if len(m.files) == 0 {
			return m, closeModalCmd
		}
		chosen := m.files[m.cursor]
		return m, tea.Batch(stageRestoreCmd(m.deps, chosen.Path), closeModalCmd)
	case "esc":
		return m, closeModalCmd
	}
	return m, nil
}

// View implements tea.Model.
func (m backupSelectModal) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Restaura una còpia de seguretat"))
	b.WriteString("\n\n")
	for i, f := range m.files {
		line := "  " + f.Name
		if i == m.cursor {
			line = focusedPanelStyle.Render("> " + f.Name)
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑↓: mou · enter: restaura · esc: cancel·la"))
	return modalStyle.Render(b.String())
}

// restoreStagedMsg carries the outcome of stageRestoreCmd.
type restoreStagedMsg struct {
	name string
	err  error
}

func stageRestoreCmd(deps Deps, path string) tea.Cmd {
	return func() tea.Msg {
		err := deps.Backup.StageRestore(path)
		return restoreStagedMsg{name: filepath.Base(path), err: err}
	}
}

var _ = context.Background // context imported for parity with other modals; remove if unused
```

Remove the trailing `var _ = context.Background` line and the `"context"` import if the compiler flags them as unused (they are there only to avoid an accidental unused-import churn; `stageRestoreCmd` does not need context). Simplest: do **not** import `"context"` — delete that import and the `var _` line.

- [ ] **Step 4: Wire the `r` key and `restoreStagedMsg` into the panel**

In `internal/adapters/tui/panel_admin.go`, add the `r` case to `handleKey`:

```go
	case "r":
		files, err := p.deps.Backup.ListBackups()
		if err != nil {
			p.result = &adminResult{err: err}
			return p, nil
		}
		if len(files) == 0 {
			p.result = &adminResult{text: dimStyle.Render("(cap còpia de seguretat)")}
			return p, nil
		}
		return p, openModalCmd(newBackupSelectModal(p.deps, files))
```

Add the `restoreStagedMsg` case to `Update`:

```go
	case restoreStagedMsg:
		if msg.err != nil {
			p.result = &adminResult{err: msg.err}
		} else {
			p.result = &adminResult{text: fmt.Sprintf("Restauració preparada: %s\nEs restaurarà en reiniciar l'aplicació.", msg.name)}
		}
		return p, nil
```

Add the action:

```go
		{Key: "r", Label: "restaura"},
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/adapters/tui/ -run 'BackupSelectModal|Restore' -v`
Expected: PASS.

- [ ] **Step 6: Full build, vet, and whole-suite test**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS across all packages.

- [ ] **Step 7: Commit**

```bash
git add internal/adapters/tui/backup_select_modal.go internal/adapters/tui/backup_select_modal_test.go internal/adapters/tui/panel_admin.go internal/adapters/tui/panel_admin_test.go
git commit -m "feat(tui): Admin panel database restore (r) with backup picker"
```

---

## Self-Review

**Spec coverage:**
- Panel rename `Informes`→`Admin`, `[6] Admin` label → Task 6 ✅
- Report on `f` → Task 6 ✅
- Import `i`, selected-year, OPEN required, replace-all, atomic, reference validation → Tasks 3, 7 ✅
- JSON format (`COMMON`/`SECTION`/`PARTNER`, sectionCode iff SECTION, money/date strings, year guard) → Task 4 ✅
- Fresh imported forecasts (approved 0 / enabled true / addedOn now) → Task 3 ✅
- `import/` dir + `ImportDir` → Task 1 ✅
- Backup `b` via `VACUUM INTO`, timestamped, keep-all → Tasks 5, 8 ✅
- Restore `r`, pick from list, safety-backup first, staged `restore-pending.db` → Tasks 5, 9 ✅
- Restore applied on next launch in `db.Open` → Task 2 ✅
- Context list "Anys amb informe desat" retained → Task 6 ✅
- Audit per created/deleted forecast on import; none for backup/restore → Task 3 ✅

**Placeholder scan:** No TBD/TODO; every code and test step is complete. Two conditional notes (drop unused `context` import in Task 9; `t.Context()` vs `context.Background()` for older Go) are explicit instructions, not placeholders.

**Type consistency:** `adminResult`, `forecastsImportedMsg`, `backupDoneMsg`, `restoreStagedMsg`, `windowStateMsg`, `reportYearsLoadedMsg` are defined once (Tasks 6–9) and used consistently. `ForecastImportEntry`/`ImportResult` (Task 3) match the importer producer (Task 4) and panel consumer (Task 7). `backup.Backuper`/`BackupFile`/`New` (Task 5) match `Deps.Backup` (Task 6) and the modal (Task 9). `db.ApplyPendingRestore` signature (Task 2) matches its `db.Open` call site.

## Out of scope (from spec §7)

Backup pruning/retention; immediate in-session restore; importing partners/sections/taxonomy/windows; scheduled/automatic backups.
