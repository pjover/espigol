# Phase 4 — Window Close & Report Snapshot Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An application-layer `WindowService` (CreateYear/Open/Close/Amend) that drives the submission-window state machine and the atomic close — running the Phase-3 allocation, persisting approved amounts, serializing the `ReportData` snapshot to JSON, and inserting a `Report` — all over a `TxManager` unit-of-work, with a `ReportRenderer` port (no-op until Phase 5).

**Architecture:** New `internal/application` package depends only on ports (`TxManager`, `ReportRenderer`, `Clock`) plus the pure Phase-3 `services.Compute`. A persistence `TxManager` runs each operation in one transaction, handing the closure a tx-scoped `RepoSet`. `model.Money` gains `MarshalJSON`/`UnmarshalJSON` so `json.Marshal(reportData)` works.

**Tech Stack:** Go 1.26, `github.com/shopspring/decimal` (via `model.Money`), `modernc.org/sqlite`, sqlc, standard library.

## Global Constraints

- **Module:** `github.com/pjover/espigol`. Go **1.26**. CGO-free.
- **App-layer purity:** `internal/application` imports only `internal/domain/...` (model, model/report, ports, services) + stdlib. **No `database/sql`, no `sqlc`, no adapters.** All DB access goes through `ports.TxManager` / `ports.RepoSet`.
- **No `float64`.** Money is `model.Money`.
- **Atomicity:** every `WindowService` operation runs inside exactly one `tx.WithinTx(ctx, func(ports.RepoSet) error)` closure; any returned error or panic rolls back.
- **All user-facing text is Catalan** in callers; the service returns typed errors (Catalan rendering is the caller's job).
- **Determinism:** when iterating maps for writes/aggregation, sort keys first.
- **Faithful to** `espigol-java` `WindowClosingService` for the close path (collect approved from all detail items; skip-unchanged; one transaction).
- **Phase-2 port signatures (verbatim, do not change):**
  - `WindowRepository`: `Save(ctx, model.SubmissionWindow) error`; `FindByYear(ctx, int) (model.SubmissionWindow, bool, error)`; `List(ctx) ([]model.SubmissionWindow, error)`.
  - `ForecastRepository`: `Save(ctx, model.ExpenseForecast) error`; `ListByYear(ctx, int) ([]model.ExpenseForecast, error)` (+ `Create`, `FindByID` unused here).
  - `TaxonomyRepository`: `SaveType`, `SaveSubtype`, `ListTypes(ctx,int)`, `ListSubtypes(ctx,int)`.
  - `PartnerRepository.List(ctx)`; `SectionRepository.List(ctx)` (+ new `ListMemberships`).
  - `ReportRepository`: `Insert(ctx, model.Report) (int, error)`; `FindLatestByYear(ctx,int) (model.Report,bool,error)`; `MarkSuperseded(ctx,int,time.Time) error`.
  - `AuditLog.Append(ctx, model.AuditEvent) error`.
- **Phase-2 model API:** `model.Money` (`String()`, `MoneyFromString`, `Cmp`, `MoneyOf`); `model.NewSubmissionWindow(year, state, openedAt,closedAt *time.Time, deadline time.Time, current,investment Money)`; `SubmissionWindow` accessors `Year/State/Deadline/CurrentExpenseLimit/InvestmentExpenseLimit/WithState/WithOpenedAt/WithClosedAt`; `model.WindowDraft/WindowOpen/WindowClosed`; `model.NewExpenseType/NewExpenseSubtype`; `ExpenseForecast.Enabled()/ApprovedAmount()/ApprovedOn()/WithApprovedAmount/WithApprovedOn/ID()`; `model.NewReport(id,year,generatedAt,snapshotJSON,pdf,supersededAt)`; `model.NewAuditEvent(id,*int,email,kind,entityType,entityID,ts,*string)`; `model.AuditWindowOpened/AuditWindowClosed/AuditReportGenerated`; `model.CategoryCurrent/CategoryInvestment`.
- **Phase-3:** `services.Compute(services.AllocationInput) (report.ReportData, error)`; `report.ReportData` tree (`Categories[].Common.Items`, `.Sections.SectionDetails[].Items`, `.Sections.Partners.PartnerDetails[].Items`, each `DetailItem{CpCode, ApprovedAmount}`).
- **Persistence wiring facts:** repo constructors take `*sqlc.Queries` (except `NewForecastRepository(conn *sql.DB, q *sqlc.Queries)`); `sqlc.New(db DBTX) *Queries`; `(*Queries).WithTx(tx *sql.Tx) *Queries`; `db.Open(path) (*sql.DB, error)`.
- **TDD:** failing test first; commit after each green step.

---

### Task 1: Money JSON marshaling

**Files:**
- Modify: `internal/domain/model/money.go`
- Test: `internal/domain/model/money_json_test.go`

**Interfaces:**
- Produces: `(Money).MarshalJSON() ([]byte, error)` and `(*Money).UnmarshalJSON([]byte) error` — JSON string form of the canonical decimal (e.g. `"31900.00"`).

- [ ] **Step 1: Write the failing test**

Create `internal/domain/model/money_json_test.go`:
```go
package model

import (
	"encoding/json"
	"testing"
)

func TestMoney_JSONRoundTrip(t *testing.T) {
	for _, s := range []string{"31900.00", "1322.22", "0.00", "-5.00"} {
		m, err := MoneyFromString(s)
		if err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if string(b) != `"`+s+`"` {
			t.Errorf("Marshal(%s) = %s, want %q", s, b, `"`+s+`"`)
		}
		var back Money
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if back.Cmp(m) != 0 || back.String() != s {
			t.Errorf("round trip %s -> %s", s, back.String())
		}
	}
}

func TestMoney_JSONInStruct(t *testing.T) {
	type wrap struct {
		Amount Money `json:"amount"`
	}
	m, _ := MoneyFromString("12.50")
	b, err := json.Marshal(wrap{Amount: m})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"amount":"12.50"}` {
		t.Errorf("got %s", b)
	}
	var w wrap
	if err := json.Unmarshal(b, &w); err != nil {
		t.Fatal(err)
	}
	if w.Amount.String() != "12.50" {
		t.Errorf("struct round trip = %s", w.Amount.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/model/ -run TestMoney_JSON -v`
Expected: FAIL — `json: unsupported type` or wrong output (no custom marshaler).

- [ ] **Step 3: Add the marshaler methods**

In `internal/domain/model/money.go`, add `"encoding/json"` to the import block and append:
```go
// MarshalJSON renders Money as its canonical decimal string, e.g. "31900.00".
func (m Money) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.String())
}

// UnmarshalJSON parses a decimal string (as produced by MarshalJSON) into Money.
func (m *Money) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsed, err := MoneyFromString(s)
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/model/ -v && go build ./...`
Expected: PASS, build clean.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/model/money.go internal/domain/model/money_json_test.go
git commit -m "feat(model): JSON marshaling for Money (decimal string)"
```

---

### Task 2: ReportRenderer port + no-op renderer

**Files:**
- Create: `internal/domain/ports/renderer.go`
- Create: `internal/adapters/report/noop.go`
- Test: `internal/adapters/report/noop_test.go`

**Interfaces:**
- Produces: `ports.ReportRenderer interface { Render(rd report.ReportData, generatedAt time.Time) ([]byte, error) }`; `report.NoopRenderer` (package `report` at `internal/adapters/report`) implementing it, returning `[]byte{}`.

- [ ] **Step 1: Write the port**

Create `internal/domain/ports/renderer.go`:
```go
package ports

import (
	"time"

	"github.com/pjover/espigol/internal/domain/model/report"
)

// ReportRenderer renders a computed ReportData into a document (e.g. PDF).
// Phase 4 uses a no-op; Phase 5 provides the real maroto/Markdown renderer.
type ReportRenderer interface {
	Render(rd report.ReportData, generatedAt time.Time) ([]byte, error)
}
```

- [ ] **Step 2: Write the failing test**

Create `internal/adapters/report/noop_test.go`:
```go
package report

import (
	"testing"
	"time"

	reportmodel "github.com/pjover/espigol/internal/domain/model/report"
	"github.com/pjover/espigol/internal/domain/ports"
)

func TestNoopRenderer_ReturnsEmpty(t *testing.T) {
	var r ports.ReportRenderer = NoopRenderer{}
	out, err := r.Render(reportmodel.ReportData{Year: 2026}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || len(out) != 0 {
		t.Errorf("want empty non-nil []byte, got %v (len %d)", out, len(out))
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/adapters/report/ -v`
Expected: FAIL — undefined `NoopRenderer`.

- [ ] **Step 4: Write the no-op renderer**

Create `internal/adapters/report/noop.go`:
```go
// Package report holds ReportRenderer adapters. Phase 4 ships only a no-op;
// Phase 5 adds the maroto PDF and Markdown renderers here.
package report

import (
	"time"

	reportmodel "github.com/pjover/espigol/internal/domain/model/report"
)

// NoopRenderer is a placeholder ReportRenderer that produces no document.
// It exists so the window-close flow can run before PDF rendering lands (Phase 5).
type NoopRenderer struct{}

// Render returns an empty (non-nil) byte slice, which satisfies the
// report.pdf BLOB NOT NULL column without producing an actual document.
func (NoopRenderer) Render(rd reportmodel.ReportData, generatedAt time.Time) ([]byte, error) {
	return []byte{}, nil
}
```

- [ ] **Step 5: Run test + build**

Run: `go test ./internal/adapters/report/ -v && go build ./...`
Expected: PASS, build clean.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/ports/renderer.go internal/adapters/report/
git commit -m "feat(ports): ReportRenderer port + no-op renderer"
```

---

### Task 3: Sections.ListMemberships

**Files:**
- Modify: `internal/domain/ports/ports.go` (add method to `SectionRepository`)
- Create: `db/queries` entry (append to `db/queries/section.sql`)
- Modify: `internal/adapters/persistence/section_repository.go`
- Test: `internal/adapters/persistence/section_repository_test.go` (add a test)

**Interfaces:**
- Produces: `SectionRepository.ListMemberships(ctx context.Context) ([]model.PartnerSection, error)` — all partner-section rows.

- [ ] **Step 1: Add the query and regenerate sqlc**

Append to `db/queries/section.sql`:
```sql
-- name: ListAllPartnerSections :many
SELECT partner_id, section_code FROM partner_section ORDER BY partner_id, section_code;
```
Run: `make sqlc-generate`
Expected: generates `ListAllPartnerSections`. Read the generated row type (likely `[]sqlc.PartnerSection`).

- [ ] **Step 2: Add the method to the port**

In `internal/domain/ports/ports.go`, add to the `SectionRepository` interface:
```go
	ListMemberships(ctx context.Context) ([]model.PartnerSection, error)
```

- [ ] **Step 3: Write the failing test**

Add to `internal/adapters/persistence/section_repository_test.go`:
```go
func TestSectionRepository_ListMemberships(t *testing.T) {
	q := openTestDB(t)
	pr := persistence.NewPartnerRepository(q)
	sr := persistence.NewSectionRepository(q)
	ctx := context.Background()

	oliva, _ := model.NewSection("oliva", "Secció d'oliva", true, 1)
	ram, _ := model.NewSection("ramaderia", "Secció de ramaderia", true, 2)
	_ = sr.Save(ctx, oliva)
	_ = sr.Save(ctx, ram)
	p1, _ := model.NewPartner(1, "A", "", "", "a@e.test", "", model.Productor, 1, time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), false)
	p2, _ := model.NewPartner(2, "B", "", "", "b@e.test", "", model.Productor, 1, time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), false)
	_ = pr.Save(ctx, p1)
	_ = pr.Save(ctx, p2)
	m1, _ := model.NewPartnerSection(1, "oliva")
	m2, _ := model.NewPartnerSection(1, "ramaderia")
	m3, _ := model.NewPartnerSection(2, "oliva")
	for _, m := range []model.PartnerSection{m1, m2, m3} {
		if err := sr.AddMembership(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	all, err := sr.ListMemberships(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("ListMemberships = %d rows, want 3", len(all))
	}
}
```
(Ensure the test file imports `"time"`.)

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/adapters/persistence/ -run TestSectionRepository_ListMemberships -v`
Expected: FAIL — `ListMemberships` undefined.

- [ ] **Step 5: Implement the method**

Add to `internal/adapters/persistence/section_repository.go`:
```go
func (r *SectionRepository) ListMemberships(ctx context.Context) ([]model.PartnerSection, error) {
	rows, err := r.q.ListAllPartnerSections(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.PartnerSection, 0, len(rows))
	for _, row := range rows {
		m, err := mapper.PartnerSectionFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}
```
(If sqlc named the generated row type differently, adjust `row`'s type usage; `mapper.PartnerSectionFromRow` already accepts `sqlc.PartnerSection`.)

- [ ] **Step 6: Run tests + build**

Run: `go test ./internal/adapters/persistence/... -v && go build ./...`
Expected: PASS (the `ports.SectionRepository` compile-assertion in `ports_check.go` still holds).

- [ ] **Step 7: Commit**

```bash
git add db/queries/section.sql internal/adapters/persistence/ internal/domain/ports/ports.go
git commit -m "feat(persistence): SectionRepository.ListMemberships (all rows)"
```

---

### Task 4: TxManager port + RepoSet + persistence impl

**Files:**
- Create: `internal/domain/ports/tx.go`
- Create: `internal/adapters/persistence/txmanager.go`
- Test: `internal/adapters/persistence/txmanager_test.go`

**Interfaces:**
- Produces:
  - `ports.RepoSet` struct (fields: `Partners PartnerRepository; Forecasts ForecastRepository; Windows WindowRepository; Taxonomy TaxonomyRepository; Sections SectionRepository; Reports ReportRepository; Audit AuditLog`).
  - `ports.TxManager interface { WithinTx(ctx context.Context, fn func(RepoSet) error) error }`.
  - `persistence.NewTxManager(db *sql.DB) *TxManager` implementing `ports.TxManager`.

- [ ] **Step 1: Write the port**

Create `internal/domain/ports/tx.go`:
```go
package ports

import "context"

// RepoSet is the set of transaction-scoped repositories handed to a WithinTx
// closure. All share one transaction.
type RepoSet struct {
	Partners  PartnerRepository
	Forecasts ForecastRepository
	Windows   WindowRepository
	Taxonomy  TaxonomyRepository
	Sections  SectionRepository
	Reports   ReportRepository
	Audit     AuditLog
}

// TxManager runs a unit of work inside a single database transaction, handing
// the closure a RepoSet bound to that transaction. The transaction commits if
// fn returns nil, and rolls back on error or panic.
type TxManager interface {
	WithinTx(ctx context.Context, fn func(RepoSet) error) error
}
```

- [ ] **Step 2: Write the failing test**

Create `internal/adapters/persistence/txmanager_test.go`:
```go
package persistence_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/ports"
)

func newTxManager(t *testing.T) (*persistence.TxManager, ports.PartnerRepository) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "tx.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	tm := persistence.NewTxManager(conn)
	// a non-tx repo for reading committed state
	return tm, persistence.NewPartnerRepository(sqlcQueries(conn))
}

func samplePartner(t *testing.T, id int) model.Partner {
	t.Helper()
	p, err := model.NewPartner(id, "P", "", "", "p"+string(rune('0'+id))+"@e.test", "", model.Productor, 1, time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), false)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTxManager_CommitsOnSuccess(t *testing.T) {
	tm, reader := newTxManager(t)
	ctx := context.Background()
	if err := tm.WithinTx(ctx, func(r ports.RepoSet) error {
		return r.Partners.Save(ctx, samplePartner(t, 1))
	}); err != nil {
		t.Fatal(err)
	}
	_, found, err := reader.FindByID(ctx, 1)
	if err != nil || !found {
		t.Errorf("partner should be committed: found=%v err=%v", found, err)
	}
}

func TestTxManager_RollsBackOnError(t *testing.T) {
	tm, reader := newTxManager(t)
	ctx := context.Background()
	sentinel := errors.New("boom")
	err := tm.WithinTx(ctx, func(r ports.RepoSet) error {
		if e := r.Partners.Save(ctx, samplePartner(t, 2)); e != nil {
			return e
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel, got %v", err)
	}
	_, found, _ := reader.FindByID(ctx, 2)
	if found {
		t.Errorf("partner must have been rolled back")
	}
}
```
Add a small helper in this test file to build a `*sqlc.Queries` for the reader:
```go
func sqlcQueries(conn *sql.DB) *sqlc.Queries { return sqlc.New(conn) }
```
with imports `"database/sql"` and `"github.com/pjover/espigol/internal/adapters/persistence/sqlc"`.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/adapters/persistence/ -run TestTxManager -v`
Expected: FAIL — undefined `NewTxManager`.

- [ ] **Step 4: Implement the TxManager**

Create `internal/adapters/persistence/txmanager.go`:
```go
package persistence

import (
	"context"
	"database/sql"

	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/ports"
)

// TxManager runs units of work in a single SQLite transaction.
type TxManager struct {
	db *sql.DB
}

func NewTxManager(db *sql.DB) *TxManager {
	return &TxManager{db: db}
}

// WithinTx begins a transaction, builds a tx-scoped RepoSet, runs fn, and
// commits on success or rolls back on error/panic.
func (t *TxManager) WithinTx(ctx context.Context, fn func(ports.RepoSet) error) (err error) {
	tx, err := t.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	q := sqlc.New(t.db).WithTx(tx)
	repos := ports.RepoSet{
		Partners:  NewPartnerRepository(q),
		Forecasts: NewForecastRepository(t.db, q),
		Windows:   NewWindowRepository(q),
		Taxonomy:  NewTaxonomyRepository(q),
		Sections:  NewSectionRepository(q),
		Reports:   NewReportRepository(q),
		Audit:     NewAuditLog(q),
	}
	if err := fn(repos); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

var _ ports.TxManager = (*TxManager)(nil)
```
Note: `Forecasts` is built with `(t.db, q)`; its `Save`/`ListByYear` use the tx-scoped `q`, so they participate in the transaction. Its `Create` (which opens its own tx) is **not** used inside `WithinTx` closures.

- [ ] **Step 5: Run tests + build**

Run: `go test ./internal/adapters/persistence/ -run TestTxManager -v && go build ./...`
Expected: PASS (commit + rollback), build clean.

- [ ] **Step 6: Commit**

```bash
git add internal/domain/ports/tx.go internal/adapters/persistence/txmanager.go internal/adapters/persistence/txmanager_test.go
git commit -m "feat: TxManager unit-of-work port + persistence impl"
```

---

### Task 5: Snapshot serialization helpers

**Files:**
- Create: `internal/application/snapshot.go`
- Test: `internal/application/snapshot_test.go`

**Interfaces:**
- Produces: `application.SnapshotToJSON(report.ReportData) (string, error)` and `application.SnapshotFromJSON(string) (report.ReportData, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/application/snapshot_test.go`:
```go
package application

import (
	"testing"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/model/report"
)

func TestSnapshotRoundTrip(t *testing.T) {
	rd := report.ReportData{
		Year:                 2026,
		HasNegativeRemainder: false,
		Categories: []report.CategoryReportData{{
			Category: model.CategoryInvestment,
			Common: report.CommonData{
				Available: model.MoneyOf(70000), Total: model.MoneyOf(31900), Remainder: model.MoneyOf(38100),
			},
			Sections: report.SectionsData{
				Available: model.MoneyOf(38100), Total: model.MoneyOf(3398), Remainder: model.MoneyOf(34702),
				Partners: report.PartnersData{
					GrandTotal: mustMoney(t, "23498.96"), FinalRemainder: mustMoney(t, "11203.04"),
				},
			},
		}},
	}
	js, err := SnapshotToJSON(rd)
	if err != nil {
		t.Fatal(err)
	}
	back, err := SnapshotFromJSON(js)
	if err != nil {
		t.Fatal(err)
	}
	if back.Categories[0].Sections.Partners.GrandTotal.String() != "23498.96" {
		t.Errorf("GrandTotal round trip = %s", back.Categories[0].Sections.Partners.GrandTotal.String())
	}
	if back.Categories[0].Common.Total.String() != "31900.00" {
		t.Errorf("Common.Total round trip = %s", back.Categories[0].Common.Total.String())
	}
	if back.Year != 2026 || back.Categories[0].Category != model.CategoryInvestment {
		t.Errorf("scalar fields lost: %+v", back)
	}
}

func mustMoney(t *testing.T, s string) model.Money {
	t.Helper()
	m, err := model.MoneyFromString(s)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/application/ -run TestSnapshotRoundTrip -v`
Expected: FAIL — undefined `SnapshotToJSON`.

- [ ] **Step 3: Implement the helpers**

Create `internal/application/snapshot.go`:
```go
// Package application orchestrates the window lifecycle over the domain ports
// and the pure allocation service.
package application

import (
	"encoding/json"

	"github.com/pjover/espigol/internal/domain/model/report"
)

// SnapshotToJSON serializes a computed ReportData to the JSON stored on a Report row.
func SnapshotToJSON(rd report.ReportData) (string, error) {
	b, err := json.Marshal(rd)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// SnapshotFromJSON deserializes a stored snapshot back into ReportData.
func SnapshotFromJSON(s string) (report.ReportData, error) {
	var rd report.ReportData
	if err := json.Unmarshal([]byte(s), &rd); err != nil {
		return report.ReportData{}, err
	}
	return rd, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/application/ -v && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/application/snapshot.go internal/application/snapshot_test.go
git commit -m "feat(application): ReportData snapshot JSON helpers"
```

---

### Task 6: WindowService — CreateYear + Open (+ errors)

**Files:**
- Create: `internal/application/errors.go`
- Create: `internal/application/window_service.go`
- Test: `internal/application/window_service_test.go`

**Interfaces:**
- Consumes: `ports.TxManager`, `ports.ReportRenderer`, `ports.Clock`; `services` (later tasks); `model`.
- Produces:
  - typed errors: `ErrYearExists`, `ErrNoPriorYear`, `ErrWindowNotFound`, `ErrWrongState`, `ErrDeadlinePassed`, `ErrIncompleteTaxonomy`, `ErrAnotherWindowOpen`.
  - `NewWindowService(tx ports.TxManager, renderer ports.ReportRenderer, clock ports.Clock) *WindowService`.
  - `(*WindowService).CreateYear(ctx, year int) (model.SubmissionWindow, error)`.
  - `(*WindowService).Open(ctx, year int) error`.

- [ ] **Step 1: Write errors.go**

Create `internal/application/errors.go`:
```go
package application

import "errors"

var (
	ErrYearExists         = errors.New("submission window already exists for that year")
	ErrNoPriorYear        = errors.New("no prior year to copy taxonomy and limits from")
	ErrWindowNotFound     = errors.New("submission window not found")
	ErrWrongState         = errors.New("operation not allowed in the window's current state")
	ErrDeadlinePassed     = errors.New("deadline must be in the future to open the window")
	ErrIncompleteTaxonomy = errors.New("taxonomy must define at least one CURRENT and one INVESTMENT type")
	ErrAnotherWindowOpen  = errors.New("another submission window is already open")
)
```

- [ ] **Step 2: Write the failing test**

Create `internal/application/window_service_test.go`:
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
	appreport "github.com/pjover/espigol/internal/adapters/report"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/ports"
)

// fixedClock is a deterministic Clock.
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func newSvc(t *testing.T) (*application.WindowService, *sql.DB) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "svc.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	clock := fixedClock{t: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)}
	svc := application.NewWindowService(persistence.NewTxManager(conn), appreport.NoopRenderer{}, clock)
	return svc, conn
}

// seedClosedYear creates a CLOSED window for `year` with a minimal taxonomy
// directly via repositories (so CreateYear/Open have a prior year to copy).
func seedClosedYear(t *testing.T, conn *sql.DB, year int) {
	t.Helper()
	q := sqlc.New(conn)
	ctx := context.Background()
	w, _ := model.NewSubmissionWindow(year, model.WindowClosed, nil, nil,
		time.Date(year, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	if err := persistence.NewWindowRepository(q).Save(ctx, w); err != nil {
		t.Fatal(err)
	}
	tax := persistence.NewTaxonomyRepository(q)
	ta, _ := model.NewExpenseType(year, "A", "[a]", model.CategoryCurrent)
	tb, _ := model.NewExpenseType(year, "B", "[b]", model.CategoryInvestment)
	_ = tax.SaveType(ctx, ta)
	_ = tax.SaveType(ctx, tb)
	sa, _ := model.NewExpenseSubtype(year, "a1", "[a1]", "A")
	sb, _ := model.NewExpenseSubtype(year, "b1", "[b1]", "B")
	_ = tax.SaveSubtype(ctx, sa)
	_ = tax.SaveSubtype(ctx, sb)
}

func TestCreateYear_CopiesTaxonomyAndLimits(t *testing.T) {
	svc, conn := newSvc(t)
	seedClosedYear(t, conn, 2026)
	ctx := context.Background()

	w, err := svc.CreateYear(ctx, 2027)
	if err != nil {
		t.Fatal(err)
	}
	if w.State() != model.WindowDraft || w.Year() != 2027 {
		t.Errorf("new window = %+v", w)
	}
	if w.CurrentExpenseLimit().String() != "30000.00" || w.InvestmentExpenseLimit().String() != "70000.00" {
		t.Errorf("limits not copied: %s / %s", w.CurrentExpenseLimit(), w.InvestmentExpenseLimit())
	}
	tax := persistence.NewTaxonomyRepository(sqlc.New(conn))
	types, _ := tax.ListTypes(ctx, 2027)
	subs, _ := tax.ListSubtypes(ctx, 2027)
	if len(types) != 2 || len(subs) != 2 {
		t.Errorf("taxonomy not copied: types=%d subs=%d", len(types), len(subs))
	}
}

func TestCreateYear_Errors(t *testing.T) {
	svc, conn := newSvc(t)
	ctx := context.Background()
	if _, err := svc.CreateYear(ctx, 2027); !errors.Is(err, application.ErrNoPriorYear) {
		t.Errorf("want ErrNoPriorYear, got %v", err)
	}
	seedClosedYear(t, conn, 2026)
	if _, err := svc.CreateYear(ctx, 2026); !errors.Is(err, application.ErrYearExists) {
		t.Errorf("want ErrYearExists, got %v", err)
	}
}

func TestOpen_HappyPathAndValidations(t *testing.T) {
	svc, conn := newSvc(t)
	seedClosedYear(t, conn, 2026)
	ctx := context.Background()
	if _, err := svc.CreateYear(ctx, 2027); err != nil {
		t.Fatal(err)
	}

	if err := svc.Open(ctx, 2027); err != nil {
		t.Fatalf("open: %v", err)
	}
	wr := persistence.NewWindowRepository(sqlc.New(conn))
	w, _, _ := wr.FindByYear(ctx, 2027)
	if w.State() != model.WindowOpen || w.OpenedAt() == nil {
		t.Errorf("window not opened: %+v", w)
	}
	// re-open a non-DRAFT window
	if err := svc.Open(ctx, 2027); !errors.Is(err, application.ErrWrongState) {
		t.Errorf("want ErrWrongState reopening, got %v", err)
	}
	// audit written
	audits, _ := persistence.NewAuditLog(sqlc.New(conn)).List(ctx)
	var sawOpen bool
	for _, a := range audits {
		if a.Kind() == model.AuditWindowOpened {
			sawOpen = true
		}
	}
	if !sawOpen {
		t.Errorf("no WINDOW_OPENED audit event")
	}
}

func TestOpen_RejectsAnotherOpen(t *testing.T) {
	svc, conn := newSvc(t)
	seedClosedYear(t, conn, 2026)
	ctx := context.Background()
	_, _ = svc.CreateYear(ctx, 2027)
	_, _ = svc.CreateYear(ctx, 2028)
	if err := svc.Open(ctx, 2027); err != nil {
		t.Fatal(err)
	}
	if err := svc.Open(ctx, 2028); !errors.Is(err, application.ErrAnotherWindowOpen) {
		t.Errorf("want ErrAnotherWindowOpen, got %v", err)
	}
}
```
Add imports `"database/sql"` to this test file (used by `newSvc`/`seedClosedYear`).

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/application/ -run 'TestCreateYear|TestOpen' -v`
Expected: FAIL — undefined `NewWindowService`.

- [ ] **Step 4: Write window_service.go (CreateYear + Open)**

Create `internal/application/window_service.go`:
```go
package application

import (
	"context"
	"strconv"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/ports"
)

// WindowService orchestrates the submission-window lifecycle.
type WindowService struct {
	tx       ports.TxManager
	renderer ports.ReportRenderer
	clock    ports.Clock
}

func NewWindowService(tx ports.TxManager, renderer ports.ReportRenderer, clock ports.Clock) *WindowService {
	return &WindowService{tx: tx, renderer: renderer, clock: clock}
}

// CreateYear creates a new DRAFT window, copying the most recent prior year's
// limits and taxonomy.
func (s *WindowService) CreateYear(ctx context.Context, year int) (model.SubmissionWindow, error) {
	var created model.SubmissionWindow
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		if _, ok, err := r.Windows.FindByYear(ctx, year); err != nil {
			return err
		} else if ok {
			return ErrYearExists
		}

		all, err := r.Windows.List(ctx)
		if err != nil {
			return err
		}
		prior, ok := mostRecentPrior(all, year)
		if !ok {
			return ErrNoPriorYear
		}

		deadline := time.Date(year, time.December, 31, 23, 59, 59, 0, time.UTC)
		w, err := model.NewSubmissionWindow(year, model.WindowDraft, nil, nil, deadline,
			prior.CurrentExpenseLimit(), prior.InvestmentExpenseLimit())
		if err != nil {
			return err
		}
		if err := r.Windows.Save(ctx, w); err != nil {
			return err
		}

		types, err := r.Taxonomy.ListTypes(ctx, prior.Year())
		if err != nil {
			return err
		}
		for _, t := range types {
			nt, err := model.NewExpenseType(year, t.Code(), t.Label(), t.Category())
			if err != nil {
				return err
			}
			if err := r.Taxonomy.SaveType(ctx, nt); err != nil {
				return err
			}
		}
		subs, err := r.Taxonomy.ListSubtypes(ctx, prior.Year())
		if err != nil {
			return err
		}
		for _, st := range subs {
			ns, err := model.NewExpenseSubtype(year, st.Code(), st.Label(), st.TypeCode())
			if err != nil {
				return err
			}
			if err := r.Taxonomy.SaveSubtype(ctx, ns); err != nil {
				return err
			}
		}
		created = w
		return nil
	})
	return created, err
}

// Open transitions a DRAFT window to OPEN after validation.
func (s *WindowService) Open(ctx context.Context, year int) error {
	now := s.clock.Now()
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		w, ok, err := r.Windows.FindByYear(ctx, year)
		if err != nil {
			return err
		}
		if !ok {
			return ErrWindowNotFound
		}
		if w.State() != model.WindowDraft {
			return ErrWrongState
		}
		if !w.Deadline().After(now) {
			return ErrDeadlinePassed
		}

		types, err := r.Taxonomy.ListTypes(ctx, year)
		if err != nil {
			return err
		}
		hasCurrent, hasInvestment := false, false
		for _, t := range types {
			switch t.Category() {
			case model.CategoryCurrent:
				hasCurrent = true
			case model.CategoryInvestment:
				hasInvestment = true
			}
		}
		if !hasCurrent || !hasInvestment {
			return ErrIncompleteTaxonomy
		}

		all, err := r.Windows.List(ctx)
		if err != nil {
			return err
		}
		for _, ow := range all {
			if ow.Year() != year && ow.State() == model.WindowOpen {
				return ErrAnotherWindowOpen
			}
		}

		if err := r.Windows.Save(ctx, w.WithState(model.WindowOpen).WithOpenedAt(now)); err != nil {
			return err
		}
		return appendAudit(ctx, r, model.AuditWindowOpened, year, now, "")
	})
}

// mostRecentPrior returns the window with the greatest year strictly less than `year`.
func mostRecentPrior(all []model.SubmissionWindow, year int) (model.SubmissionWindow, bool) {
	var best model.SubmissionWindow
	found := false
	for _, w := range all {
		if w.Year() < year && (!found || w.Year() > best.Year()) {
			best = w
			found = true
		}
	}
	return best, found
}

// appendAudit writes a system-actor audit event for a window/year.
func appendAudit(ctx context.Context, r ports.RepoSet, kind model.AuditKind, year int, at time.Time, payload string) error {
	var payloadPtr *string
	if payload != "" {
		payloadPtr = &payload
	}
	e, err := model.NewAuditEvent(0, nil, "system@espigol", kind, "SubmissionWindow", strconv.Itoa(year), at, payloadPtr)
	if err != nil {
		return err
	}
	return r.Audit.Append(ctx, e)
}
```
(Task 6 does not import `fmt` — `appendAudit` uses `strconv`. Task 7 adds `fmt` when `Close` formats the audit payload.)

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/application/ -v && go build ./...`
Expected: PASS (CreateYear + Open cases).

- [ ] **Step 6: Commit**

```bash
git add internal/application/errors.go internal/application/window_service.go internal/application/window_service_test.go
git commit -m "feat(application): WindowService CreateYear + Open"
```

---

### Task 7: WindowService — Close

**Files:**
- Modify: `internal/application/window_service.go`
- Test: `internal/application/close_test.go`

**Interfaces:**
- Consumes: `services.Compute`, `report.ReportData`, the renderer, `SnapshotToJSON`.
- Produces: `(*WindowService).Close(ctx, year int) (model.Report, error)`; internal helpers `computeReport`, `collectApproved`, `persistApproved`, `buildSubtypeCategory`.

- [ ] **Step 1: Write the failing test**

Create `internal/application/close_test.go`:
```go
package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

// seedOpenYearWithForecasts builds an OPEN 2027 window with a tiny scenario:
// common (current) 100, a soci (partner) investment 500; limits 200/1000.
func seedOpenYearWithForecasts(t *testing.T, conn *sql.DB) {
	t.Helper()
	q := sqlc.New(conn)
	ctx := context.Background()
	wr := persistence.NewWindowRepository(q)
	tax := persistence.NewTaxonomyRepository(q)
	pr := persistence.NewPartnerRepository(q)
	fr := persistence.NewForecastRepository(conn, q)

	w, _ := model.NewSubmissionWindow(2027, model.WindowOpen, ptrTime(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)), nil,
		time.Date(2027, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(200), model.MoneyOf(1000))
	_ = wr.Save(ctx, w)
	ta, _ := model.NewExpenseType(2027, "A", "[a]", model.CategoryCurrent)
	tb, _ := model.NewExpenseType(2027, "B", "[b]", model.CategoryInvestment)
	_ = tax.SaveType(ctx, ta)
	_ = tax.SaveType(ctx, tb)
	sa, _ := model.NewExpenseSubtype(2027, "a1", "[a1]", "A")
	sb, _ := model.NewExpenseSubtype(2027, "b1", "[b1]", "B")
	_ = tax.SaveSubtype(ctx, sa)
	_ = tax.SaveSubtype(ctx, sb)
	p, _ := model.NewPartner(1, "Soci 1", "", "", "s1@e.test", "", model.Productor, 1, time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), false)
	_ = pr.Save(ctx, p)

	planned := time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	common, _ := model.NewUnsavedExpenseForecast(1, "Comú", "", model.MoneyOf(100), model.ZeroMoney(), nil, planned, 2027, "a1", model.NewCommonScope(), planned, true)
	soci, _ := model.NewUnsavedExpenseForecast(1, "Soci", "", model.MoneyOf(500), model.ZeroMoney(), nil, planned, 2027, "b1", model.NewPartnerScope(), planned, true)
	if _, err := fr.Create(ctx, common); err != nil {
		t.Fatal(err)
	}
	if _, err := fr.Create(ctx, soci); err != nil {
		t.Fatal(err)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestClose_PersistsApprovedAndReport(t *testing.T) {
	svc, conn := newSvc(t)
	seedOpenYearWithForecasts(t, conn)
	ctx := context.Background()

	rep, err := svc.Close(ctx, 2027)
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if rep.Year() != 2027 || rep.SnapshotJSON() == "" {
		t.Errorf("report wrong: %+v", rep)
	}

	// window CLOSED
	w, _, _ := persistence.NewWindowRepository(sqlc.New(conn)).FindByYear(ctx, 2027)
	if w.State() != model.WindowClosed || w.ClosedAt() == nil {
		t.Errorf("window not closed: %+v", w)
	}
	// forecasts got approvedAmount + approvedOn
	fs, _ := persistence.NewForecastRepository(conn, sqlc.New(conn)).ListByYear(ctx, 2027)
	for _, f := range fs {
		if f.ApprovedOn() == nil {
			t.Errorf("forecast %s missing approvedOn", f.ID())
		}
	}
	// snapshot deserializes; common total 100
	rd, err := application.SnapshotFromJSON(rep.SnapshotJSON())
	if err != nil {
		t.Fatal(err)
	}
	if rd.Categories[0].Common.Total.String() != "100.00" {
		t.Errorf("snapshot common total = %s, want 100.00", rd.Categories[0].Common.Total.String())
	}
	// audit WINDOW_CLOSED
	audits, _ := persistence.NewAuditLog(sqlc.New(conn)).List(ctx)
	var sawClose bool
	for _, a := range audits {
		if a.Kind() == model.AuditWindowClosed {
			sawClose = true
		}
	}
	if !sawClose {
		t.Errorf("no WINDOW_CLOSED audit")
	}
}

func TestClose_RejectsNonOpen(t *testing.T) {
	svc, conn := newSvc(t)
	seedClosedYear(t, conn, 2026)
	ctx := context.Background()
	if _, err := svc.Close(ctx, 2026); !errors.Is(err, application.ErrWrongState) {
		t.Errorf("want ErrWrongState closing a CLOSED window, got %v", err)
	}
}
```
Add `"database/sql"` to this file's imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/application/ -run TestClose -v`
Expected: FAIL — undefined `Close`.

- [ ] **Step 3: Implement Close + helpers**

Add to `internal/application/window_service.go` imports: `"fmt"`, `"sort"`, `"github.com/pjover/espigol/internal/domain/model/report"`, and `"github.com/pjover/espigol/internal/domain/services"`. Append:
```go
// Close runs the allocation for an OPEN year, persists approved amounts, stores
// a Report snapshot, flips the window to CLOSED, and audits — atomically.
func (s *WindowService) Close(ctx context.Context, year int) (model.Report, error) {
	now := s.clock.Now()
	var saved model.Report
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		w, ok, err := r.Windows.FindByYear(ctx, year)
		if err != nil {
			return err
		}
		if !ok {
			return ErrWindowNotFound
		}
		if w.State() != model.WindowOpen {
			return ErrWrongState
		}

		rd, err := s.computeReport(ctx, r, w)
		if err != nil {
			return err
		}
		writes, err := persistApproved(ctx, r, year, rd, now)
		if err != nil {
			return err
		}
		rep, err := s.buildReport(ctx, r, year, rd, now)
		if err != nil {
			return err
		}

		if err := r.Windows.Save(ctx, w.WithState(model.WindowClosed).WithClosedAt(now)); err != nil {
			return err
		}
		payload := fmt.Sprintf(`{"reportId":%d,"forecastsApproved":%d}`, rep.ID(), writes)
		if err := appendAudit(ctx, r, model.AuditWindowClosed, year, now, payload); err != nil {
			return err
		}
		saved = rep
		return nil
	})
	return saved, err
}

// computeReport gathers inputs from the tx repos and runs the allocation.
func (s *WindowService) computeReport(ctx context.Context, r ports.RepoSet, w model.SubmissionWindow) (report.ReportData, error) {
	year := w.Year()
	all, err := r.Forecasts.ListByYear(ctx, year)
	if err != nil {
		return report.ReportData{}, err
	}
	enabled := make([]model.ExpenseForecast, 0, len(all))
	for _, f := range all {
		if f.Enabled() {
			enabled = append(enabled, f)
		}
	}
	partners, err := r.Partners.List(ctx)
	if err != nil {
		return report.ReportData{}, err
	}
	sections, err := r.Sections.List(ctx)
	if err != nil {
		return report.ReportData{}, err
	}
	memberships, err := r.Sections.ListMemberships(ctx)
	if err != nil {
		return report.ReportData{}, err
	}
	subCat, err := buildSubtypeCategory(ctx, r, year)
	if err != nil {
		return report.ReportData{}, err
	}
	return services.Compute(services.AllocationInput{
		Year:            year,
		Forecasts:       enabled,
		Partners:        partners,
		Sections:        sections,
		Memberships:     memberships,
		SubtypeCategory: subCat,
		CurrentLimit:    w.CurrentExpenseLimit(),
		InvestmentLimit: w.InvestmentExpenseLimit(),
	})
}

// buildReport serializes the snapshot, renders the pdf, and inserts the Report row.
func (s *WindowService) buildReport(ctx context.Context, r ports.RepoSet, year int, rd report.ReportData, now time.Time) (model.Report, error) {
	snapshot, err := SnapshotToJSON(rd)
	if err != nil {
		return model.Report{}, err
	}
	pdf, err := s.renderer.Render(rd, now)
	if err != nil {
		return model.Report{}, err
	}
	rep, err := model.NewReport(0, year, now, snapshot, pdf, nil)
	if err != nil {
		return model.Report{}, err
	}
	id, err := r.Reports.Insert(ctx, rep)
	if err != nil {
		return model.Report{}, err
	}
	return model.NewReport(id, year, now, snapshot, pdf, nil)
}

// buildSubtypeCategory maps each subtype code to its type's category for the year.
func buildSubtypeCategory(ctx context.Context, r ports.RepoSet, year int) (map[string]model.ExpenseCategory, error) {
	types, err := r.Taxonomy.ListTypes(ctx, year)
	if err != nil {
		return nil, err
	}
	catByType := make(map[string]model.ExpenseCategory, len(types))
	for _, t := range types {
		catByType[t.Code()] = t.Category()
	}
	subs, err := r.Taxonomy.ListSubtypes(ctx, year)
	if err != nil {
		return nil, err
	}
	out := make(map[string]model.ExpenseCategory, len(subs))
	for _, st := range subs {
		if cat, ok := catByType[st.TypeCode()]; ok {
			out[st.Code()] = cat
		}
	}
	return out, nil
}

// collectApproved gathers approved amounts from every detail item, keyed by forecast id.
func collectApproved(rd report.ReportData) map[string]model.Money {
	out := map[string]model.Money{}
	for _, cat := range rd.Categories {
		for _, item := range cat.Common.Items {
			out[item.CpCode] = item.ApprovedAmount
		}
		for _, sd := range cat.Sections.SectionDetails {
			for _, item := range sd.Items {
				out[item.CpCode] = item.ApprovedAmount
			}
		}
		for _, pd := range cat.Sections.Partners.PartnerDetails {
			for _, item := range pd.Items {
				out[item.CpCode] = item.ApprovedAmount
			}
		}
	}
	return out
}

// persistApproved writes approved amounts onto enabled forecasts (skipping
// unchanged ones). Returns the number of rows written.
func persistApproved(ctx context.Context, r ports.RepoSet, year int, rd report.ReportData, now time.Time) (int, error) {
	approved := collectApproved(rd)
	all, err := r.Forecasts.ListByYear(ctx, year)
	if err != nil {
		return 0, err
	}
	byID := make(map[string]model.ExpenseForecast, len(all))
	for _, f := range all {
		if f.Enabled() {
			byID[f.ID()] = f
		}
	}
	ids := make([]string, 0, len(approved))
	for id := range approved {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	writes := 0
	for _, id := range ids {
		f, ok := byID[id]
		if !ok {
			continue
		}
		amt := approved[id]
		if f.ApprovedAmount().Cmp(amt) == 0 && f.ApprovedOn() != nil {
			continue
		}
		if err := r.Forecasts.Save(ctx, f.WithApprovedAmount(amt).WithApprovedOn(now)); err != nil {
			return 0, err
		}
		writes++
	}
	return writes, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/application/ -v && go build ./...`
Expected: PASS (Close persists approved, stores report, closes window, audits; rejects non-OPEN).

- [ ] **Step 5: Commit**

```bash
git add internal/application/window_service.go internal/application/close_test.go
git commit -m "feat(application): WindowService Close (allocate, persist, snapshot, audit)"
```

---

### Task 8: WindowService — Amend

**Files:**
- Modify: `internal/application/window_service.go`
- Test: `internal/application/amend_test.go`

**Interfaces:**
- Produces: `(*WindowService).Amend(ctx, year int) (model.Report, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/application/amend_test.go`:
```go
package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

func TestAmend_SupersedesAndKeepsClosed(t *testing.T) {
	svc, conn := newSvc(t)
	seedOpenYearWithForecasts(t, conn)
	ctx := context.Background()

	first, err := svc.Close(ctx, 2027)
	if err != nil {
		t.Fatal(err)
	}

	second, err := svc.Amend(ctx, 2027)
	if err != nil {
		t.Fatalf("amend: %v", err)
	}
	if second.ID() == first.ID() {
		t.Errorf("amend should insert a new report")
	}

	// window stays CLOSED
	w, _, _ := persistence.NewWindowRepository(sqlc.New(conn)).FindByYear(ctx, 2027)
	if w.State() != model.WindowClosed {
		t.Errorf("window should stay CLOSED, got %s", w.State())
	}
	// latest non-superseded report is the amended one
	latest, ok, _ := persistence.NewReportRepository(sqlc.New(conn)).FindLatestByYear(ctx, 2027)
	if !ok || latest.ID() != second.ID() {
		t.Errorf("latest report = %d, want amended %d", latest.ID(), second.ID())
	}
	// a REPORT_GENERATED audit exists
	audits, _ := persistence.NewAuditLog(sqlc.New(conn)).List(ctx)
	var sawGen bool
	for _, a := range audits {
		if a.Kind() == model.AuditReportGenerated {
			sawGen = true
		}
	}
	if !sawGen {
		t.Errorf("no REPORT_GENERATED audit")
	}
}

func TestAmend_RejectsNonClosed(t *testing.T) {
	svc, conn := newSvc(t)
	seedClosedYear(t, conn, 2026)
	ctx := context.Background()
	_, _ = svc.CreateYear(ctx, 2027)
	if _, err := svc.Amend(ctx, 2027); !errors.Is(err, application.ErrWrongState) {
		t.Errorf("want ErrWrongState amending a DRAFT window, got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/application/ -run TestAmend -v`
Expected: FAIL — undefined `Amend`.

- [ ] **Step 3: Implement Amend**

Append to `internal/application/window_service.go`:
```go
// Amend re-runs the allocation for a CLOSED year, supersedes the current report,
// inserts a new one, and updates approved amounts — without changing window state.
func (s *WindowService) Amend(ctx context.Context, year int) (model.Report, error) {
	now := s.clock.Now()
	var saved model.Report
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		w, ok, err := r.Windows.FindByYear(ctx, year)
		if err != nil {
			return err
		}
		if !ok {
			return ErrWindowNotFound
		}
		if w.State() != model.WindowClosed {
			return ErrWrongState
		}

		rd, err := s.computeReport(ctx, r, w)
		if err != nil {
			return err
		}
		if _, err := persistApproved(ctx, r, year, rd, now); err != nil {
			return err
		}

		if latest, ok, err := r.Reports.FindLatestByYear(ctx, year); err != nil {
			return err
		} else if ok {
			if err := r.Reports.MarkSuperseded(ctx, latest.ID(), now); err != nil {
				return err
			}
		}

		rep, err := s.buildReport(ctx, r, year, rd, now)
		if err != nil {
			return err
		}
		if err := appendAudit(ctx, r, model.AuditReportGenerated, year, now, ""); err != nil {
			return err
		}
		saved = rep
		return nil
	})
	return saved, err
}
```

- [ ] **Step 4: Run tests + full suite**

Run: `go test ./internal/application/ -v && go vet ./... && go test ./...`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/application/window_service.go internal/application/amend_test.go
git commit -m "feat(application): WindowService Amend (regenerate, supersede)"
```

---

## Self-Review

**Spec coverage (against the Phase 4 design):**
- §2.1 ReportRenderer port + no-op → Task 2. TxManager/RepoSet → Task 4.
- §2.2 Money JSON → Task 1.
- §2.3 WindowService struct/ctor → Task 6.
- §3.1 CreateYear (copy prior taxonomy+limits, DRAFT, default deadline, ErrYearExists/ErrNoPriorYear) → Task 6.
- §3.2 Open (DRAFT-only, future deadline, both categories, no other OPEN, audit) → Task 6.
- §3.3 Close (gather, Compute, collect approved from all items, skip-unchanged, snapshot, render, insert Report, CLOSED, audit payload) → Task 7.
- §3.4 Amend (CLOSED-only, supersede latest, new report, no state change, REPORT_GENERATED) → Task 8.
- §3.5 atomicity (single WithinTx per op) → Tasks 6–8.
- §4 ListMemberships, TxManager impl, no-op renderer, no migration → Tasks 3, 4, 2.
- §5 tests (Money JSON, TxManager commit/rollback, WindowService ops, snapshot round-trip) → Tasks 1, 4, 5, 6, 7, 8.

**Placeholder scan:** No "TBD"/"implement later". The `var _ = fmt.Sprintf` line in Task 6 is explicitly flagged for removal in Task 7 (it prevents an unused-import error in the CreateYear/Open-only intermediate state) — alternatively drop the `fmt` import in Task 6 and add it in Task 7; the step says so. The sqlc-row-type note in Task 3 step 5 instructs the implementer to reconcile the generated identifier.

**Type consistency:** `ports.RepoSet` field types match the Phase-2 port interfaces; `NewWindowService(tx, renderer, clock)` matches its call site in the test helper `newSvc`; `services.Compute`/`AllocationInput` field names match Phase 3; `report.ReportData` traversal (`Categories[].Common.Items`, `.Sections.SectionDetails[].Items`, `.Sections.Partners.PartnerDetails[].Items`, `DetailItem.CpCode/ApprovedAmount`) matches the Phase-3 report structs; `model.NewReport`/`NewAuditEvent`/`NewSubmissionWindow`/`NewUnsavedExpenseForecast` signatures match Phase 2. `model.AuditWindowOpened/AuditWindowClosed/AuditReportGenerated` are the Phase-2 enum constants.

**Determinism:** `persistApproved` sorts forecast ids before writing; `mostRecentPrior` is a max-scan (order-independent).
