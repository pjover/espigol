# Subsidy Reconciliation — Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the reconciliation data layer — `Concession`, `ConcessionForecast`, `Invoice`, `InvoicePayment`, `ForecastInvoice` — with persistence, JSON import, and a TUI "Ajuts" editing panel, so 2025's granted subsidy and invoices can be stored, validated, and round-tripped. No reconciliation math and no report output (those are Phases 2–3).

**Architecture:** Follows the existing hexagonal layout. New goose migration + sqlc queries → generated row types; hand-written mappers + two repositories (`ConcessionRepository`, `InvoiceRepository`) added to `ports.RepoSet`. A new `application.ReconciliationService` does replace-all import and single-record CRUD inside one `TxManager.WithinTx`, enforcing referential integrity (hard) and integrity checks (soft warnings). A new `importer.LoadReconciliation` parses `import/reconciliation-<year>.json`. A new Bubble Tea `ajutsPanel` lists/edits concessions and invoices and triggers the import.

**Tech Stack:** Go, modernc.org/sqlite (pure-Go, WAL, `foreign_keys(1)`), goose migrations, sqlc, Bubble Tea + lipgloss, shopspring/decimal via `model.Money`.

## Global Constraints

- **Program in English; Catalan only for outputs/UI.** Entities/fields/DB identifiers are English (`Concession`, `GroupCode`, `Invoice`); only TUI strings and labels are Catalan ("Concessió", "Factura", "Grup", "Demanat", "Concedit").
- **All monetary values are `model.Money`** (scale-2, HALF_UP), stored as TEXT via `.String()` / `model.MoneyFromString`. Never `float64`.
- **No window-state gate:** reconciliation data is a year-keyed overlay, editable in any window state (unlike the OPEN-gated forecast importer).
- **Import is replace-all and atomic:** one `TxManager.WithinTx`; any hard-validation failure rolls back and leaves the year's prior reconciliation data untouched.
- **Dates:** invoice `issueDate` and payment `paidOn` are calendar dates (`FormatDate`/`ParseDate`, layout `2006-01-02`). JSON dates are `YYYY-MM-DD`.
- **Soft checks never abort** — they are collected as Catalan warning strings and returned/surfaced, mirroring the workbook's `OK?` columns.
- **`invoice` has a surrogate autoincrement `id`;** payments and forecast-links reference it and cascade on delete (`ON DELETE CASCADE`, enforced since `db.Open` sets `foreign_keys(1)`).
- **One concession per forecast** is enforced by `concession_forecast` PK `(year, forecast_id)`.
- Follow existing patterns: immutable domain structs with constructors + getters (no setters beyond `With…` copies); repositories take `*sqlc.Queries`; services return sentinel errors from `internal/application/errors.go`; commit after each task; run `make vet` and `go test ./...` before committing.
- **TUI multi-child editing uses delimited text fields** (the generic `formModal` has only single-line text inputs): forecast-ID lists as `CP25008,CP25009`; payments as `2025-04-01:1234.56;2025-05-01:200.00`; invoice→forecast links as `CP25030:500.00;CP25032:734.56`. Bulk data still comes through JSON import.

---

### Task 1: Schema migration + sqlc queries + generation

**Files:**
- Create: `db/migrations/00003_reconciliation.sql`
- Create: `db/queries/reconciliation.sql`
- Generate: `internal/adapters/persistence/sqlc/*` (via `make sqlc-generate`)
- Test: `internal/adapters/persistence/reconciliation_schema_test.go`

**Interfaces:**
- Produces (generated sqlc): row types `sqlc.Concession`, `sqlc.ConcessionForecast`, `sqlc.Invoice`, `sqlc.InvoicePayment`, `sqlc.ForecastInvoice`; query methods `ListConcessionsByYear`, `UpsertConcession`, `DeleteConcession`, `DeleteConcessionsByYear`, `ListConcessionForecastsByYear`, `InsertConcessionForecast`, `DeleteConcessionForecastsByGroup`, `DeleteConcessionForecastsByYear`, `ListInvoicesByYear`, `InsertInvoice` (`:one`, returns `int64` id), `UpdateInvoice`, `DeleteInvoice`, `DeleteInvoicesByYear`, `ListInvoicePaymentsByYear`, `InsertInvoicePayment`, `DeletePaymentsByInvoice`, `ListForecastInvoicesByYear`, `InsertForecastInvoice`, `DeleteForecastInvoicesByInvoice`.

- [ ] **Step 1: Write the migration**

Create `db/migrations/00003_reconciliation.sql`:

```sql
-- +goose Up
CREATE TABLE concession (
    year            INTEGER NOT NULL,
    group_code      TEXT NOT NULL,
    subtype_code    TEXT NOT NULL,
    concept         TEXT NOT NULL,
    requested_total TEXT NOT NULL,
    granted_amount  TEXT NOT NULL,
    PRIMARY KEY (year, group_code),
    FOREIGN KEY (year, subtype_code) REFERENCES expense_subtype(year, code)
);

CREATE TABLE concession_forecast (
    year        INTEGER NOT NULL,
    forecast_id TEXT NOT NULL,
    group_code  TEXT NOT NULL,
    PRIMARY KEY (year, forecast_id),
    FOREIGN KEY (year, group_code) REFERENCES concession(year, group_code),
    FOREIGN KEY (forecast_id) REFERENCES expense_forecast(id)
);
CREATE INDEX idx_concession_forecast_group ON concession_forecast(year, group_code);

CREATE TABLE invoice (
    id         INTEGER PRIMARY KEY,
    year       INTEGER NOT NULL,
    issuer     TEXT NOT NULL,
    nif        TEXT NOT NULL,
    number     TEXT NOT NULL,
    issue_date TEXT NOT NULL,
    net_amount TEXT NOT NULL,
    file_path  TEXT,
    notes      TEXT,
    UNIQUE (year, nif, number),
    FOREIGN KEY (year) REFERENCES submission_window(year)
);

CREATE TABLE invoice_payment (
    id         INTEGER PRIMARY KEY,
    invoice_id INTEGER NOT NULL,
    paid_on    TEXT NOT NULL,
    amount     TEXT NOT NULL,
    FOREIGN KEY (invoice_id) REFERENCES invoice(id) ON DELETE CASCADE
);
CREATE INDEX idx_invoice_payment_invoice ON invoice_payment(invoice_id);

CREATE TABLE forecast_invoice (
    forecast_id TEXT NOT NULL,
    invoice_id  INTEGER NOT NULL,
    amount      TEXT NOT NULL,
    PRIMARY KEY (forecast_id, invoice_id),
    FOREIGN KEY (forecast_id) REFERENCES expense_forecast(id),
    FOREIGN KEY (invoice_id) REFERENCES invoice(id) ON DELETE CASCADE
);
CREATE INDEX idx_forecast_invoice_invoice ON forecast_invoice(invoice_id);

-- +goose Down
DROP TABLE forecast_invoice;
DROP TABLE invoice_payment;
DROP TABLE invoice;
DROP TABLE concession_forecast;
DROP TABLE concession;
```

- [ ] **Step 2: Write the queries**

Create `db/queries/reconciliation.sql`:

```sql
-- name: ListConcessionsByYear :many
SELECT year, group_code, subtype_code, concept, requested_total, granted_amount
FROM concession WHERE year = ? ORDER BY group_code;

-- name: UpsertConcession :exec
INSERT INTO concession (year, group_code, subtype_code, concept, requested_total, granted_amount)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(year, group_code) DO UPDATE SET
    subtype_code=excluded.subtype_code, concept=excluded.concept,
    requested_total=excluded.requested_total, granted_amount=excluded.granted_amount;

-- name: DeleteConcession :exec
DELETE FROM concession WHERE year = ? AND group_code = ?;

-- name: DeleteConcessionsByYear :exec
DELETE FROM concession WHERE year = ?;

-- name: ListConcessionForecastsByYear :many
SELECT year, forecast_id, group_code FROM concession_forecast
WHERE year = ? ORDER BY group_code, forecast_id;

-- name: InsertConcessionForecast :exec
INSERT INTO concession_forecast (year, forecast_id, group_code) VALUES (?, ?, ?);

-- name: DeleteConcessionForecastsByGroup :exec
DELETE FROM concession_forecast WHERE year = ? AND group_code = ?;

-- name: DeleteConcessionForecastsByYear :exec
DELETE FROM concession_forecast WHERE year = ?;

-- name: ListInvoicesByYear :many
SELECT id, year, issuer, nif, number, issue_date, net_amount, file_path, notes
FROM invoice WHERE year = ? ORDER BY id;

-- name: InsertInvoice :one
INSERT INTO invoice (year, issuer, nif, number, issue_date, net_amount, file_path, notes)
VALUES (?, ?, ?, ?, ?, ?, ?, ?) RETURNING id;

-- name: UpdateInvoice :exec
UPDATE invoice SET year=?, issuer=?, nif=?, number=?, issue_date=?, net_amount=?, file_path=?, notes=?
WHERE id=?;

-- name: DeleteInvoice :exec
DELETE FROM invoice WHERE id = ?;

-- name: DeleteInvoicesByYear :exec
DELETE FROM invoice WHERE year = ?;

-- name: ListInvoicePaymentsByYear :many
SELECT p.id, p.invoice_id, p.paid_on, p.amount FROM invoice_payment p
JOIN invoice i ON i.id = p.invoice_id WHERE i.year = ? ORDER BY p.invoice_id, p.id;

-- name: InsertInvoicePayment :exec
INSERT INTO invoice_payment (invoice_id, paid_on, amount) VALUES (?, ?, ?);

-- name: DeletePaymentsByInvoice :exec
DELETE FROM invoice_payment WHERE invoice_id = ?;

-- name: ListForecastInvoicesByYear :many
SELECT fi.forecast_id, fi.invoice_id, fi.amount FROM forecast_invoice fi
JOIN invoice i ON i.id = fi.invoice_id WHERE i.year = ? ORDER BY fi.invoice_id, fi.forecast_id;

-- name: InsertForecastInvoice :exec
INSERT INTO forecast_invoice (forecast_id, invoice_id, amount) VALUES (?, ?, ?);

-- name: DeleteForecastInvoicesByInvoice :exec
DELETE FROM forecast_invoice WHERE invoice_id = ?;
```

- [ ] **Step 3: Regenerate sqlc**

Run: `make sqlc-generate`
Expected: no errors; `git status` shows new/updated files under `internal/adapters/persistence/sqlc/`.

- [ ] **Step 4: Write the schema smoke test**

Create `internal/adapters/persistence/reconciliation_schema_test.go`:

```go
package persistence_test

import (
	"context"
	"testing"

	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
)

func TestReconciliationSchema_TablesExistAndQueryEmpty(t *testing.T) {
	q := openTestDB(t)
	ctx := context.Background()

	cs, err := q.ListConcessionsByYear(ctx, 2025)
	if err != nil {
		t.Fatalf("ListConcessionsByYear: %v", err)
	}
	if len(cs) != 0 {
		t.Fatalf("want 0 concessions, got %d", len(cs))
	}
	inv, err := q.ListInvoicesByYear(ctx, 2025)
	if err != nil {
		t.Fatalf("ListInvoicesByYear: %v", err)
	}
	if len(inv) != 0 {
		t.Fatalf("want 0 invoices, got %d", len(inv))
	}
	_ = sqlc.Concession{}
}
```

- [ ] **Step 5: Run the test**

Run: `go test ./internal/adapters/persistence/ -run TestReconciliationSchema -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add db/migrations/00003_reconciliation.sql db/queries/reconciliation.sql internal/adapters/persistence/sqlc internal/adapters/persistence/reconciliation_schema_test.go
git commit -m "feat(persistence): reconciliation schema + sqlc queries"
```

---

### Task 2: Domain model — `Concession` + `ConcessionForecast`

**Files:**
- Create: `internal/domain/model/concession.go`
- Test: `internal/domain/model/concession_test.go`

**Interfaces:**
- Produces:
  - `model.NewConcession(year int, groupCode, subtypeCode, concept string, requested, granted model.Money) (model.Concession, error)` with getters `Year() int`, `GroupCode() string`, `SubtypeCode() string`, `Concept() string`, `RequestedTotal() model.Money`, `GrantedAmount() model.Money`.
  - `model.NewConcessionForecast(year int, groupCode, forecastID string) (model.ConcessionForecast, error)` with getters `Year() int`, `GroupCode() string`, `ForecastID() string`.

- [ ] **Step 1: Write the failing tests**

Create `internal/domain/model/concession_test.go`:

```go
package model_test

import (
	"testing"

	"github.com/pjover/espigol/internal/domain/model"
)

func TestNewConcession_Valid(t *testing.T) {
	c, err := model.NewConcession(2025, "A6-02", "a6", "Adob orgànic",
		model.MoneyOf(13880), model.MoneyOf(13880))
	if err != nil {
		t.Fatalf("NewConcession: %v", err)
	}
	if c.GroupCode() != "A6-02" || c.SubtypeCode() != "a6" || c.Concept() != "Adob orgànic" {
		t.Errorf("unexpected fields: %+v", c)
	}
	if c.GrantedAmount().Cmp(model.MoneyOf(13880)) != 0 {
		t.Errorf("granted = %s", c.GrantedAmount())
	}
}

func TestNewConcession_Rejects(t *testing.T) {
	cases := map[string]struct{ group, subtype string }{
		"empty group":   {"", "a6"},
		"empty subtype": {"A6-02", ""},
	}
	for name, tc := range cases {
		if _, err := model.NewConcession(2025, tc.group, tc.subtype, "c",
			model.ZeroMoney(), model.ZeroMoney()); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestNewConcessionForecast_Valid(t *testing.T) {
	cf, err := model.NewConcessionForecast(2025, "A6-02", "CP25008")
	if err != nil {
		t.Fatalf("NewConcessionForecast: %v", err)
	}
	if cf.ForecastID() != "CP25008" || cf.GroupCode() != "A6-02" || cf.Year() != 2025 {
		t.Errorf("unexpected: %+v", cf)
	}
}

func TestNewConcessionForecast_Rejects(t *testing.T) {
	if _, err := model.NewConcessionForecast(2025, "", "CP25008"); err == nil {
		t.Error("empty group: expected error")
	}
	if _, err := model.NewConcessionForecast(2025, "A6-02", ""); err == nil {
		t.Error("empty forecastID: expected error")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/domain/model/ -run Concession -v`
Expected: FAIL — undefined `model.NewConcession`.

- [ ] **Step 3: Implement the model**

Create `internal/domain/model/concession.go`:

```go
package model

import "fmt"

// Concession is a granted subsidy for a (year, groupCode) bundle — the funder's
// "Concedit" per "Grup". It bundles one or more ExpenseForecasts that share a
// (subtypeCode, Concept); membership lives in ConcessionForecast.
type Concession struct {
	year           int
	groupCode      string
	subtypeCode    string
	concept        string
	requestedTotal Money
	grantedAmount  Money
}

func NewConcession(year int, groupCode, subtypeCode, concept string, requested, granted Money) (Concession, error) {
	if groupCode == "" {
		return Concession{}, fmt.Errorf("concession groupCode must not be empty")
	}
	if subtypeCode == "" {
		return Concession{}, fmt.Errorf("concession subtypeCode must not be empty")
	}
	return Concession{year, groupCode, subtypeCode, concept, requested, granted}, nil
}

func (c Concession) Year() int              { return c.year }
func (c Concession) GroupCode() string      { return c.groupCode }
func (c Concession) SubtypeCode() string    { return c.subtypeCode }
func (c Concession) Concept() string        { return c.concept }
func (c Concession) RequestedTotal() Money  { return c.requestedTotal }
func (c Concession) GrantedAmount() Money   { return c.grantedAmount }

// ConcessionForecast links one ExpenseForecast to its Concession group. The
// (year, forecastID) pair is unique — a forecast belongs to at most one group.
type ConcessionForecast struct {
	year       int
	groupCode  string
	forecastID string
}

func NewConcessionForecast(year int, groupCode, forecastID string) (ConcessionForecast, error) {
	if groupCode == "" {
		return ConcessionForecast{}, fmt.Errorf("concessionForecast groupCode must not be empty")
	}
	if forecastID == "" {
		return ConcessionForecast{}, fmt.Errorf("concessionForecast forecastID must not be empty")
	}
	return ConcessionForecast{year, groupCode, forecastID}, nil
}

func (c ConcessionForecast) Year() int          { return c.year }
func (c ConcessionForecast) GroupCode() string  { return c.groupCode }
func (c ConcessionForecast) ForecastID() string { return c.forecastID }
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/domain/model/ -run Concession -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/model/concession.go internal/domain/model/concession_test.go
git commit -m "feat(model): Concession + ConcessionForecast"
```

---

### Task 3: Domain model — `Invoice` aggregate (`InvoicePayment`, `ForecastInvoice`)

**Files:**
- Create: `internal/domain/model/invoice.go`
- Test: `internal/domain/model/invoice_test.go`

**Interfaces:**
- Produces:
  - `model.NewInvoicePayment(id, invoiceID int, paidOn time.Time, amount model.Money) model.InvoicePayment` with getters `ID()`, `InvoiceID()`, `PaidOn() time.Time`, `Amount() model.Money`.
  - `model.NewForecastInvoice(forecastID string, invoiceID int, amount model.Money) (model.ForecastInvoice, error)` with getters `ForecastID()`, `InvoiceID()`, `Amount()`.
  - `model.NewInvoice(id, year int, issuer, nif, number string, issueDate time.Time, net model.Money, filePath, notes *string, payments []model.InvoicePayment, links []model.ForecastInvoice) (model.Invoice, error)` with getters `ID() int`, `Year() int`, `Issuer()`, `Nif()`, `Number() string`, `IssueDate() time.Time`, `NetAmount() model.Money`, `FilePath() *string`, `Notes() *string`, `Payments() []model.InvoicePayment`, `Links() []model.ForecastInvoice`, plus `PaidTotal() model.Money` (Σ payment amounts) and `WithID(id int) model.Invoice`.

- [ ] **Step 1: Write the failing tests**

Create `internal/domain/model/invoice_test.go`:

```go
package model_test

import (
	"testing"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
)

func TestNewInvoice_AggregatesAndPaidTotal(t *testing.T) {
	d := time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC)
	pays := []model.InvoicePayment{
		model.NewInvoicePayment(0, 0, d, model.MoneyOf(1000)),
		model.NewInvoicePayment(0, 0, d.AddDate(0, 1, 0), model.MoneyOf(234)),
	}
	link, err := model.NewForecastInvoice("CP25030", 0, model.MoneyOf(500))
	if err != nil {
		t.Fatalf("NewForecastInvoice: %v", err)
	}
	inv, err := model.NewInvoice(0, 2025, "Jardines Campaner", "B12345678", "F878",
		d, model.MoneyOf(1234), nil, nil, pays, []model.ForecastInvoice{link})
	if err != nil {
		t.Fatalf("NewInvoice: %v", err)
	}
	if inv.Number() != "F878" || inv.Year() != 2025 {
		t.Errorf("unexpected header: %+v", inv)
	}
	if len(inv.Payments()) != 2 || len(inv.Links()) != 1 {
		t.Errorf("children: payments=%d links=%d", len(inv.Payments()), len(inv.Links()))
	}
	if inv.PaidTotal().Cmp(model.MoneyOf(1234)) != 0 {
		t.Errorf("PaidTotal = %s, want 1234.00", inv.PaidTotal())
	}
}

func TestNewInvoice_Rejects(t *testing.T) {
	d := time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC)
	if _, err := model.NewInvoice(0, 2025, "", "n", "num", d, model.ZeroMoney(), nil, nil, nil, nil); err == nil {
		t.Error("empty issuer: expected error")
	}
	if _, err := model.NewInvoice(0, 2025, "iss", "n", "", d, model.ZeroMoney(), nil, nil, nil, nil); err == nil {
		t.Error("empty number: expected error")
	}
}

func TestForecastInvoice_Rejects(t *testing.T) {
	if _, err := model.NewForecastInvoice("", 1, model.ZeroMoney()); err == nil {
		t.Error("empty forecastID: expected error")
	}
}

func TestInvoice_WithID(t *testing.T) {
	d := time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC)
	inv, _ := model.NewInvoice(0, 2025, "iss", "n", "num", d, model.MoneyOf(10), nil, nil, nil, nil)
	if inv.WithID(7).ID() != 7 {
		t.Error("WithID did not set id")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/domain/model/ -run 'Invoice|ForecastInvoice' -v`
Expected: FAIL — undefined `model.NewInvoice`.

- [ ] **Step 3: Implement the model**

Create `internal/domain/model/invoice.go`:

```go
package model

import (
	"fmt"
	"time"
)

// InvoicePayment is one transfer against an invoice. An invoice may be paid in
// several payments; zero payments means unpaid.
type InvoicePayment struct {
	id        int
	invoiceID int
	paidOn    time.Time
	amount    Money
}

func NewInvoicePayment(id, invoiceID int, paidOn time.Time, amount Money) InvoicePayment {
	return InvoicePayment{id, invoiceID, paidOn, amount}
}

func (p InvoicePayment) ID() int          { return p.id }
func (p InvoicePayment) InvoiceID() int    { return p.invoiceID }
func (p InvoicePayment) PaidOn() time.Time { return p.paidOn }
func (p InvoicePayment) Amount() Money     { return p.amount }

// ForecastInvoice links an Invoice to an ExpenseForecast with the euro share of
// that invoice charged to the forecast — the M–N reconciliation truth.
type ForecastInvoice struct {
	forecastID string
	invoiceID  int
	amount     Money
}

func NewForecastInvoice(forecastID string, invoiceID int, amount Money) (ForecastInvoice, error) {
	if forecastID == "" {
		return ForecastInvoice{}, fmt.Errorf("forecastInvoice forecastID must not be empty")
	}
	return ForecastInvoice{forecastID, invoiceID, amount}, nil
}

func (f ForecastInvoice) ForecastID() string { return f.forecastID }
func (f ForecastInvoice) InvoiceID() int      { return f.invoiceID }
func (f ForecastInvoice) Amount() Money        { return f.amount }

// Invoice is a supplier invoice (Factura) and the aggregate root over its
// payments and forecast-links. NetAmount is the ex-VAT executed spend.
type Invoice struct {
	id        int
	year      int
	issuer    string
	nif       string
	number    string
	issueDate time.Time
	netAmount Money
	filePath  *string
	notes     *string
	payments  []InvoicePayment
	links     []ForecastInvoice
}

func NewInvoice(id, year int, issuer, nif, number string, issueDate time.Time,
	net Money, filePath, notes *string, payments []InvoicePayment, links []ForecastInvoice) (Invoice, error) {
	if issuer == "" {
		return Invoice{}, fmt.Errorf("invoice issuer must not be empty")
	}
	if number == "" {
		return Invoice{}, fmt.Errorf("invoice number must not be empty")
	}
	return Invoice{id, year, issuer, nif, number, issueDate, net, filePath, notes, payments, links}, nil
}

func (i Invoice) ID() int                    { return i.id }
func (i Invoice) Year() int                  { return i.year }
func (i Invoice) Issuer() string             { return i.issuer }
func (i Invoice) Nif() string                { return i.nif }
func (i Invoice) Number() string             { return i.number }
func (i Invoice) IssueDate() time.Time       { return i.issueDate }
func (i Invoice) NetAmount() Money           { return i.netAmount }
func (i Invoice) FilePath() *string          { return i.filePath }
func (i Invoice) Notes() *string             { return i.notes }
func (i Invoice) Payments() []InvoicePayment { return i.payments }
func (i Invoice) Links() []ForecastInvoice   { return i.links }

// PaidTotal sums the invoice's payment amounts.
func (i Invoice) PaidTotal() Money {
	total := ZeroMoney()
	for _, p := range i.payments {
		total = total.Plus(p.amount)
	}
	return total
}

// WithID returns a copy with the id set (used after the repository allocates it).
func (i Invoice) WithID(id int) Invoice { i.id = id; return i }
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/domain/model/ -run 'Invoice|ForecastInvoice' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/model/invoice.go internal/domain/model/invoice_test.go
git commit -m "feat(model): Invoice aggregate with payments + forecast links"
```

---

### Task 4: `ConcessionRepository` — port, mapper, repo, wiring

**Files:**
- Modify: `internal/domain/ports/ports.go` (add interface), `internal/domain/ports/tx.go` (add to `RepoSet`)
- Create: `internal/adapters/persistence/mapper/concession.go`
- Create: `internal/adapters/persistence/concession_repository.go`
- Modify: `internal/adapters/persistence/txmanager.go`, `internal/adapters/persistence/ports_check.go`
- Test: `internal/adapters/persistence/concession_repository_test.go`

**Interfaces:**
- Consumes: `sqlc` query methods from Task 1; `model.Concession`/`model.ConcessionForecast` from Task 2.
- Produces: `ports.ConcessionRepository` with
  - `ListByYear(ctx, year int) ([]model.Concession, error)`
  - `ListForecastLinksByYear(ctx, year int) ([]model.ConcessionForecast, error)`
  - `Save(ctx, c model.Concession) error`
  - `Delete(ctx, year int, groupCode string) error` (cascades its forecast links)
  - `ReplaceMembership(ctx, year int, groupCode string, forecastIDs []string) error`
  - `ReplaceForYear(ctx, year int, concessions []model.Concession, links []model.ConcessionForecast) error`
  - Added to `ports.RepoSet` as field `Concessions ConcessionRepository`.

- [ ] **Step 1: Add the port interface**

In `internal/domain/ports/ports.go`, add after `TaxonomyRepository`:

```go
// ConcessionRepository manages Concession grants and their forecast membership.
type ConcessionRepository interface {
	ListByYear(ctx context.Context, year int) ([]model.Concession, error)
	ListForecastLinksByYear(ctx context.Context, year int) ([]model.ConcessionForecast, error)
	Save(ctx context.Context, c model.Concession) error
	Delete(ctx context.Context, year int, groupCode string) error
	ReplaceMembership(ctx context.Context, year int, groupCode string, forecastIDs []string) error
	ReplaceForYear(ctx context.Context, year int, concessions []model.Concession, links []model.ConcessionForecast) error
}
```

In `internal/domain/ports/tx.go`, add to `RepoSet`:

```go
	Concessions ConcessionRepository
```

- [ ] **Step 2: Write the mapper**

Create `internal/adapters/persistence/mapper/concession.go`:

```go
package mapper

import (
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func ConcessionToUpsert(c model.Concession) sqlc.UpsertConcessionParams {
	return sqlc.UpsertConcessionParams{
		Year:           int64(c.Year()),
		GroupCode:      c.GroupCode(),
		SubtypeCode:    c.SubtypeCode(),
		Concept:        c.Concept(),
		RequestedTotal: c.RequestedTotal().String(),
		GrantedAmount:  c.GrantedAmount().String(),
	}
}

func ConcessionFromRow(r sqlc.Concession) (model.Concession, error) {
	req, err := model.MoneyFromString(r.RequestedTotal)
	if err != nil {
		return model.Concession{}, err
	}
	granted, err := model.MoneyFromString(r.GrantedAmount)
	if err != nil {
		return model.Concession{}, err
	}
	return model.NewConcession(int(r.Year), r.GroupCode, r.SubtypeCode, r.Concept, req, granted)
}

func ConcessionForecastFromRow(r sqlc.ConcessionForecast) (model.ConcessionForecast, error) {
	return model.NewConcessionForecast(int(r.Year), r.GroupCode, r.ForecastID)
}
```

- [ ] **Step 3: Write the repository**

Create `internal/adapters/persistence/concession_repository.go`:

```go
package persistence

import (
	"context"

	"github.com/pjover/espigol/internal/adapters/persistence/mapper"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

type ConcessionRepository struct {
	q *sqlc.Queries
}

func NewConcessionRepository(q *sqlc.Queries) *ConcessionRepository {
	return &ConcessionRepository{q: q}
}

func (r *ConcessionRepository) ListByYear(ctx context.Context, year int) ([]model.Concession, error) {
	rows, err := r.q.ListConcessionsByYear(ctx, int64(year))
	if err != nil {
		return nil, err
	}
	out := make([]model.Concession, 0, len(rows))
	for _, row := range rows {
		c, err := mapper.ConcessionFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (r *ConcessionRepository) ListForecastLinksByYear(ctx context.Context, year int) ([]model.ConcessionForecast, error) {
	rows, err := r.q.ListConcessionForecastsByYear(ctx, int64(year))
	if err != nil {
		return nil, err
	}
	out := make([]model.ConcessionForecast, 0, len(rows))
	for _, row := range rows {
		cf, err := mapper.ConcessionForecastFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, cf)
	}
	return out, nil
}

func (r *ConcessionRepository) Save(ctx context.Context, c model.Concession) error {
	return r.q.UpsertConcession(ctx, mapper.ConcessionToUpsert(c))
}

func (r *ConcessionRepository) Delete(ctx context.Context, year int, groupCode string) error {
	if err := r.q.DeleteConcessionForecastsByGroup(ctx, sqlc.DeleteConcessionForecastsByGroupParams{
		Year: int64(year), GroupCode: groupCode,
	}); err != nil {
		return err
	}
	return r.q.DeleteConcession(ctx, sqlc.DeleteConcessionParams{Year: int64(year), GroupCode: groupCode})
}

func (r *ConcessionRepository) ReplaceMembership(ctx context.Context, year int, groupCode string, forecastIDs []string) error {
	if err := r.q.DeleteConcessionForecastsByGroup(ctx, sqlc.DeleteConcessionForecastsByGroupParams{
		Year: int64(year), GroupCode: groupCode,
	}); err != nil {
		return err
	}
	for _, fid := range forecastIDs {
		if err := r.q.InsertConcessionForecast(ctx, sqlc.InsertConcessionForecastParams{
			Year: int64(year), ForecastID: fid, GroupCode: groupCode,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *ConcessionRepository) ReplaceForYear(ctx context.Context, year int, concessions []model.Concession, links []model.ConcessionForecast) error {
	if err := r.q.DeleteConcessionForecastsByYear(ctx, int64(year)); err != nil {
		return err
	}
	if err := r.q.DeleteConcessionsByYear(ctx, int64(year)); err != nil {
		return err
	}
	for _, c := range concessions {
		if err := r.q.UpsertConcession(ctx, mapper.ConcessionToUpsert(c)); err != nil {
			return err
		}
	}
	for _, l := range links {
		if err := r.q.InsertConcessionForecast(ctx, sqlc.InsertConcessionForecastParams{
			Year: int64(l.Year()), ForecastID: l.ForecastID(), GroupCode: l.GroupCode(),
		}); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Wire into RepoSet + ports_check**

In `internal/adapters/persistence/txmanager.go`, add to the `repos := ports.RepoSet{…}` literal:

```go
		Concessions: NewConcessionRepository(q),
```

In `internal/adapters/persistence/ports_check.go`, add to the `var (…)` block:

```go
	_ ports.ConcessionRepository = (*ConcessionRepository)(nil)
```

- [ ] **Step 5: Write the round-trip test**

Create `internal/adapters/persistence/concession_repository_test.go`:

```go
package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/domain/model"
)

func TestConcessionRepository_RoundTrip(t *testing.T) {
	fr, q := newForecastRepo(t)
	seedForYear(t, q, 2025)
	ctx := context.Background()

	planned := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	p7, _ := model.NewPartner(7, "X", "Y", "V", "x@e.cat", "6", model.Productor, 1, planned, false)
	uf, _ := model.NewUnsavedExpenseForecast(p7, "Adob", "d", model.MoneyOf(6580), model.ZeroMoney(),
		nil, planned, 2025, "a1", model.NewCommonScope(), planned, true)
	f, err := fr.Create(ctx, uf) // allocates CP25001
	if err != nil {
		t.Fatalf("create forecast: %v", err)
	}

	repo := persistence.NewConcessionRepository(q)
	c, _ := model.NewConcession(2025, "A6-02", "a1", "Adob orgànic", model.MoneyOf(13880), model.MoneyOf(13880))
	if err := repo.Save(ctx, c); err != nil {
		t.Fatalf("Save concession: %v", err)
	}
	if err := repo.ReplaceMembership(ctx, 2025, "A6-02", []string{f.ID()}); err != nil {
		t.Fatalf("ReplaceMembership: %v", err)
	}

	got, err := repo.ListByYear(ctx, 2025)
	if err != nil || len(got) != 1 || got[0].GrantedAmount().Cmp(model.MoneyOf(13880)) != 0 {
		t.Fatalf("ListByYear = (%+v, %v)", got, err)
	}
	links, err := repo.ListForecastLinksByYear(ctx, 2025)
	if err != nil || len(links) != 1 || links[0].ForecastID() != f.ID() {
		t.Fatalf("links = (%+v, %v)", links, err)
	}

	// Delete cascades membership.
	if err := repo.Delete(ctx, 2025, "A6-02"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, _ = repo.ListByYear(ctx, 2025)
	links, _ = repo.ListForecastLinksByYear(ctx, 2025)
	if len(got) != 0 || len(links) != 0 {
		t.Fatalf("after delete: concessions=%d links=%d", len(got), len(links))
	}
}
```

- [ ] **Step 6: Run the test**

Run: `go test ./internal/adapters/persistence/ -run TestConcessionRepository -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/domain/ports internal/adapters/persistence/mapper/concession.go internal/adapters/persistence/concession_repository.go internal/adapters/persistence/txmanager.go internal/adapters/persistence/ports_check.go internal/adapters/persistence/concession_repository_test.go
git commit -m "feat(persistence): ConcessionRepository"
```

---

### Task 5: `InvoiceRepository` — port, mapper, repo, wiring

**Files:**
- Modify: `internal/domain/ports/ports.go`, `internal/domain/ports/tx.go`
- Create: `internal/adapters/persistence/mapper/invoice.go`
- Create: `internal/adapters/persistence/invoice_repository.go`
- Modify: `internal/adapters/persistence/txmanager.go`, `internal/adapters/persistence/ports_check.go`
- Test: `internal/adapters/persistence/invoice_repository_test.go`

**Interfaces:**
- Consumes: Task 1 sqlc methods; `model.Invoice`/`InvoicePayment`/`ForecastInvoice` from Task 3.
- Produces: `ports.InvoiceRepository` with
  - `ListByYear(ctx, year int) ([]model.Invoice, error)` (each aggregate has its payments + links populated)
  - `Save(ctx, inv model.Invoice) (model.Invoice, error)` (insert if `ID()==0`, else update; replaces children; returns invoice with id)
  - `Delete(ctx, invoiceID int) error`
  - `ReplaceForYear(ctx, year int, invoices []model.Invoice) error`
  - Added to `ports.RepoSet` as `Invoices InvoiceRepository`.

- [ ] **Step 1: Add the port interface**

In `internal/domain/ports/ports.go`, add:

```go
// InvoiceRepository manages Invoice aggregates (header + payments + forecast links).
type InvoiceRepository interface {
	ListByYear(ctx context.Context, year int) ([]model.Invoice, error)
	Save(ctx context.Context, inv model.Invoice) (model.Invoice, error)
	Delete(ctx context.Context, invoiceID int) error
	ReplaceForYear(ctx context.Context, year int, invoices []model.Invoice) error
}
```

In `internal/domain/ports/tx.go`, add to `RepoSet`:

```go
	Invoices InvoiceRepository
```

- [ ] **Step 2: Write the mapper**

Create `internal/adapters/persistence/mapper/invoice.go`:

```go
package mapper

import (
	"database/sql"

	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func nullString(p *string) sql.NullString {
	if p == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *p, Valid: true}
}

func stringPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	s := ns.String
	return &s
}

func InvoiceHeaderFromRow(r sqlc.Invoice) (model.Invoice, error) {
	net, err := model.MoneyFromString(r.NetAmount)
	if err != nil {
		return model.Invoice{}, err
	}
	issued, err := ParseDate(r.IssueDate)
	if err != nil {
		return model.Invoice{}, err
	}
	return model.NewInvoice(int(r.ID), int(r.Year), r.Issuer, r.Nif, r.Number, issued,
		net, stringPtr(r.FilePath), stringPtr(r.Notes), nil, nil)
}

func InvoicePaymentFromRow(r sqlc.InvoicePayment) (model.InvoicePayment, error) {
	amt, err := model.MoneyFromString(r.Amount)
	if err != nil {
		return model.InvoicePayment{}, err
	}
	paid, err := ParseDate(r.PaidOn)
	if err != nil {
		return model.InvoicePayment{}, err
	}
	return model.NewInvoicePayment(int(r.ID), int(r.InvoiceID), paid, amt), nil
}

func ForecastInvoiceFromRow(r sqlc.ForecastInvoice) (model.ForecastInvoice, error) {
	amt, err := model.MoneyFromString(r.Amount)
	if err != nil {
		return model.ForecastInvoice{}, err
	}
	return model.NewForecastInvoice(r.ForecastID, int(r.InvoiceID), amt)
}

func InvoiceToInsert(inv model.Invoice) sqlc.InsertInvoiceParams {
	return sqlc.InsertInvoiceParams{
		Year:      int64(inv.Year()),
		Issuer:    inv.Issuer(),
		Nif:       inv.Nif(),
		Number:    inv.Number(),
		IssueDate: FormatDate(inv.IssueDate()),
		NetAmount: inv.NetAmount().String(),
		FilePath:  nullString(inv.FilePath()),
		Notes:     nullString(inv.Notes()),
	}
}

func InvoiceToUpdate(inv model.Invoice) sqlc.UpdateInvoiceParams {
	return sqlc.UpdateInvoiceParams{
		Year:      int64(inv.Year()),
		Issuer:    inv.Issuer(),
		Nif:       inv.Nif(),
		Number:    inv.Number(),
		IssueDate: FormatDate(inv.IssueDate()),
		NetAmount: inv.NetAmount().String(),
		FilePath:  nullString(inv.FilePath()),
		Notes:     nullString(inv.Notes()),
		ID:        int64(inv.ID()),
	}
}
```

- [ ] **Step 3: Write the repository**

Create `internal/adapters/persistence/invoice_repository.go`:

```go
package persistence

import (
	"context"

	"github.com/pjover/espigol/internal/adapters/persistence/mapper"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

type InvoiceRepository struct {
	q *sqlc.Queries
}

func NewInvoiceRepository(q *sqlc.Queries) *InvoiceRepository {
	return &InvoiceRepository{q: q}
}

func (r *InvoiceRepository) ListByYear(ctx context.Context, year int) ([]model.Invoice, error) {
	headers, err := r.q.ListInvoicesByYear(ctx, int64(year))
	if err != nil {
		return nil, err
	}
	payRows, err := r.q.ListInvoicePaymentsByYear(ctx, int64(year))
	if err != nil {
		return nil, err
	}
	linkRows, err := r.q.ListForecastInvoicesByYear(ctx, int64(year))
	if err != nil {
		return nil, err
	}
	paysByInv := map[int][]model.InvoicePayment{}
	for _, pr := range payRows {
		p, err := mapper.InvoicePaymentFromRow(pr)
		if err != nil {
			return nil, err
		}
		paysByInv[p.InvoiceID()] = append(paysByInv[p.InvoiceID()], p)
	}
	linksByInv := map[int][]model.ForecastInvoice{}
	for _, lr := range linkRows {
		l, err := mapper.ForecastInvoiceFromRow(lr)
		if err != nil {
			return nil, err
		}
		linksByInv[l.InvoiceID()] = append(linksByInv[l.InvoiceID()], l)
	}
	out := make([]model.Invoice, 0, len(headers))
	for _, hr := range headers {
		h, err := mapper.InvoiceHeaderFromRow(hr)
		if err != nil {
			return nil, err
		}
		inv, err := model.NewInvoice(h.ID(), h.Year(), h.Issuer(), h.Nif(), h.Number(),
			h.IssueDate(), h.NetAmount(), h.FilePath(), h.Notes(),
			paysByInv[h.ID()], linksByInv[h.ID()])
		if err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, nil
}

// Save inserts (id==0) or updates the header, then replaces children.
func (r *InvoiceRepository) Save(ctx context.Context, inv model.Invoice) (model.Invoice, error) {
	id := int64(inv.ID())
	if inv.ID() == 0 {
		newID, err := r.q.InsertInvoice(ctx, mapper.InvoiceToInsert(inv))
		if err != nil {
			return model.Invoice{}, err
		}
		id = newID
		inv = inv.WithID(int(newID))
	} else {
		if err := r.q.UpdateInvoice(ctx, mapper.InvoiceToUpdate(inv)); err != nil {
			return model.Invoice{}, err
		}
	}
	if err := r.q.DeletePaymentsByInvoice(ctx, id); err != nil {
		return model.Invoice{}, err
	}
	if err := r.q.DeleteForecastInvoicesByInvoice(ctx, id); err != nil {
		return model.Invoice{}, err
	}
	for _, p := range inv.Payments() {
		if err := r.q.InsertInvoicePayment(ctx, sqlc.InsertInvoicePaymentParams{
			InvoiceID: id, PaidOn: mapper.FormatDate(p.PaidOn()), Amount: p.Amount().String(),
		}); err != nil {
			return model.Invoice{}, err
		}
	}
	for _, l := range inv.Links() {
		if err := r.q.InsertForecastInvoice(ctx, sqlc.InsertForecastInvoiceParams{
			ForecastID: l.ForecastID(), InvoiceID: id, Amount: l.Amount().String(),
		}); err != nil {
			return model.Invoice{}, err
		}
	}
	return inv, nil
}

func (r *InvoiceRepository) Delete(ctx context.Context, invoiceID int) error {
	return r.q.DeleteInvoice(ctx, int64(invoiceID)) // children cascade
}

func (r *InvoiceRepository) ReplaceForYear(ctx context.Context, year int, invoices []model.Invoice) error {
	if err := r.q.DeleteInvoicesByYear(ctx, int64(year)); err != nil { // children cascade
		return err
	}
	for _, inv := range invoices {
		if _, err := r.Save(ctx, inv.WithID(0)); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Wire into RepoSet + ports_check**

In `internal/adapters/persistence/txmanager.go` `repos` literal, add:

```go
		Invoices: NewInvoiceRepository(q),
```

In `internal/adapters/persistence/ports_check.go`, add:

```go
	_ ports.InvoiceRepository = (*InvoiceRepository)(nil)
```

- [ ] **Step 5: Write the round-trip test**

Create `internal/adapters/persistence/invoice_repository_test.go`:

```go
package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/domain/model"
)

func TestInvoiceRepository_RoundTrip(t *testing.T) {
	fr, q := newForecastRepo(t)
	seedForYear(t, q, 2025)
	ctx := context.Background()

	planned := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	p7, _ := model.NewPartner(7, "X", "Y", "V", "x@e.cat", "6", model.Productor, 1, planned, false)
	uf, _ := model.NewUnsavedExpenseForecast(p7, "Màquina", "d", model.MoneyOf(500), model.ZeroMoney(),
		nil, planned, 2025, "a1", model.NewCommonScope(), planned, true)
	f, _ := fr.Create(ctx, uf)

	repo := persistence.NewInvoiceRepository(q)
	d := time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC)
	pay := model.NewInvoicePayment(0, 0, d, model.MoneyOf(500))
	link, _ := model.NewForecastInvoice(f.ID(), 0, model.MoneyOf(500))
	inv, _ := model.NewInvoice(0, 2025, "Ribot", "B999", "FD-39521", d, model.MoneyOf(500),
		nil, nil, []model.InvoicePayment{pay}, []model.ForecastInvoice{link})

	saved, err := repo.Save(ctx, inv)
	if err != nil || saved.ID() == 0 {
		t.Fatalf("Save = (%+v, %v)", saved, err)
	}

	got, err := repo.ListByYear(ctx, 2025)
	if err != nil || len(got) != 1 {
		t.Fatalf("ListByYear = (%d, %v)", len(got), err)
	}
	g := got[0]
	if g.Number() != "FD-39521" || len(g.Payments()) != 1 || len(g.Links()) != 1 {
		t.Fatalf("unexpected aggregate: %+v", g)
	}
	if g.PaidTotal().Cmp(model.MoneyOf(500)) != 0 || g.Links()[0].ForecastID() != f.ID() {
		t.Fatalf("children wrong: paid=%s link=%s", g.PaidTotal(), g.Links()[0].ForecastID())
	}

	if err := repo.Delete(ctx, g.ID()); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, _ = repo.ListByYear(ctx, 2025)
	if len(got) != 0 {
		t.Fatalf("after delete: %d invoices", len(got))
	}
}
```

- [ ] **Step 6: Run the test**

Run: `go test ./internal/adapters/persistence/ -run TestInvoiceRepository -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/domain/ports internal/adapters/persistence/mapper/invoice.go internal/adapters/persistence/invoice_repository.go internal/adapters/persistence/txmanager.go internal/adapters/persistence/ports_check.go internal/adapters/persistence/invoice_repository_test.go
git commit -m "feat(persistence): InvoiceRepository (aggregate)"
```

---

### Task 6: `ReconciliationService` — import + CRUD + validation

**Files:**
- Create: `internal/application/reconciliation_service.go`
- Modify: `internal/application/errors.go` (add sentinels)
- Test: `internal/application/reconciliation_service_test.go`

**Interfaces:**
- Consumes: `ports.TxManager`, `ports.RepoSet` (`Concessions`, `Invoices`, `Forecasts`, `Taxonomy`, `Windows`); models from Tasks 2–3.
- Produces:
  - Input DTOs: `application.ConcessionInput{Year int; GroupCode, SubtypeCode, Concept string; RequestedTotal, GrantedAmount model.Money; ForecastIDs []string}`; `application.InvoiceInput{ID, Year int; Issuer, Nif, Number string; IssueDate time.Time; NetAmount model.Money; FilePath, Notes string; Payments []application.PaymentInput; Links []application.LinkInput}`; `application.PaymentInput{PaidOn time.Time; Amount model.Money}`; `application.LinkInput{ForecastID string; Amount model.Money}`.
  - Import DTO: `application.ReconciliationImport{Year int; Concessions []ConcessionInput; Invoices []InvoiceInput}` and `application.ReconciliationImportResult{Concessions, Invoices int; Warnings []string}`.
  - `func NewReconciliationService(tx ports.TxManager) *ReconciliationService`
  - `(*ReconciliationService).AdminImport(ctx, in ReconciliationImport) (ReconciliationImportResult, error)`
  - `(*ReconciliationService).ListConcessions(ctx, year int) ([]model.Concession, error)`
  - `(*ReconciliationService).ListConcessionLinks(ctx, year int) ([]model.ConcessionForecast, error)`
  - `(*ReconciliationService).SaveConcession(ctx, in ConcessionInput) error`
  - `(*ReconciliationService).DeleteConcession(ctx, year int, groupCode string) error`
  - `(*ReconciliationService).ListInvoices(ctx, year int) ([]model.Invoice, error)`
  - `(*ReconciliationService).SaveInvoice(ctx, in InvoiceInput) (model.Invoice, error)`
  - `(*ReconciliationService).DeleteInvoice(ctx, invoiceID int) error`

- [ ] **Step 1: Add sentinel errors**

In `internal/application/errors.go`, add to the `errors.New` block:

```go
	ErrConcessionSubtypeMissing = errors.New("concession subtype not found for year")
	ErrReconForecastMissing     = errors.New("referenced forecast not found for year")
```

- [ ] **Step 2: Write the failing tests**

Create `internal/application/reconciliation_service_test.go`:

```go
package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

// buildReconWorld seeds a 2025 window + subtype a6 + partner + one forecast and
// returns the tx manager and the forecast id. Reuses the shared newTestTxWorld
// helper convention from window_service_test.go.
func TestReconciliationImport_HappyPathAndWarnings(t *testing.T) {
	world := newReconWorld(t) // helper below
	svc := application.NewReconciliationService(world.tx)
	ctx := context.Background()

	in := application.ReconciliationImport{
		Year: 2025,
		Concessions: []application.ConcessionInput{{
			Year: 2025, GroupCode: "A6-02", SubtypeCode: "a6", Concept: "Adob orgànic",
			RequestedTotal: model.MoneyOf(6580), GrantedAmount: model.MoneyOf(6580),
			ForecastIDs: []string{world.forecastID},
		}},
		Invoices: []application.InvoiceInput{{
			Year: 2025, Issuer: "Sup", Nif: "B1", Number: "F1",
			IssueDate: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC), NetAmount: model.MoneyOf(500),
			Payments: []application.PaymentInput{{PaidOn: time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC), Amount: model.MoneyOf(500)}},
			Links:    []application.LinkInput{{ForecastID: world.forecastID, Amount: model.MoneyOf(500)}},
		}},
	}
	res, err := svc.AdminImport(ctx, in)
	if err != nil {
		t.Fatalf("AdminImport: %v", err)
	}
	if res.Concessions != 1 || res.Invoices != 1 {
		t.Fatalf("counts: %+v", res)
	}
	// forecast GrossAmount is 500 but Demanat is 6580 -> soft warning expected.
	if len(res.Warnings) == 0 {
		t.Errorf("expected a Demanat vs Previst warning")
	}

	got, _ := svc.ListConcessions(ctx, 2025)
	if len(got) != 1 {
		t.Fatalf("ListConcessions = %d", len(got))
	}
}

func TestReconciliationImport_UnknownForecastRollsBack(t *testing.T) {
	world := newReconWorld(t)
	svc := application.NewReconciliationService(world.tx)
	ctx := context.Background()

	in := application.ReconciliationImport{
		Year: 2025,
		Concessions: []application.ConcessionInput{{
			Year: 2025, GroupCode: "A6-02", SubtypeCode: "a6", Concept: "x",
			RequestedTotal: model.ZeroMoney(), GrantedAmount: model.ZeroMoney(),
			ForecastIDs: []string{"CP25999"}, // does not exist
		}},
	}
	if _, err := svc.AdminImport(ctx, in); err == nil {
		t.Fatal("expected error for unknown forecast")
	}
	got, _ := svc.ListConcessions(ctx, 2025)
	if len(got) != 0 {
		t.Fatalf("rollback failed: %d concessions", len(got))
	}
}

func TestReconciliationImport_UnknownSubtypeFails(t *testing.T) {
	world := newReconWorld(t)
	svc := application.NewReconciliationService(world.tx)
	ctx := context.Background()
	in := application.ReconciliationImport{
		Year: 2025,
		Concessions: []application.ConcessionInput{{
			Year: 2025, GroupCode: "Z9-01", SubtypeCode: "zz", Concept: "x",
			RequestedTotal: model.ZeroMoney(), GrantedAmount: model.ZeroMoney(),
		}},
	}
	if _, err := svc.AdminImport(ctx, in); err == nil {
		t.Fatal("expected error for unknown subtype")
	}
}
```

- [ ] **Step 3: Add the `newReconWorld` test helper**

Append to `internal/application/reconciliation_service_test.go`:

```go
type reconWorld struct {
	tx         *persistence.TxManager
	forecastID string
}

func newReconWorld(t *testing.T) reconWorld {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	q := sqlc.New(conn)
	ctx := context.Background()

	win := persistence.NewWindowRepository(q)
	tax := persistence.NewTaxonomyRepository(q)
	pr := persistence.NewPartnerRepository(q)
	fr := persistence.NewForecastRepository(conn, q)

	w, _ := model.NewSubmissionWindow(2025, model.WindowClosed, nil, nil,
		time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	_ = win.Save(ctx, w)
	typ, _ := model.NewExpenseType(2025, "A", "[a]", model.CategoryCurrent)
	_ = tax.SaveType(ctx, typ)
	st, _ := model.NewExpenseSubtype(2025, "a6", "[a6]", "A")
	_ = tax.SaveSubtype(ctx, st)
	planned := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	p7, _ := model.NewPartner(7, "X", "Y", "V", "x@e.cat", "6", model.Productor, 1, planned, false)
	_ = pr.Save(ctx, p7)
	uf, _ := model.NewUnsavedExpenseForecast(p7, "Adob", "d", model.MoneyOf(500), model.ZeroMoney(),
		nil, planned, 2025, "a6", model.NewCommonScope(), planned, true)
	f, _ := fr.Create(ctx, uf)

	return reconWorld{tx: persistence.NewTxManager(conn), forecastID: f.ID()}
}
```

Add the imports this helper needs at the top of the test file: `"path/filepath"`, `"github.com/pjover/espigol/internal/adapters/persistence"`, `"github.com/pjover/espigol/internal/adapters/persistence/db"`, `"github.com/pjover/espigol/internal/adapters/persistence/sqlc"`.

- [ ] **Step 4: Run to verify failure**

Run: `go test ./internal/application/ -run TestReconciliation -v`
Expected: FAIL — undefined `application.NewReconciliationService`.

- [ ] **Step 5: Implement the service**

Create `internal/application/reconciliation_service.go`:

```go
package application

import (
	"context"
	"fmt"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/ports"
)

// PaymentInput / LinkInput / ConcessionInput / InvoiceInput are the driving-side
// DTOs the TUI and importer build. Amounts are already model.Money.
type PaymentInput struct {
	PaidOn time.Time
	Amount model.Money
}

type LinkInput struct {
	ForecastID string
	Amount     model.Money
}

type ConcessionInput struct {
	Year           int
	GroupCode      string
	SubtypeCode    string
	Concept        string
	RequestedTotal model.Money
	GrantedAmount  model.Money
	ForecastIDs    []string
}

type InvoiceInput struct {
	ID        int
	Year      int
	Issuer    string
	Nif       string
	Number    string
	IssueDate time.Time
	NetAmount model.Money
	FilePath  string
	Notes     string
	Payments  []PaymentInput
	Links     []LinkInput
}

type ReconciliationImport struct {
	Year        int
	Concessions []ConcessionInput
	Invoices    []InvoiceInput
}

type ReconciliationImportResult struct {
	Concessions int
	Invoices    int
	Warnings    []string
}

type ReconciliationService struct {
	tx ports.TxManager
}

func NewReconciliationService(tx ports.TxManager) *ReconciliationService {
	return &ReconciliationService{tx: tx}
}

func strOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// buildInvoice converts an InvoiceInput into a model.Invoice aggregate.
func buildInvoice(in InvoiceInput) (model.Invoice, error) {
	pays := make([]model.InvoicePayment, 0, len(in.Payments))
	for _, p := range in.Payments {
		pays = append(pays, model.NewInvoicePayment(0, in.ID, p.PaidOn, p.Amount))
	}
	links := make([]model.ForecastInvoice, 0, len(in.Links))
	for _, l := range in.Links {
		fi, err := model.NewForecastInvoice(l.ForecastID, in.ID, l.Amount)
		if err != nil {
			return model.Invoice{}, err
		}
		links = append(links, fi)
	}
	return model.NewInvoice(in.ID, in.Year, in.Issuer, in.Nif, in.Number, in.IssueDate,
		in.NetAmount, strOrNil(in.FilePath), strOrNil(in.Notes), pays, links)
}

// validateReferences checks subtype + forecast existence for the year and
// returns soft-check warnings (Catalan). Hard failures return an error.
func validateReferences(ctx context.Context, r ports.RepoSet, in ReconciliationImport) ([]string, error) {
	subs, err := r.Taxonomy.ListSubtypes(ctx, in.Year)
	if err != nil {
		return nil, err
	}
	subCodes := map[string]bool{}
	for _, s := range subs {
		subCodes[s.Code()] = true
	}
	forecasts, err := r.Forecasts.ListByYear(ctx, in.Year)
	if err != nil {
		return nil, err
	}
	grossByID := map[string]model.Money{}
	for _, f := range forecasts {
		grossByID[f.ID()] = f.GrossAmount()
	}

	var warnings []string
	for _, c := range in.Concessions {
		if !subCodes[c.SubtypeCode] {
			return nil, fmt.Errorf("%w: group %s subtype %q", ErrConcessionSubtypeMissing, c.GroupCode, c.SubtypeCode)
		}
		sumPrevist := model.ZeroMoney()
		for _, fid := range c.ForecastIDs {
			g, ok := grossByID[fid]
			if !ok {
				return nil, fmt.Errorf("%w: group %s forecast %q", ErrReconForecastMissing, c.GroupCode, fid)
			}
			sumPrevist = sumPrevist.Plus(g)
		}
		if c.GrantedAmount.Cmp(c.RequestedTotal) > 0 {
			warnings = append(warnings, fmt.Sprintf("Concessió %s: Concedit (%s) > Demanat (%s)",
				c.GroupCode, c.GrantedAmount, c.RequestedTotal))
		}
		if len(c.ForecastIDs) > 0 && c.RequestedTotal.Minus(sumPrevist).Decimal().Abs().Cmp(cent.Decimal()) > 0 {
			warnings = append(warnings, fmt.Sprintf("Concessió %s: Demanat (%s) ≠ Σ Previst (%s)",
				c.GroupCode, c.RequestedTotal, sumPrevist))
		}
	}
	for _, inv := range in.Invoices {
		sumLinks := model.ZeroMoney()
		for _, l := range inv.Links {
			if _, ok := grossByID[l.ForecastID]; !ok {
				return nil, fmt.Errorf("%w: invoice %s forecast %q", ErrReconForecastMissing, inv.Number, l.ForecastID)
			}
			sumLinks = sumLinks.Plus(l.Amount)
		}
		if sumLinks.Cmp(inv.NetAmount) > 0 {
			warnings = append(warnings, fmt.Sprintf("Factura %s: Σ enllaços (%s) > Import (%s)",
				inv.Number, sumLinks, inv.NetAmount))
		}
		sumPays := model.ZeroMoney()
		for _, p := range inv.Payments {
			sumPays = sumPays.Plus(p.Amount)
		}
		if sumPays.Cmp(inv.NetAmount) > 0 {
			warnings = append(warnings, fmt.Sprintf("Factura %s: Σ pagaments (%s) > Import (%s)",
				inv.Number, sumPays, inv.NetAmount))
		}
	}
	return warnings, nil
}

func (s *ReconciliationService) AdminImport(ctx context.Context, in ReconciliationImport) (ReconciliationImportResult, error) {
	var res ReconciliationImportResult
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		if _, ok, err := r.Windows.FindByYear(ctx, in.Year); err != nil {
			return err
		} else if !ok {
			return ErrWindowNotFound
		}

		warnings, err := validateReferences(ctx, r, in)
		if err != nil {
			return err
		}

		// Build concessions + membership links.
		concessions := make([]model.Concession, 0, len(in.Concessions))
		var links []model.ConcessionForecast
		for _, c := range in.Concessions {
			mc, err := model.NewConcession(c.Year, c.GroupCode, c.SubtypeCode, c.Concept, c.RequestedTotal, c.GrantedAmount)
			if err != nil {
				return err
			}
			concessions = append(concessions, mc)
			for _, fid := range c.ForecastIDs {
				cf, err := model.NewConcessionForecast(c.Year, c.GroupCode, fid)
				if err != nil {
					return err
				}
				links = append(links, cf)
			}
		}
		if err := r.Concessions.ReplaceForYear(ctx, in.Year, concessions, links); err != nil {
			return err
		}

		invoices := make([]model.Invoice, 0, len(in.Invoices))
		for _, invIn := range in.Invoices {
			inv, err := buildInvoice(invIn)
			if err != nil {
				return err
			}
			invoices = append(invoices, inv)
		}
		if err := r.Invoices.ReplaceForYear(ctx, in.Year, invoices); err != nil {
			return err
		}

		res = ReconciliationImportResult{Concessions: len(concessions), Invoices: len(invoices), Warnings: warnings}
		return nil
	})
	if err != nil {
		return ReconciliationImportResult{}, err
	}
	return res, nil
}

func (s *ReconciliationService) ListConcessions(ctx context.Context, year int) ([]model.Concession, error) {
	var out []model.Concession
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		var err error
		out, err = r.Concessions.ListByYear(ctx, year)
		return err
	})
	return out, err
}

func (s *ReconciliationService) ListConcessionLinks(ctx context.Context, year int) ([]model.ConcessionForecast, error) {
	var out []model.ConcessionForecast
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		var err error
		out, err = r.Concessions.ListForecastLinksByYear(ctx, year)
		return err
	})
	return out, err
}

func (s *ReconciliationService) SaveConcession(ctx context.Context, in ConcessionInput) error {
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		subs, err := r.Taxonomy.ListSubtypes(ctx, in.Year)
		if err != nil {
			return err
		}
		found := false
		for _, sub := range subs {
			if sub.Code() == in.SubtypeCode {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("%w: %q", ErrConcessionSubtypeMissing, in.SubtypeCode)
		}
		c, err := model.NewConcession(in.Year, in.GroupCode, in.SubtypeCode, in.Concept, in.RequestedTotal, in.GrantedAmount)
		if err != nil {
			return err
		}
		if err := r.Concessions.Save(ctx, c); err != nil {
			return err
		}
		return r.Concessions.ReplaceMembership(ctx, in.Year, in.GroupCode, in.ForecastIDs)
	})
}

func (s *ReconciliationService) DeleteConcession(ctx context.Context, year int, groupCode string) error {
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		return r.Concessions.Delete(ctx, year, groupCode)
	})
}

func (s *ReconciliationService) ListInvoices(ctx context.Context, year int) ([]model.Invoice, error) {
	var out []model.Invoice
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		var err error
		out, err = r.Invoices.ListByYear(ctx, year)
		return err
	})
	return out, err
}

func (s *ReconciliationService) SaveInvoice(ctx context.Context, in InvoiceInput) (model.Invoice, error) {
	var saved model.Invoice
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		inv, err := buildInvoice(in)
		if err != nil {
			return err
		}
		saved, err = r.Invoices.Save(ctx, inv)
		return err
	})
	return saved, err
}

func (s *ReconciliationService) DeleteInvoice(ctx context.Context, invoiceID int) error {
	return s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		return r.Invoices.Delete(ctx, invoiceID)
	})
}
```

- [ ] **Step 6: Add the `cent` helper if not present**

The soft check uses a `cent` threshold. Add to `internal/application/reconciliation_service.go` package-level (only if the project has no existing equivalent — grep first with `grep -rn "0.01" internal/application`):

```go
// cent is the 0,01 € tolerance for soft integrity checks (matches the workbook's
// ABS(diff) < 0.01 OK? test).
var cent = mustCent()

func mustCent() model.Money {
	c, _ := model.MoneyFromString("0.01")
	return c
}
```

The soft check already compares against `cent.Decimal()` (`…Decimal().Abs().Cmp(cent.Decimal())`), so no further change is needed.

- [ ] **Step 7: Run to verify pass**

Run: `go test ./internal/application/ -run TestReconciliation -v`
Expected: PASS (happy path returns a warning; unknown forecast + subtype error and roll back).

- [ ] **Step 8: Commit**

```bash
git add internal/application/reconciliation_service.go internal/application/reconciliation_service_test.go internal/application/errors.go
git commit -m "feat(application): ReconciliationService import + CRUD + validation"
```

---

### Task 7: Importer adapter — `LoadReconciliation`

**Files:**
- Create: `internal/adapters/importer/reconciliation.go`
- Test: `internal/adapters/importer/reconciliation_test.go`

**Interfaces:**
- Consumes: `application.ReconciliationImport` + DTOs from Task 6; `model.MoneyFromString`.
- Produces: `func importer.LoadReconciliation(path string, year int) (application.ReconciliationImport, error)`. JSON shape per spec §4: top-level `year`, `concessions[]` (`groupCode`, `subtypeCode`, `concept`, `requestedTotal`, `grantedAmount`, `forecastIds[]`), `invoices[]` (`issuer`, `nif`, `number`, `issueDate`, `netAmount`, `filePath`, `notes`, `payments[]{paidOn,amount}`, `links[]{forecastId,amount}`).

- [ ] **Step 1: Write the failing test**

Create `internal/adapters/importer/reconciliation_test.go`:

```go
package importer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pjover/espigol/internal/adapters/importer"
	"github.com/pjover/espigol/internal/domain/model"
)

func TestLoadReconciliation_ParsesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reconciliation-2025.json")
	os.WriteFile(path, []byte(`{
      "year": 2025,
      "concessions": [
        {"groupCode":"A6-02","subtypeCode":"a6","concept":"Adob orgànic",
         "requestedTotal":"13880.00","grantedAmount":"13880.00","forecastIds":["CP25008","CP25009"]}
      ],
      "invoices": [
        {"issuer":"Ribot","nif":"B999","number":"FD-39521","issueDate":"2025-03-14",
         "netAmount":"500.00","filePath":"x.pdf","notes":"n",
         "payments":[{"paidOn":"2025-04-01","amount":"500.00"}],
         "links":[{"forecastId":"CP25030","amount":"500.00"}]}
      ]
    }`), 0o644)

	in, err := importer.LoadReconciliation(path, 2025)
	if err != nil {
		t.Fatalf("LoadReconciliation: %v", err)
	}
	if len(in.Concessions) != 1 || in.Concessions[0].GroupCode != "A6-02" {
		t.Fatalf("concessions: %+v", in.Concessions)
	}
	if in.Concessions[0].GrantedAmount.Cmp(model.MoneyOf(13880)) != 0 {
		t.Errorf("granted = %s", in.Concessions[0].GrantedAmount)
	}
	if len(in.Concessions[0].ForecastIDs) != 2 {
		t.Errorf("forecastIds = %v", in.Concessions[0].ForecastIDs)
	}
	if len(in.Invoices) != 1 || len(in.Invoices[0].Payments) != 1 || len(in.Invoices[0].Links) != 1 {
		t.Fatalf("invoices: %+v", in.Invoices)
	}
}

func TestLoadReconciliation_YearMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "reconciliation-2025.json")
	os.WriteFile(path, []byte(`{"year":2024,"concessions":[],"invoices":[]}`), 0o644)
	if _, err := importer.LoadReconciliation(path, 2025); err == nil {
		t.Fatal("expected year-mismatch error")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/adapters/importer/ -run TestLoadReconciliation -v`
Expected: FAIL — undefined `importer.LoadReconciliation`.

- [ ] **Step 3: Implement the loader**

Create `internal/adapters/importer/reconciliation.go`:

```go
// Package importer also reads reconciliation import files
// (Home/import/reconciliation-<year>.json) into an application.ReconciliationImport.
// It performs format/parse validation only; referential integrity is enforced by
// ReconciliationService.AdminImport.
package importer

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

type reconDoc struct {
	Year        int                 `json:"year"`
	Concessions []reconConcession   `json:"concessions"`
	Invoices    []reconInvoice      `json:"invoices"`
}

type reconConcession struct {
	GroupCode      string   `json:"groupCode"`
	SubtypeCode    string   `json:"subtypeCode"`
	Concept        string   `json:"concept"`
	RequestedTotal string   `json:"requestedTotal"`
	GrantedAmount  string   `json:"grantedAmount"`
	ForecastIDs    []string `json:"forecastIds"`
}

type reconInvoice struct {
	Issuer    string         `json:"issuer"`
	Nif       string         `json:"nif"`
	Number    string         `json:"number"`
	IssueDate string         `json:"issueDate"`
	NetAmount string         `json:"netAmount"`
	FilePath  string         `json:"filePath"`
	Notes     string         `json:"notes"`
	Payments  []reconPayment `json:"payments"`
	Links     []reconLink    `json:"links"`
}

type reconPayment struct {
	PaidOn string `json:"paidOn"`
	Amount string `json:"amount"`
}

type reconLink struct {
	ForecastID string `json:"forecastId"`
	Amount     string `json:"amount"`
}

const reconDateLayout = "2006-01-02"

func LoadReconciliation(path string, year int) (application.ReconciliationImport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return application.ReconciliationImport{}, fmt.Errorf("reading import file: %w", err)
	}
	var doc reconDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return application.ReconciliationImport{}, fmt.Errorf("parsing import file: %w", err)
	}
	if doc.Year != year {
		return application.ReconciliationImport{}, fmt.Errorf("file year %d does not match selected year %d", doc.Year, year)
	}

	out := application.ReconciliationImport{Year: year}
	for i, c := range doc.Concessions {
		req, err := model.MoneyFromString(c.RequestedTotal)
		if err != nil {
			return application.ReconciliationImport{}, fmt.Errorf("concession[%d]: invalid requestedTotal %q: %w", i, c.RequestedTotal, err)
		}
		granted, err := model.MoneyFromString(c.GrantedAmount)
		if err != nil {
			return application.ReconciliationImport{}, fmt.Errorf("concession[%d]: invalid grantedAmount %q: %w", i, c.GrantedAmount, err)
		}
		out.Concessions = append(out.Concessions, application.ConcessionInput{
			Year: year, GroupCode: c.GroupCode, SubtypeCode: c.SubtypeCode, Concept: c.Concept,
			RequestedTotal: req, GrantedAmount: granted, ForecastIDs: c.ForecastIDs,
		})
	}
	for i, inv := range doc.Invoices {
		net, err := model.MoneyFromString(inv.NetAmount)
		if err != nil {
			return application.ReconciliationImport{}, fmt.Errorf("invoice[%d]: invalid netAmount %q: %w", i, inv.NetAmount, err)
		}
		issued, err := time.Parse(reconDateLayout, inv.IssueDate)
		if err != nil {
			return application.ReconciliationImport{}, fmt.Errorf("invoice[%d]: invalid issueDate %q: %w", i, inv.IssueDate, err)
		}
		var pays []application.PaymentInput
		for j, p := range inv.Payments {
			amt, err := model.MoneyFromString(p.Amount)
			if err != nil {
				return application.ReconciliationImport{}, fmt.Errorf("invoice[%d].payment[%d]: invalid amount %q: %w", i, j, p.Amount, err)
			}
			paid, err := time.Parse(reconDateLayout, p.PaidOn)
			if err != nil {
				return application.ReconciliationImport{}, fmt.Errorf("invoice[%d].payment[%d]: invalid paidOn %q: %w", i, j, p.PaidOn, err)
			}
			pays = append(pays, application.PaymentInput{PaidOn: paid, Amount: amt})
		}
		var links []application.LinkInput
		for j, l := range inv.Links {
			amt, err := model.MoneyFromString(l.Amount)
			if err != nil {
				return application.ReconciliationImport{}, fmt.Errorf("invoice[%d].link[%d]: invalid amount %q: %w", i, j, l.Amount, err)
			}
			links = append(links, application.LinkInput{ForecastID: l.ForecastID, Amount: amt})
		}
		out.Invoices = append(out.Invoices, application.InvoiceInput{
			Year: year, Issuer: inv.Issuer, Nif: inv.Nif, Number: inv.Number, IssueDate: issued,
			NetAmount: net, FilePath: inv.FilePath, Notes: inv.Notes, Payments: pays, Links: links,
		})
	}
	return out, nil
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/adapters/importer/ -run TestLoadReconciliation -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/importer/reconciliation.go internal/adapters/importer/reconciliation_test.go
git commit -m "feat(importer): LoadReconciliation JSON parser"
```

---

### Task 8: Wire `ReconciliationService` into the TUI

**Files:**
- Modify: `internal/adapters/tui/deps.go` (add `Reconciliation` field)
- Modify: `internal/wire/wire.go` (construct + inject)
- Test: `internal/wire/wire_test.go` (if present) or a build check

**Interfaces:**
- Consumes: `application.NewReconciliationService` from Task 6.
- Produces: `tui.Deps.Reconciliation *application.ReconciliationService`, populated in `wire.TUI`.

- [ ] **Step 1: Add the Deps field**

In `internal/adapters/tui/deps.go`, add to the `Deps` struct (after `Reports`):

```go
	Reconciliation *application.ReconciliationService
```

- [ ] **Step 2: Construct it in wire.TUI**

In `internal/wire/wire.go`, inside `TUI(...)`, add to the `deps := tui.Deps{…}` literal:

```go
		Reconciliation: application.NewReconciliationService(txm),
```

- [ ] **Step 3: Verify it builds**

Run: `go build ./... && make vet`
Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/adapters/tui/deps.go internal/wire/wire.go
git commit -m "feat(wire): inject ReconciliationService into TUI Deps"
```

---

### Task 9: "Ajuts" panel — skeleton + read-only lists + registration

**Files:**
- Create: `internal/adapters/tui/panel_ajuts.go`
- Modify: `internal/wire/wire.go` (append `tui.NewAjutsPanel(deps)` to the panels slice)
- Test: `internal/adapters/tui/panel_ajuts_test.go`

**Interfaces:**
- Consumes: `Deps.Reconciliation`; `yearSelectedMsg{Year int}`, `panelInitMsg`, `Panel`, `Action`, `openModalCmd`, `newConfirmModal`, styles (`focusedPanelStyle`, `dimStyle`), `truncate`, `scrollOffset` (existing tui helpers).
- Produces: `func NewAjutsPanel(deps Deps) Panel`. Panel `Title() == "Ajuts"`. Internal view toggle between Concessions and Factures with `tab`.

- [ ] **Step 1: Write the failing test**

Create `internal/adapters/tui/panel_ajuts_test.go`:

```go
package tui

import (
	"testing"
)

func TestAjutsPanel_Title(t *testing.T) {
	p := NewAjutsPanel(Deps{})
	if p.Title() != "Ajuts" {
		t.Errorf("Title = %q, want Ajuts", p.Title())
	}
}

func TestAjutsPanel_EmptyView(t *testing.T) {
	p := NewAjutsPanel(Deps{})
	out := p.View(80, 10)
	if out == "" {
		t.Error("expected non-empty view")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/adapters/tui/ -run TestAjutsPanel -v`
Expected: FAIL — undefined `NewAjutsPanel`.

- [ ] **Step 3: Implement the panel skeleton**

Create `internal/adapters/tui/panel_ajuts.go`:

```go
package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pjover/espigol/internal/domain/model"
)

type ajutsView int

const (
	ajutsConcessions ajutsView = iota
	ajutsInvoices
)

// ajutsLoadedMsg carries a (re)load of the year's concessions + invoices. err
// follows the reload-priority convention (mutation error wins over reload error).
type ajutsLoadedMsg struct {
	year        int
	concessions []model.Concession
	links       []model.ConcessionForecast
	invoices    []model.Invoice
	err         error
}

type ajutsPanel struct {
	deps        Deps
	year        int
	view        ajutsView
	concessions []model.Concession
	links       []model.ConcessionForecast
	invoices    []model.Invoice
	selected    int
	err         error
}

func NewAjutsPanel(deps Deps) Panel { return ajutsPanel{deps: deps} }

func (p ajutsPanel) Title() string { return "Ajuts" }

func (p ajutsPanel) load(ctx context.Context, year int) ajutsLoadedMsg {
	if p.deps.Reconciliation == nil {
		return ajutsLoadedMsg{year: year}
	}
	cs, err := p.deps.Reconciliation.ListConcessions(ctx, year)
	if err != nil {
		return ajutsLoadedMsg{year: year, err: err}
	}
	links, err := p.deps.Reconciliation.ListConcessionLinks(ctx, year)
	if err != nil {
		return ajutsLoadedMsg{year: year, err: err}
	}
	inv, err := p.deps.Reconciliation.ListInvoices(ctx, year)
	if err != nil {
		return ajutsLoadedMsg{year: year, err: err}
	}
	return ajutsLoadedMsg{year: year, concessions: cs, links: links, invoices: inv}
}

func (p ajutsPanel) loadCmd() tea.Cmd {
	year := p.year
	return func() tea.Msg { return p.load(context.Background(), year) }
}

// reloadCmd runs a mutation then reloads; the mutation error takes priority.
func (p ajutsPanel) reloadCmd(run func(ctx context.Context) error) tea.Cmd {
	year := p.year
	return func() tea.Msg {
		mutateErr := run(context.Background())
		msg := p.load(context.Background(), year)
		if mutateErr != nil {
			msg.err = mutateErr
		}
		return msg
	}
}

func (p ajutsPanel) rowCount() int {
	if p.view == ajutsConcessions {
		return len(p.concessions)
	}
	return len(p.invoices)
}

func (p ajutsPanel) Update(msg tea.Msg) (Panel, tea.Cmd) {
	switch msg := msg.(type) {
	case panelInitMsg:
		return p, p.loadCmd()
	case yearSelectedMsg:
		p.year = msg.Year
		p.selected = 0
		return p, p.loadCmd()
	case ajutsLoadedMsg:
		if msg.year != p.year {
			return p, nil
		}
		p.concessions = msg.concessions
		p.links = msg.links
		p.invoices = msg.invoices
		p.err = msg.err
		if p.selected >= p.rowCount() {
			p.selected = max(0, p.rowCount()-1)
		}
		return p, nil
	case tea.KeyMsg:
		return p.handleKey(msg)
	}
	return p, nil
}

func (p ajutsPanel) handleKey(msg tea.KeyMsg) (Panel, tea.Cmd) {
	switch msg.String() {
	case "tab":
		if p.view == ajutsConcessions {
			p.view = ajutsInvoices
		} else {
			p.view = ajutsConcessions
		}
		p.selected = 0
		return p, nil
	case "up", "k":
		if p.selected > 0 {
			p.selected--
		}
		return p, nil
	case "down", "j":
		if p.selected < p.rowCount()-1 {
			p.selected++
		}
		return p, nil
	}
	return p, nil
}

// forecastIDsFor returns the CP ids linked to a concession group, comma-joined.
func (p ajutsPanel) forecastIDsFor(groupCode string) string {
	var ids []string
	for _, l := range p.links {
		if l.GroupCode() == groupCode {
			ids = append(ids, l.ForecastID())
		}
	}
	return strings.Join(ids, ",")
}

func (p ajutsPanel) View(width, height int) string {
	header := "[ Concessions | Factures ]"
	if p.view == ajutsInvoices {
		header = "[ concessions | FACTURES ]"
	} else {
		header = "[ CONCESSIONS | factures ]"
	}
	var b strings.Builder
	b.WriteString(dimStyle.Render(truncate(header+"   (tab per canviar)", width)))
	b.WriteString("\n")

	if p.view == ajutsConcessions {
		if len(p.concessions) == 0 {
			b.WriteString(dimStyle.Render("(cap concessió)"))
			return b.String()
		}
		listH := height - 1
		off := scrollOffset(p.selected, len(p.concessions), listH)
		end := min(off+listH, len(p.concessions))
		for i := off; i < end; i++ {
			c := p.concessions[i]
			raw := truncate(fmt.Sprintf("%s  %s  Demanat %s  Concedit %s",
				c.GroupCode(), c.Concept(), c.RequestedTotal(), c.GrantedAmount()), width-2)
			b.WriteString(renderRow(raw, i == p.selected))
			b.WriteString("\n")
		}
		return b.String()
	}

	if len(p.invoices) == 0 {
		b.WriteString(dimStyle.Render("(cap factura)"))
		return b.String()
	}
	listH := height - 1
	off := scrollOffset(p.selected, len(p.invoices), listH)
	end := min(off+listH, len(p.invoices))
	for i := off; i < end; i++ {
		inv := p.invoices[i]
		raw := truncate(fmt.Sprintf("%s  %s  Import %s  %s",
			inv.Number(), inv.Issuer(), inv.NetAmount(), paidLabel(inv)), width-2)
		b.WriteString(renderRow(raw, i == p.selected))
		b.WriteString("\n")
	}
	return b.String()
}

func renderRow(raw string, selected bool) string {
	if selected {
		return focusedPanelStyle.Render("> " + raw)
	}
	return "  " + raw
}

// paidLabel is the Catalan payment status derived from payments vs net.
func paidLabel(inv model.Invoice) string {
	paid := inv.PaidTotal()
	switch {
	case paid.IsZero():
		return "no pagat"
	case paid.Cmp(inv.NetAmount()) >= 0:
		return "pagat"
	default:
		return "parcial"
	}
}

func (p ajutsPanel) Detail() string {
	if p.err != nil {
		return errDetail(p.err)
	}
	if p.view == ajutsConcessions {
		if p.selected < 0 || p.selected >= len(p.concessions) {
			return ""
		}
		c := p.concessions[p.selected]
		return fmt.Sprintf("Concessió %s · %s · subtipus %s · Previsions: %s",
			c.GroupCode(), c.Concept(), c.SubtypeCode(), p.forecastIDsFor(c.GroupCode()))
	}
	if p.selected < 0 || p.selected >= len(p.invoices) {
		return ""
	}
	inv := p.invoices[p.selected]
	return fmt.Sprintf("Factura %s · %s · Import %s · %d enllaços · %s",
		inv.Number(), inv.Issuer(), inv.NetAmount(), len(inv.Links()), paidLabel(inv))
}

func (p ajutsPanel) Actions() []Action {
	return []Action{{Key: "tab", Label: "canvia vista"}}
}
```

Note: if `min`/`max` generic builtins are not already used elsewhere in the package, they are provided by Go 1.21+ builtins (the repo already uses `max` in `panel_taxonomy.go`).

- [ ] **Step 4: Register the panel**

In `internal/wire/wire.go`, append to the `panels := []tui.Panel{…}` slice (after `tui.NewForecastsPanel(deps)`):

```go
		tui.NewAjutsPanel(deps),
```

- [ ] **Step 5: Run tests + build**

Run: `go test ./internal/adapters/tui/ -run TestAjutsPanel -v && go build ./...`
Expected: PASS + clean build.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/tui/panel_ajuts.go internal/adapters/tui/panel_ajuts_test.go internal/wire/wire.go
git commit -m "feat(tui): Ajuts panel with concessions + factures lists"
```

---

### Task 10: "Ajuts" panel — JSON import trigger (`i`)

**Files:**
- Modify: `internal/adapters/tui/panel_ajuts.go`
- Test: extend `internal/adapters/tui/panel_ajuts_test.go`

**Interfaces:**
- Consumes: `importer.LoadReconciliation` (Task 7), `Deps.Reconciliation.AdminImport` (Task 6), `Deps.Cfg.ImportDir`.
- Produces: an `i` key that loads `import/reconciliation-<year>.json` and replaces the year's reconciliation data, then reloads; a transient status line in `Detail()`.

- [ ] **Step 1: Write the failing test**

Add to `internal/adapters/tui/panel_ajuts_test.go`:

```go
func TestAjutsPanel_ImportActionAdvertised(t *testing.T) {
	p := NewAjutsPanel(Deps{})
	found := false
	for _, a := range p.(ajutsPanel).Actions() {
		if a.Key == "i" {
			found = true
		}
	}
	if !found {
		t.Error("expected an 'i' import action")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/adapters/tui/ -run TestAjutsPanel_ImportActionAdvertised -v`
Expected: FAIL — no `i` action.

- [ ] **Step 3: Add the import command + key + status**

In `internal/adapters/tui/panel_ajuts.go`:

Add imports `"path/filepath"` and `"github.com/pjover/espigol/internal/adapters/importer"`.

Add a status field to the struct: `status string` (after `err error`).

Add a message type and command:

```go
type ajutsImportedMsg struct {
	year   int
	result string
	err    error
}

func (p ajutsPanel) importCmd() tea.Cmd {
	year := p.year
	deps := p.deps
	return func() tea.Msg {
		if deps.Reconciliation == nil || deps.Cfg == nil {
			return ajutsImportedMsg{year: year, err: fmt.Errorf("importació no disponible")}
		}
		path := filepath.Join(deps.Cfg.ImportDir, fmt.Sprintf("reconciliation-%d.json", year))
		in, err := importer.LoadReconciliation(path, year)
		if err != nil {
			return ajutsImportedMsg{year: year, err: err}
		}
		res, err := deps.Reconciliation.AdminImport(context.Background(), in)
		if err != nil {
			return ajutsImportedMsg{year: year, err: err}
		}
		msg := fmt.Sprintf("Importat: %d concessions, %d factures", res.Concessions, res.Invoices)
		if len(res.Warnings) > 0 {
			msg += fmt.Sprintf(" (%d avisos)", len(res.Warnings))
		}
		return ajutsImportedMsg{year: year, result: msg}
	}
}
```

In `handleKey`, add before the final `return`:

```go
	case "i":
		return p, p.importCmd()
```

In `Update`, add a case (before `tea.KeyMsg`):

```go
	case ajutsImportedMsg:
		if msg.year != p.year {
			return p, nil
		}
		if msg.err != nil {
			p.err = msg.err
			return p, nil
		}
		p.status = msg.result
		p.err = nil
		return p, p.loadCmd()
```

In `Detail()`, prepend the status line when present — change the first lines of `Detail()` to:

```go
func (p ajutsPanel) Detail() string {
	prefix := ""
	if p.status != "" {
		prefix = p.status + "\n"
	}
	if p.err != nil {
		return prefix + errDetail(p.err)
	}
	// ... existing body, but return prefix + <existing return value>
```

Concretely, wrap the existing returns so each returns `prefix + <value>`. For example the concessions branch becomes:

```go
		return prefix + fmt.Sprintf("Concessió %s · %s · subtipus %s · Previsions: %s",
			c.GroupCode(), c.Concept(), c.SubtypeCode(), p.forecastIDsFor(c.GroupCode()))
```

and likewise for the empty-selection returns (`return prefix`) and the invoice branch.

In `Actions()`, add the import action:

```go
func (p ajutsPanel) Actions() []Action {
	return []Action{
		{Key: "tab", Label: "canvia vista"},
		{Key: "i", Label: "importa JSON"},
	}
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/adapters/tui/ -run TestAjutsPanel -v && go build ./...`
Expected: PASS + clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/tui/panel_ajuts.go internal/adapters/tui/panel_ajuts_test.go
git commit -m "feat(tui): Ajuts panel JSON import (i)"
```

---

### Task 11: "Ajuts" panel — Concessions CRUD

**Files:**
- Modify: `internal/adapters/tui/panel_ajuts.go`
- Create: `internal/adapters/tui/panel_ajuts_concession.go` (form builders + parse helpers)
- Test: `internal/adapters/tui/panel_ajuts_concession_test.go`

**Interfaces:**
- Consumes: `formModal`/`formFieldDef`/`newFormModal`/`openModalCmd`/`newConfirmModal`; `Deps.Reconciliation.SaveConcession`/`DeleteConcession` (Task 6).
- Produces: `n`/`e`/`d` keys on the Concessions view; helpers `parseForecastIDs(string) []string`, `parseMoney(string) (model.Money, error)`.

- [ ] **Step 1: Write the failing test (parse helper)**

Create `internal/adapters/tui/panel_ajuts_concession_test.go`:

```go
package tui

import "testing"

func TestParseForecastIDs(t *testing.T) {
	got := parseForecastIDs(" CP25008 , CP25009 ,, ")
	if len(got) != 2 || got[0] != "CP25008" || got[1] != "CP25009" {
		t.Fatalf("parseForecastIDs = %v", got)
	}
	if len(parseForecastIDs("")) != 0 {
		t.Error("empty string should yield no ids")
	}
}

func TestParseMoney(t *testing.T) {
	m, err := parseMoney(" 13880.00 ")
	if err != nil || m.String() != "13880.00" {
		t.Fatalf("parseMoney = (%s, %v)", m, err)
	}
	if _, err := parseMoney("abc"); err == nil {
		t.Error("expected error for non-numeric")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/adapters/tui/ -run 'ParseForecastIDs|ParseMoney' -v`
Expected: FAIL — undefined `parseForecastIDs`.

- [ ] **Step 3: Implement parse helpers + form builders**

Create `internal/adapters/tui/panel_ajuts_concession.go`:

```go
package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

// parseForecastIDs splits a comma-separated CP-id list, trimming blanks.
func parseForecastIDs(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func parseMoney(s string) (model.Money, error) {
	return model.MoneyFromString(strings.TrimSpace(s))
}

// concessionForm builds the create/edit form. existing == nil means create
// (Grup editable); otherwise edit (Grup fixed).
func (p ajutsPanel) concessionForm(existing *model.Concession) formModal {
	title := "Nova concessió"
	group, subtype, concept, demanat, concedit, forecasts := "", "", "", "0.00", "0.00", ""
	if existing != nil {
		title = "Edita concessió"
		group = existing.GroupCode()
		subtype = existing.SubtypeCode()
		concept = existing.Concept()
		demanat = existing.RequestedTotal().String()
		concedit = existing.GrantedAmount().String()
		forecasts = p.forecastIDsFor(existing.GroupCode())
	}

	var fields []formFieldDef
	if existing == nil {
		fields = append(fields, formFieldDef{Label: "Grup", Placeholder: "A6-02", Value: group})
	}
	fields = append(fields,
		formFieldDef{Label: "Subtipus", Placeholder: "a6", Value: subtype},
		formFieldDef{Label: "Concepte", Placeholder: "Adob orgànic", Value: concept},
		formFieldDef{Label: "Demanat", Placeholder: "0.00", Value: demanat},
		formFieldDef{Label: "Concedit", Placeholder: "0.00", Value: concedit},
		formFieldDef{Label: "Previsions", Placeholder: "CP25008,CP25009", Value: forecasts},
	)

	year := p.year
	onSubmit := func(values map[string]string) tea.Cmd {
		req, err := parseMoney(values["Demanat"])
		if err != nil {
			return nil
		}
		granted, err := parseMoney(values["Concedit"])
		if err != nil {
			return nil
		}
		gc := group
		if existing == nil {
			gc = strings.TrimSpace(values["Grup"])
		}
		in := application.ConcessionInput{
			Year: year, GroupCode: gc,
			SubtypeCode: strings.TrimSpace(values["Subtipus"]),
			Concept:     values["Concepte"],
			RequestedTotal: req, GrantedAmount: granted,
			ForecastIDs: parseForecastIDs(values["Previsions"]),
		}
		return p.reloadCmd(func(ctx context.Context) error {
			return p.deps.Reconciliation.SaveConcession(ctx, in)
		})
	}
	return newFormModal(title, fields, onSubmit)
}

// handleConcessionKey handles n/e/d while the Concessions view is active.
func (p ajutsPanel) handleConcessionKey(key string) (Panel, tea.Cmd) {
	switch key {
	case "n":
		return p, openModalCmd(p.concessionForm(nil))
	case "e":
		if p.selected < 0 || p.selected >= len(p.concessions) {
			return p, nil
		}
		c := p.concessions[p.selected]
		return p, openModalCmd(p.concessionForm(&c))
	case "d":
		if p.selected < 0 || p.selected >= len(p.concessions) {
			return p, nil
		}
		c := p.concessions[p.selected]
		gc := c.GroupCode()
		onConfirm := p.reloadCmd(func(ctx context.Context) error {
			return p.deps.Reconciliation.DeleteConcession(ctx, p.year, gc)
		})
		return p, openModalCmd(newConfirmModal(fmt.Sprintf("Eliminar la concessió %s?", gc), onConfirm))
	}
	return p, nil
}
```

- [ ] **Step 4: Route keys + advertise actions**

In `internal/adapters/tui/panel_ajuts.go` `handleKey`, before the final `return p, nil`, add:

```go
	if p.view == ajutsConcessions && p.deps.Reconciliation != nil {
		switch msg.String() {
		case "n", "e", "d":
			return p.handleConcessionKey(msg.String())
		}
	}
```

In `Actions()`, when `p.view == ajutsConcessions`, include the CRUD keys. Replace `Actions()` with:

```go
func (p ajutsPanel) Actions() []Action {
	actions := []Action{
		{Key: "tab", Label: "canvia vista"},
		{Key: "i", Label: "importa JSON"},
	}
	if p.view == ajutsConcessions {
		actions = append(actions,
			Action{Key: "n", Label: "nova"},
			Action{Key: "e", Label: "edita"},
			Action{Key: "d", Label: "elimina"})
	}
	return actions
}
```

- [ ] **Step 5: Run tests + build**

Run: `go test ./internal/adapters/tui/ -run 'ParseForecastIDs|ParseMoney|TestAjutsPanel' -v && go build ./...`
Expected: PASS + clean build.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/tui/panel_ajuts.go internal/adapters/tui/panel_ajuts_concession.go internal/adapters/tui/panel_ajuts_concession_test.go
git commit -m "feat(tui): Ajuts concessions CRUD"
```

---

### Task 12: "Ajuts" panel — Factures CRUD (invoice + payments + links)

**Files:**
- Modify: `internal/adapters/tui/panel_ajuts.go`
- Create: `internal/adapters/tui/panel_ajuts_invoice.go` (form + parse helpers)
- Test: `internal/adapters/tui/panel_ajuts_invoice_test.go`

**Interfaces:**
- Consumes: `Deps.Reconciliation.SaveInvoice`/`DeleteInvoice` (Task 6); form helpers.
- Produces: `n`/`e`/`d` on the Factures view; helpers `parsePayments(string) ([]application.PaymentInput, error)` (format `YYYY-MM-DD:amount;…`) and `parseLinks(string) ([]application.LinkInput, error)` (format `CPid:amount;…`), and their inverse formatters `formatPayments`/`formatLinks`.

- [ ] **Step 1: Write the failing test**

Create `internal/adapters/tui/panel_ajuts_invoice_test.go`:

```go
package tui

import (
	"testing"

	"github.com/pjover/espigol/internal/domain/model"
)

func TestParsePayments(t *testing.T) {
	pays, err := parsePayments("2025-04-01:1234.56; 2025-05-01:200.00 ")
	if err != nil || len(pays) != 2 {
		t.Fatalf("parsePayments = (%v, %v)", pays, err)
	}
	if pays[0].Amount.Cmp(model.MoneyOf(0).Plus(mustMoneyT(t, "1234.56"))) != 0 {
		t.Errorf("amount[0] = %s", pays[0].Amount)
	}
	if pays[0].PaidOn.Year() != 2025 || pays[0].PaidOn.Month() != 4 {
		t.Errorf("date[0] = %v", pays[0].PaidOn)
	}
	if _, err := parsePayments("bad"); err == nil {
		t.Error("expected error for malformed payment")
	}
}

func TestParseLinks(t *testing.T) {
	links, err := parseLinks("CP25030:500.00;CP25032:734.56")
	if err != nil || len(links) != 2 || links[1].ForecastID != "CP25032" {
		t.Fatalf("parseLinks = (%v, %v)", links, err)
	}
}

func mustMoneyT(t *testing.T, s string) model.Money {
	t.Helper()
	m, err := model.MoneyFromString(s)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/adapters/tui/ -run 'ParsePayments|ParseLinks' -v`
Expected: FAIL — undefined `parsePayments`.

- [ ] **Step 3: Implement parse/format helpers + invoice form**

Create `internal/adapters/tui/panel_ajuts_invoice.go`:

```go
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

const ajutsDateLayout = "2006-01-02"

// parsePayments parses "YYYY-MM-DD:amount;YYYY-MM-DD:amount".
func parsePayments(s string) ([]application.PaymentInput, error) {
	var out []application.PaymentInput
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		date, amtStr, ok := strings.Cut(part, ":")
		if !ok {
			return nil, fmt.Errorf("pagament %q: format esperat DATA:IMPORT", part)
		}
		d, err := time.Parse(ajutsDateLayout, strings.TrimSpace(date))
		if err != nil {
			return nil, fmt.Errorf("pagament %q: data invàlida: %w", part, err)
		}
		amt, err := model.MoneyFromString(strings.TrimSpace(amtStr))
		if err != nil {
			return nil, fmt.Errorf("pagament %q: import invàlid: %w", part, err)
		}
		out = append(out, application.PaymentInput{PaidOn: d, Amount: amt})
	}
	return out, nil
}

// parseLinks parses "CPid:amount;CPid:amount".
func parseLinks(s string) ([]application.LinkInput, error) {
	var out []application.LinkInput
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, amtStr, ok := strings.Cut(part, ":")
		if !ok {
			return nil, fmt.Errorf("enllaç %q: format esperat CPID:IMPORT", part)
		}
		amt, err := model.MoneyFromString(strings.TrimSpace(amtStr))
		if err != nil {
			return nil, fmt.Errorf("enllaç %q: import invàlid: %w", part, err)
		}
		out = append(out, application.LinkInput{ForecastID: strings.TrimSpace(id), Amount: amt})
	}
	return out, nil
}

func formatPayments(pays []model.InvoicePayment) string {
	var parts []string
	for _, p := range pays {
		parts = append(parts, fmt.Sprintf("%s:%s", p.PaidOn().Format(ajutsDateLayout), p.Amount()))
	}
	return strings.Join(parts, ";")
}

func formatLinks(links []model.ForecastInvoice) string {
	var parts []string
	for _, l := range links {
		parts = append(parts, fmt.Sprintf("%s:%s", l.ForecastID(), l.Amount()))
	}
	return strings.Join(parts, ";")
}

func (p ajutsPanel) invoiceForm(existing *model.Invoice) formModal {
	title := "Nova factura"
	id := 0
	issuer, nif, number, date, net, file, notes, pays, links := "", "", "", "", "0.00", "", "", "", ""
	if existing != nil {
		title = "Edita factura"
		id = existing.ID()
		issuer = existing.Issuer()
		nif = existing.Nif()
		number = existing.Number()
		date = existing.IssueDate().Format(ajutsDateLayout)
		net = existing.NetAmount().String()
		if existing.FilePath() != nil {
			file = *existing.FilePath()
		}
		if existing.Notes() != nil {
			notes = *existing.Notes()
		}
		pays = formatPayments(existing.Payments())
		links = formatLinks(existing.Links())
	}

	fields := []formFieldDef{
		{Label: "Proveïdor", Placeholder: "Ribot", Value: issuer},
		{Label: "NIF", Placeholder: "B12345678", Value: nif},
		{Label: "Núm", Placeholder: "FD-39521", Value: number},
		{Label: "Data", Placeholder: "2025-03-14", Value: date},
		{Label: "Import", Placeholder: "0.00", Value: net},
		{Label: "Arxiu", Placeholder: "factura.pdf", Value: file},
		{Label: "Notes", Placeholder: "", Value: notes},
		{Label: "Pagaments", Placeholder: "2025-04-01:500.00", Value: pays},
		{Label: "Enllaços", Placeholder: "CP25030:500.00", Value: links},
	}

	year := p.year
	onSubmit := func(values map[string]string) tea.Cmd {
		net, err := parseMoney(values["Import"])
		if err != nil {
			return nil
		}
		issued, err := time.Parse(ajutsDateLayout, strings.TrimSpace(values["Data"]))
		if err != nil {
			return nil
		}
		pays, err := parsePayments(values["Pagaments"])
		if err != nil {
			return nil
		}
		links, err := parseLinks(values["Enllaços"])
		if err != nil {
			return nil
		}
		in := application.InvoiceInput{
			ID: id, Year: year,
			Issuer: values["Proveïdor"], Nif: strings.TrimSpace(values["NIF"]),
			Number: strings.TrimSpace(values["Núm"]), IssueDate: issued, NetAmount: net,
			FilePath: strings.TrimSpace(values["Arxiu"]), Notes: values["Notes"],
			Payments: pays, Links: links,
		}
		return p.reloadCmd(func(ctx context.Context) error {
			_, err := p.deps.Reconciliation.SaveInvoice(ctx, in)
			return err
		})
	}
	return newFormModal(title, fields, onSubmit)
}

func (p ajutsPanel) handleInvoiceKey(key string) (Panel, tea.Cmd) {
	switch key {
	case "n":
		return p, openModalCmd(p.invoiceForm(nil))
	case "e":
		if p.selected < 0 || p.selected >= len(p.invoices) {
			return p, nil
		}
		inv := p.invoices[p.selected]
		return p, openModalCmd(p.invoiceForm(&inv))
	case "d":
		if p.selected < 0 || p.selected >= len(p.invoices) {
			return p, nil
		}
		inv := p.invoices[p.selected]
		id, num := inv.ID(), inv.Number()
		onConfirm := p.reloadCmd(func(ctx context.Context) error {
			return p.deps.Reconciliation.DeleteInvoice(ctx, id)
		})
		return p, openModalCmd(newConfirmModal(fmt.Sprintf("Eliminar la factura %s?", num), onConfirm))
	}
	return p, nil
}
```

- [ ] **Step 4: Route keys + advertise actions**

In `internal/adapters/tui/panel_ajuts.go` `handleKey`, extend the CRUD-routing block added in Task 11 to also cover the invoices view:

```go
	if p.deps.Reconciliation != nil {
		switch msg.String() {
		case "n", "e", "d":
			if p.view == ajutsConcessions {
				return p.handleConcessionKey(msg.String())
			}
			return p.handleInvoiceKey(msg.String())
		}
	}
```

(Replace the concessions-only block from Task 11 with this combined block.)

In `Actions()`, extend the CRUD actions to both views — change the `if p.view == ajutsConcessions` guard so the `n/e/d` actions are appended for **either** view (the labels are identical):

```go
	actions = append(actions,
		Action{Key: "n", Label: "nova"},
		Action{Key: "e", Label: "edita"},
		Action{Key: "d", Label: "elimina"})
```

(Remove the `if p.view == ajutsConcessions` wrapper so it applies to both.)

- [ ] **Step 5: Run tests + build + full suite**

Run: `go test ./internal/adapters/tui/ -run 'ParsePayments|ParseLinks|TestAjutsPanel' -v && go build ./... && make vet && go test ./...`
Expected: all PASS, clean vet.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/tui/panel_ajuts.go internal/adapters/tui/panel_ajuts_invoice.go internal/adapters/tui/panel_ajuts_invoice_test.go
git commit -m "feat(tui): Ajuts factures CRUD (invoice + payments + links)"
```

---

### Task 13: End-to-end 2025 fixture import test

**Files:**
- Create: `internal/application/reconciliation_fixture_test.go`
- Create (test fixture): `internal/application/testdata/reconciliation-2025-sample.json`

**Interfaces:**
- Consumes: `importer.LoadReconciliation` + `ReconciliationService.AdminImport`; the `newReconWorld` helper (extended to seed the specific forecasts the fixture references).

**Note:** This uses a small representative fixture (not the full 59-invoice private data, which lives under `private/`). It locks in the appendix bundle behavior (`A6-02 = CP25008 + CP25009`) and the one-invoice-many-forecasts case. The full private-data validation can be run manually once real data is imported.

- [ ] **Step 1: Create the fixture**

Create `internal/application/testdata/reconciliation-2025-sample.json`:

```json
{
  "year": 2025,
  "concessions": [
    {"groupCode":"A6-02","subtypeCode":"a6","concept":"Adob orgànic",
     "requestedTotal":"13880.00","grantedAmount":"13880.00","forecastIds":["CP25008","CP25009"]}
  ],
  "invoices": [
    {"issuer":"Jardines Campaner","nif":"B12345678","number":"F878","issueDate":"2025-03-14",
     "netAmount":"1234.56","filePath":"f878.pdf","notes":"Varies màquines",
     "payments":[{"paidOn":"2025-04-01","amount":"1234.56"}],
     "links":[{"forecastId":"CP25008","amount":"500.00"},{"forecastId":"CP25009","amount":"734.56"}]}
  ]
}
```

- [ ] **Step 2: Write the test**

Create `internal/application/reconciliation_fixture_test.go`:

```go
package application_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pjover/espigol/internal/adapters/importer"
	"github.com/pjover/espigol/internal/application"
)

func TestReconciliation2025Fixture_ImportsAndBundles(t *testing.T) {
	world := newReconWorldWithForecasts(t, "CP25008", "CP25009")
	svc := application.NewReconciliationService(world.tx)
	ctx := context.Background()

	in, err := importer.LoadReconciliation(filepath.Join("testdata", "reconciliation-2025-sample.json"), 2025)
	if err != nil {
		t.Fatalf("LoadReconciliation: %v", err)
	}
	res, err := svc.AdminImport(ctx, in)
	if err != nil {
		t.Fatalf("AdminImport: %v", err)
	}
	if res.Concessions != 1 || res.Invoices != 1 {
		t.Fatalf("counts: %+v", res)
	}

	links, _ := svc.ListConcessionLinks(ctx, 2025)
	if len(links) != 2 {
		t.Fatalf("A6-02 bundle should have 2 forecasts, got %d", len(links))
	}
	invs, _ := svc.ListInvoices(ctx, 2025)
	if len(invs) != 1 || len(invs[0].Links()) != 2 {
		t.Fatalf("F878 should link 2 forecasts, got %+v", invs)
	}
}
```

- [ ] **Step 3: Add `newReconWorldWithForecasts` helper**

In `internal/application/reconciliation_service_test.go`, generalize the seed. Add:

```go
// newReconWorldWithForecasts seeds a 2025 window + subtype a6 + partner and
// creates forecasts until the given ids exist (CP250nn are allocated in order).
func newReconWorldWithForecasts(t *testing.T, ids ...string) reconWorld {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	q := sqlc.New(conn)
	ctx := context.Background()

	win := persistence.NewWindowRepository(q)
	tax := persistence.NewTaxonomyRepository(q)
	pr := persistence.NewPartnerRepository(q)
	fr := persistence.NewForecastRepository(conn, q)

	w, _ := model.NewSubmissionWindow(2025, model.WindowClosed, nil, nil,
		time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	_ = win.Save(ctx, w)
	typ, _ := model.NewExpenseType(2025, "A", "[a]", model.CategoryCurrent)
	_ = tax.SaveType(ctx, typ)
	st, _ := model.NewExpenseSubtype(2025, "a6", "[a6]", "A")
	_ = tax.SaveSubtype(ctx, st)
	planned := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	p7, _ := model.NewPartner(7, "X", "Y", "V", "x@e.cat", "6", model.Productor, 1, planned, false)
	_ = pr.Save(ctx, p7)

	// Allocate forecasts CP25001.. until all requested ids are present.
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	have := map[string]bool{}
	for len(have) < len(want) {
		uf, _ := model.NewUnsavedExpenseForecast(p7, "f", "d", model.MoneyOf(6940), model.ZeroMoney(),
			nil, planned, 2025, "a6", model.NewCommonScope(), planned, true)
		f, err := fr.Create(ctx, uf)
		if err != nil {
			t.Fatal(err)
		}
		if want[f.ID()] {
			have[f.ID()] = true
		}
		if f.ID() > "CP25099" { // safety stop
			t.Fatalf("could not allocate ids %v (got up to %s)", ids, f.ID())
		}
	}
	return reconWorld{tx: persistence.NewTxManager(conn), forecastID: ids[0]}
}
```

(Each fixture forecast has GrossAmount 6940, so `Σ Previst = 13880` matches `Demanat 13880.00` — no soft warning from this fixture.)

- [ ] **Step 4: Run the test**

Run: `go test ./internal/application/ -run TestReconciliation2025Fixture -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/application/reconciliation_fixture_test.go internal/application/testdata/reconciliation-2025-sample.json internal/application/reconciliation_service_test.go
git commit -m "test(application): 2025 reconciliation fixture import + bundle check"
```

---

### Task 14: Docs — README reconciliation section

**Files:**
- Modify: `README.md` (admin-panel keys + reconciliation JSON example)

**Interfaces:** none (documentation).

- [ ] **Step 1: Document the Ajuts panel + JSON format**

In `README.md`, add a subsection under the admin-panel documentation describing: the **Ajuts** panel (`tab` toggles Concessions/Factures; `i` imports `import/reconciliation-<year>.json`; `n`/`e`/`d` create/edit/delete), the delimited-text conventions for the TUI forms (forecast-ID lists `CP25008,CP25009`; payments `2025-04-01:500.00;…`; links `CP25030:500.00;…`), and the JSON import shape (copy the example block from `docs/superpowers/specs/2026-07-08-subsidy-reconciliation-phase1-design.md` §4). Note that reconciliation data has **no window-state gate** and that the algorithm/report (Phases 2–3) are not yet implemented.

- [ ] **Step 2: Verify + commit**

Run: `go build ./...` (sanity) and eyeball the README render.

```bash
git add README.md
git commit -m "docs(readme): Ajuts reconciliation panel + JSON import"
```

---

## Self-Review

**Spec coverage** (against `2026-07-08-subsidy-reconciliation-phase1-design.md`):
- §1 schema (5 tables) → Task 1. ✅
- §2 domain + ports → Tasks 2–5. ✅
- §3 persistence (queries/mappers/repos/wiring) → Tasks 1, 4, 5. ✅
- §4 JSON import (replace-all, format, no state gate) → Tasks 6 (service), 7 (parser), 10 (trigger). ✅
- §5 TUI "Ajuts" panel (Concessions + Factures views + import) → Tasks 9–12. ✅
- §6 validation (hard referential; soft warnings incl. 0,01 € tolerance) → Task 6 (`validateReferences`). ✅
- §7 testing (repo round-trip, importer, 2025 fixture) → Tasks 4, 5, 7, 13. ✅
- §8 out-of-scope (algorithm, gating logic, report, SplitKey) → not implemented; noted in Task 14 docs. ✅

**Placeholder scan:** no TBD/TODO; all code steps show full code. The one prose-only step is Task 14 (documentation), which is inherently descriptive. ✅

**Type consistency:** `ReconciliationService` methods, `ConcessionInput`/`InvoiceInput`/`PaymentInput`/`LinkInput`, and `ReplaceForYear`/`ReplaceMembership`/`Save` signatures match across Tasks 4–12. Panel helpers `parseForecastIDs`/`parseMoney`/`parsePayments`/`parseLinks`/`formatPayments`/`formatLinks` are defined once and reused. `model.Invoice` aggregate getters used by mappers/panels match Task 3. ✅

**Note for the implementer:** run `make sqlc-generate` (Task 1) before Task 4 — the repositories reference generated `sqlc.*Params` types. If sqlc names a generated field differently than assumed (e.g. `Number` vs `Number_`), adjust the mapper field names to match the generated struct; the generated code is the source of truth.
