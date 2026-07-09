# Subsidy Reconciliation — Phase 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the reconciliation *algorithm* — a pure `Compute(ReconciliationInput) → ReconciliationData` function in `internal/domain/services/reconciliation.go` plus a thin `ReconciliationService.Compute(ctx, year)` orchestrator in the application layer — so Phase 1's imported grants + invoices produce a per-forecast `Subvenció assignada` snapshot ready for Phase 3 to render and persist.

**Architecture:** Mirrors the existing `services.Compute(AllocationInput) → report.ReportData` seam used by `window_service.go`. Pure math + JSON-serialisable data structs in `internal/domain/services/`; orchestration (read forecasts + concessions + invoices + taxonomy + partners inside one `TxManager.WithinTx`) in `internal/application/reconciliation_service.go`. No persistence, no TUI, no report rendering — those are Phase 3.

**Tech Stack:** Go, `shopspring/decimal` via `model.Money` (scale 2, HALF_UP; `TimesRatio(decimal.Decimal)` with a largest-remainder pass for cent-closing).

## Global Constraints

- **Program in English.** Type names, method names, enum values, field names — English only. Catalan display strings are Phase 3's concern.
- **All monetary values are `model.Money`.** Never `float64`. `TimesRatio` takes `decimal.Decimal`, not float.
- **No writes, no migrations, no new ports, no new sqlc, no new repos, no TUI changes.** Phase 2 is a read-only computation library. Every existing port used is already in `ports.RepoSet`.
- **Domain services stay pure.** `internal/domain/services/reconciliation.go` imports only `model` and `time` — no ports, no context.Context, no I/O. The application service is the only place that fetches through ports.
- **Filter forecasts by `Enabled == true`.** Disabled forecasts are skipped entirely (no `Executed`, no output row).
- **Payment gating is all-or-nothing.** Invoice contributes to `Executed` iff `Σ payments ≥ netAmount − 0.01`.
- **Largest-remainder cent-close** so `Σ Assigned_i == Base_g` exactly, no rounding leak.
- **`NetDeviation` is reporting only.** It never changes any per-forecast `Assigned`.
- **`ExpenseCategory` values:** `model.CategoryCurrent` and `model.CategoryInvestment` (string enums).
- **Follow existing patterns:** immutable domain structs, pure algorithms in `domain/services`, orchestration in `application`, `TxManager.WithinTx` for consistency. Run `make vet` and `go test ./...` after each task before committing. One commit per task.

---

### Task 1: Types + skeleton `Compute` (empty-input case)

**Files:**
- Create: `internal/domain/services/reconciliation.go`
- Create: `internal/domain/services/reconciliation_test.go`

**Interfaces:**
- Produces: exported types `ReconciliationInput`, `ReconciliationData`, `CategoryReconciliation`, `SubtypeReconciliation`, `ConcessionReconciliation`, `ForecastReconciliation`, `InvoiceContribution`, `ForecastReconStatus` (int enum with 5 values), and the entry point `Compute(in ReconciliationInput) (ReconciliationData, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/domain/services/reconciliation_test.go`:

```go
package services

import (
	"testing"

	"github.com/pjover/espigol/internal/domain/model"
)

func TestReconciliation_EmptyInput_ReturnsEmptyData(t *testing.T) {
	in := ReconciliationInput{Year: 2025}
	got, err := ComputeReconciliation(in)
	if err != nil {
		t.Fatalf("ComputeReconciliation: %v", err)
	}
	if got.Year != 2025 {
		t.Errorf("Year = %d, want 2025", got.Year)
	}
	if len(got.Categories) != 0 {
		t.Errorf("Categories = %d, want 0", len(got.Categories))
	}
	// Enum values must be declared
	_ = StatusFullyJustified
	_ = StatusPartiallyJustified
	_ = StatusOverExecuted
	_ = StatusPaymentPending
	_ = StatusNoInvoice
	_ = model.ZeroMoney() // used later
}
```

*Note:* the entry point is named `ComputeReconciliation` (not `Compute`) so it doesn't collide with `services.Compute` (the allocation entry point) — both live in the same package.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/domain/services/... -run TestReconciliation_EmptyInput -v`
Expected: FAIL — `undefined: ReconciliationInput`, `undefined: ComputeReconciliation`, etc.

- [ ] **Step 3: Implement types + skeleton**

Create `internal/domain/services/reconciliation.go`:

```go
// Package services — reconciliation.go is the Phase 2 pure algorithm that
// turns the year's Concession + Invoice data into a per-forecast
// AssignedSubsidy snapshot. It has zero I/O; orchestration lives in
// internal/application/reconciliation_service.go.
package services

import (
	"time"

	"github.com/pjover/espigol/internal/domain/model"
)

// ReconciliationInput is everything ComputeReconciliation needs to compute
// per-forecast subsidies for a single year. The application service builds
// this from ports.RepoSet reads inside a TxManager.WithinTx.
type ReconciliationInput struct {
	Year        int
	Forecasts   []model.ExpenseForecast // ALL year's forecasts; algorithm filters Enabled==true
	Concessions []model.Concession
	Links       []model.ConcessionForecast // membership (year, groupCode, forecastID)
	Invoices    []model.Invoice            // aggregate: payments + links included
	Subtypes    []model.ExpenseSubtype     // year-scoped
	Types       []model.ExpenseType        // year-scoped (subtype→type→category lookup)
	Partners    []model.Partner
}

// ReconciliationData is the JSON-serialisable snapshot produced by
// ComputeReconciliation. Categories are ordered CURRENT then INVESTMENT.
// Empty categories/subtypes/concessions are omitted.
type ReconciliationData struct {
	Year       int                      `json:"year"`
	Categories []CategoryReconciliation `json:"categories"`
}

type CategoryReconciliation struct {
	Category     model.ExpenseCategory   `json:"category"`
	Requested    model.Money             `json:"requested"`
	Granted      model.Money             `json:"granted"`
	Executed     model.Money             `json:"executed"`
	Assigned     model.Money             `json:"assigned"`
	NetDeviation model.Money             `json:"netDeviation"` // Σ Subtype.Deviation
	Subtypes     []SubtypeReconciliation `json:"subtypes"`
}

type SubtypeReconciliation struct {
	Code        string                     `json:"code"`
	Label       string                     `json:"label"`
	Requested   model.Money                `json:"requested"`
	Granted     model.Money                `json:"granted"`
	Executed    model.Money                `json:"executed"`
	Assigned    model.Money                `json:"assigned"`
	Deviation   model.Money                `json:"deviation"` // Granted − Executed (raw)
	Concessions []ConcessionReconciliation `json:"concessions"`
}

type ConcessionReconciliation struct {
	GroupCode  string                    `json:"groupCode"`
	Concept    string                    `json:"concept"`
	Requested  model.Money               `json:"requested"`
	Granted    model.Money               `json:"granted"`
	Executed   model.Money               `json:"executed"`
	Assigned   model.Money               `json:"assigned"`
	Difference model.Money               `json:"difference"` // Granted − Executed
	Forecasts  []ForecastReconciliation  `json:"forecasts"`
}

type ForecastReconciliation struct {
	ForecastID     string                `json:"forecastId"`
	PartnerID      int                   `json:"partnerId"`
	Concept        string                `json:"concept"`
	GrossAmount    model.Money           `json:"grossAmount"`
	ApprovedAmount model.Money           `json:"approvedAmount"`
	Executed       model.Money           `json:"executed"`
	Pending        model.Money           `json:"pending"`
	Assigned       model.Money           `json:"assigned"`
	Status         ForecastReconStatus   `json:"status"`
	Invoices       []InvoiceContribution `json:"invoices"`
}

type InvoiceContribution struct {
	InvoiceID    int         `json:"invoiceId"`
	Issuer       string      `json:"issuer"`
	Number       string      `json:"number"`
	IssueDate    time.Time   `json:"issueDate"`
	LinkedAmount model.Money `json:"linkedAmount"`
	FullyPaid    bool        `json:"fullyPaid"`
	PaidOn       *time.Time  `json:"paidOn,omitempty"`
}

// ForecastReconStatus flags each forecast's reconciliation state. Precedence
// (first-match wins as applied by the algorithm): NoInvoice, PaymentPending,
// OverExecuted, PartiallyJustified, FullyJustified.
type ForecastReconStatus int

const (
	StatusFullyJustified ForecastReconStatus = iota
	StatusPartiallyJustified
	StatusOverExecuted
	StatusPaymentPending
	StatusNoInvoice
)

// ComputeReconciliation is the pure entry point. Given the year's forecasts,
// concessions, invoices, taxonomy, and partners, it returns the snapshot tree
// described by the Phase 2 spec. Skeleton in Task 1; filled in Tasks 2-5.
func ComputeReconciliation(in ReconciliationInput) (ReconciliationData, error) {
	return ReconciliationData{Year: in.Year}, nil
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/domain/services/... -run TestReconciliation_EmptyInput -v`
Expected: PASS.

Then: `go vet ./... && go build ./...` — must both succeed.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/services/reconciliation.go internal/domain/services/reconciliation_test.go
git commit -m "feat(services): reconciliation Compute skeleton + return types"
```

---

### Task 2: `Executed` and `Pending` per forecast (payment gating)

**Files:**
- Modify: `internal/domain/services/reconciliation.go`
- Test: `internal/domain/services/reconciliation_test.go`

**Interfaces:**
- Consumes: types from Task 1 (`ReconciliationInput`, `ForecastReconciliation` fields `Executed`, `Pending`, `Invoices`).
- Produces (internal to this file): a helper `executedAndPending(in ReconciliationInput) map[string]forecastExec` where `forecastExec` bundles `{Executed, Pending model.Money; Invoices []InvoiceContribution}` — used by subsequent tasks.

- [ ] **Step 1: Write the failing tests**

Append to `internal/domain/services/reconciliation_test.go`:

```go
import (
	"time"
	// keep existing imports
)

// mkForecast is a compact ExpenseForecast constructor for these tests.
func mkForecast(t *testing.T, id string, partnerID int, subtypeCode string, gross string) model.ExpenseForecast {
	t.Helper()
	planned := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	p, err := model.NewPartner(partnerID, "X", "Y", "V", "x"+id+"@e.cat", "6", model.Productor, 1, planned, false)
	if err != nil {
		t.Fatal(err)
	}
	grossMoney, err := model.MoneyFromString(gross)
	if err != nil {
		t.Fatal(err)
	}
	f, err := model.NewSavedExpenseForecast(id, p, "concept "+id, "", grossMoney, model.ZeroMoney(),
		nil, planned, 2025, subtypeCode, model.NewCommonScope(), planned, true)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// mkInvoice builds an Invoice with one payment and one link. paidTotal is
// how much has been paid (may be less than net → unpaid).
func mkInvoice(t *testing.T, id int, year int, net string, paidTotal string, forecastID string, linkAmount string) model.Invoice {
	t.Helper()
	netM, _ := model.MoneyFromString(net)
	paidM, _ := model.MoneyFromString(paidTotal)
	linkM, _ := model.MoneyFromString(linkAmount)
	issued := time.Date(year, 3, 1, 0, 0, 0, 0, time.UTC)
	paidOn := time.Date(year, 4, 1, 0, 0, 0, 0, time.UTC)

	var payments []model.InvoicePayment
	if !paidM.IsZero() {
		p, err := model.NewInvoicePayment(id, paidOn, paidM)
		if err != nil {
			t.Fatal(err)
		}
		payments = []model.InvoicePayment{p}
	}
	link, err := model.NewForecastInvoice(year, forecastID, id, linkM)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := model.NewInvoice(id, year, "Sup", "B1", "F"+forecastID, issued, netM, "", "", payments, []model.ForecastInvoice{link})
	if err != nil {
		t.Fatal(err)
	}
	return inv
}

func TestReconciliation_PaymentGate_UnpaidExcludedFromExecuted(t *testing.T) {
	f := mkForecast(t, "CP25001", 7, "a2", "100.00")
	unpaid := mkInvoice(t, 1, 2025, "60.00", "0.00", "CP25001", "60.00")
	partiallyPaid := mkInvoice(t, 2, 2025, "40.00", "20.00", "CP25001", "40.00")
	fullyPaid := mkInvoice(t, 3, 2025, "50.00", "50.00", "CP25001", "50.00")

	in := ReconciliationInput{
		Year:      2025,
		Forecasts: []model.ExpenseForecast{f},
		Invoices:  []model.Invoice{unpaid, partiallyPaid, fullyPaid},
	}
	m := executedAndPending(in)

	got := m["CP25001"]
	wantExec, _ := model.MoneyFromString("50.00")
	wantPend, _ := model.MoneyFromString("100.00")
	if got.Executed.Cmp(wantExec) != 0 {
		t.Errorf("Executed = %s, want 50.00", got.Executed.String())
	}
	if got.Pending.Cmp(wantPend) != 0 {
		t.Errorf("Pending = %s, want 100.00", got.Pending.String())
	}
	if len(got.Invoices) != 3 {
		t.Errorf("Invoices = %d, want 3", len(got.Invoices))
	}
	// Ordered by IssueDate then Number — all same date here, ensure Number order F<id>
	if got.Invoices[0].Number != "FCP25001" || !got.Invoices[2].FullyPaid {
		t.Errorf("invoices not in expected order/state: %+v", got.Invoices)
	}
}

func TestReconciliation_PaymentGate_ForecastWithNoLinks_ExecutedZero(t *testing.T) {
	f := mkForecast(t, "CP25002", 7, "a2", "50.00")
	in := ReconciliationInput{
		Year:      2025,
		Forecasts: []model.ExpenseForecast{f},
	}
	m := executedAndPending(in)
	got := m["CP25002"]
	if !got.Executed.IsZero() || !got.Pending.IsZero() {
		t.Errorf("empty forecast: executed=%s pending=%s", got.Executed, got.Pending)
	}
	if len(got.Invoices) != 0 {
		t.Errorf("Invoices should be empty, got %d", len(got.Invoices))
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/domain/services/... -run TestReconciliation_PaymentGate -v`
Expected: FAIL — `undefined: executedAndPending`.

- [ ] **Step 3: Implement the helper**

Append to `internal/domain/services/reconciliation.go`:

```go
import (
	"sort"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
)

// forecastExec bundles the per-forecast paid/pending totals with the list of
// invoice contributions (paid AND unpaid). It's the shared intermediate the
// downstream stages of ComputeReconciliation consume.
type forecastExec struct {
	Executed model.Money
	Pending  model.Money
	Invoices []InvoiceContribution
}

// executedAndPending walks the year's invoices and produces per-forecast
// paid/pending totals + audit contributions. Invoices are classified as
// fully paid iff Σ payments ≥ netAmount − 0.01. Enabled==false forecasts are
// skipped: their forecastExec is not populated (they don't appear in the map).
func executedAndPending(in ReconciliationInput) map[string]forecastExec {
	// Set of enabled forecast IDs (unknown IDs are ignored — data hygiene is
	// Phase 1's job; here we just don't produce output rows for them).
	enabled := make(map[string]bool, len(in.Forecasts))
	for _, f := range in.Forecasts {
		if f.Enabled() {
			enabled[f.ID()] = true
		}
	}
	out := make(map[string]forecastExec, len(enabled))
	for id := range enabled {
		out[id] = forecastExec{Executed: model.ZeroMoney(), Pending: model.ZeroMoney()}
	}

	for _, inv := range in.Invoices {
		paidTotal := inv.PaidTotal()
		fullyPaid := invoiceFullyPaid(paidTotal, inv.NetAmount())
		paidOn := latestPaidOn(inv, fullyPaid)
		for _, link := range inv.Links() {
			id := link.ForecastID()
			if !enabled[id] {
				continue
			}
			cur := out[id]
			contrib := InvoiceContribution{
				InvoiceID:    inv.ID(),
				Issuer:       inv.Issuer(),
				Number:       inv.Number(),
				IssueDate:    inv.IssueDate(),
				LinkedAmount: link.Amount(),
				FullyPaid:    fullyPaid,
				PaidOn:       paidOn,
			}
			if fullyPaid {
				cur.Executed = cur.Executed.Plus(link.Amount())
			} else {
				cur.Pending = cur.Pending.Plus(link.Amount())
			}
			cur.Invoices = append(cur.Invoices, contrib)
			out[id] = cur
		}
	}
	// Deterministic ordering for each forecast's invoice list.
	for id, fx := range out {
		sort.Slice(fx.Invoices, func(i, j int) bool {
			if !fx.Invoices[i].IssueDate.Equal(fx.Invoices[j].IssueDate) {
				return fx.Invoices[i].IssueDate.Before(fx.Invoices[j].IssueDate)
			}
			return fx.Invoices[i].Number < fx.Invoices[j].Number
		})
		out[id] = fx
	}
	return out
}

// invoiceFullyPaid = Σ payments ≥ netAmount − 0.01 (all-or-nothing rule).
func invoiceFullyPaid(paidTotal, netAmount model.Money) bool {
	// paidTotal ≥ netAmount − 0.01  ⇔  paidTotal + 0.01 ≥ netAmount
	// Using cent-level compare via Money.Cmp.
	oneCent, _ := model.MoneyFromString("0.01")
	return paidTotal.Plus(oneCent).Cmp(netAmount) >= 0
}

// latestPaidOn returns the latest payment date if fully paid, else nil.
func latestPaidOn(inv model.Invoice, fullyPaid bool) *time.Time {
	if !fullyPaid || len(inv.Payments()) == 0 {
		return nil
	}
	latest := inv.Payments()[0].PaidOn()
	for _, p := range inv.Payments()[1:] {
		if p.PaidOn().After(latest) {
			latest = p.PaidOn()
		}
	}
	return &latest
}
```

*Note on model accessors:* this task assumes `Invoice.Issuer()`, `Invoice.Number()`, `Invoice.IssueDate()`, `InvoicePayment.PaidOn()`, `ForecastInvoice.ForecastID()`, and `ForecastInvoice.Amount()` exist as unexported-field accessors on the domain models (they were introduced in Phase 1). If any is named differently, use the actual name — do not rename the model.

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/domain/services/... -run TestReconciliation -v`
Expected: PASS (both new tests + the skeleton test from Task 1).

Then: `go vet ./... && go build ./...`.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/services/reconciliation.go internal/domain/services/reconciliation_test.go
git commit -m "feat(services): reconciliation Executed/Pending per forecast with payment gating"
```

---

### Task 3: Per-group `Base` cap + per-forecast `Assigned` proration (largest-remainder)

**Files:**
- Modify: `internal/domain/services/reconciliation.go`
- Test: `internal/domain/services/reconciliation_test.go`

**Interfaces:**
- Consumes: `executedAndPending` from Task 2, types from Task 1.
- Produces (internal): a helper `assignForGroups(in ReconciliationInput, exec map[string]forecastExec) (groups map[string]groupResult, forecastAssigned map[string]model.Money)` where `groupResult` bundles `{Base, Assigned model.Money}` per group.

- [ ] **Step 1: Write the failing tests**

Append to `internal/domain/services/reconciliation_test.go`:

```go
func TestReconciliation_Group_UnderRun_AssignedEqualsExecuted(t *testing.T) {
	// Granted 100, Executed 60 → Assigned = 60, prorated to forecasts
	f1 := mkForecast(t, "CP25001", 7, "a2", "70.00")
	f2 := mkForecast(t, "CP25002", 7, "a2", "30.00")
	invGranted, _ := model.MoneyFromString("100.00")
	c, _ := model.NewConcession(2025, "A2-01", "a2", "concept", model.MoneyOf(100), invGranted)
	l1, _ := model.NewConcessionForecast(2025, "A2-01", "CP25001")
	l2, _ := model.NewConcessionForecast(2025, "A2-01", "CP25002")
	invA := mkInvoice(t, 10, 2025, "40.00", "40.00", "CP25001", "40.00")
	invB := mkInvoice(t, 11, 2025, "20.00", "20.00", "CP25002", "20.00")

	in := ReconciliationInput{
		Year: 2025, Forecasts: []model.ExpenseForecast{f1, f2},
		Concessions: []model.Concession{c},
		Links:       []model.ConcessionForecast{l1, l2},
		Invoices:    []model.Invoice{invA, invB},
	}
	exec := executedAndPending(in)
	groups, assigned := assignForGroups(in, exec)

	if got := groups["A2-01"].Base.String(); got != "60.00" {
		t.Errorf("Base = %s, want 60.00", got)
	}
	if got := assigned["CP25001"].String(); got != "40.00" {
		t.Errorf("CP25001 assigned = %s, want 40.00", got)
	}
	if got := assigned["CP25002"].String(); got != "20.00" {
		t.Errorf("CP25002 assigned = %s, want 20.00", got)
	}
}

func TestReconciliation_Group_OverRun_CappedAtGranted(t *testing.T) {
	// Granted 100, Executed 150 → Assigned = 100, prorated by share of Executed
	f1 := mkForecast(t, "CP25001", 7, "a2", "70.00")
	f2 := mkForecast(t, "CP25002", 7, "a2", "30.00")
	c, _ := model.NewConcession(2025, "A2-01", "a2", "concept", model.MoneyOf(100), model.MoneyOf(100))
	l1, _ := model.NewConcessionForecast(2025, "A2-01", "CP25001")
	l2, _ := model.NewConcessionForecast(2025, "A2-01", "CP25002")
	invA := mkInvoice(t, 10, 2025, "100.00", "100.00", "CP25001", "100.00")
	invB := mkInvoice(t, 11, 2025, "50.00", "50.00", "CP25002", "50.00")

	in := ReconciliationInput{
		Year: 2025, Forecasts: []model.ExpenseForecast{f1, f2},
		Concessions: []model.Concession{c},
		Links:       []model.ConcessionForecast{l1, l2},
		Invoices:    []model.Invoice{invA, invB},
	}
	exec := executedAndPending(in)
	groups, assigned := assignForGroups(in, exec)

	if got := groups["A2-01"].Base.String(); got != "100.00" {
		t.Errorf("Base = %s, want 100.00", got)
	}
	// share 100/150 * 100 = 66.67 (largest remainder); 50/150 * 100 = 33.33
	sum := assigned["CP25001"].Plus(assigned["CP25002"])
	if sum.String() != "100.00" {
		t.Errorf("Σ Assigned = %s, want 100.00 exactly", sum.String())
	}
}

func TestReconciliation_LargestRemainder_ClosesCent(t *testing.T) {
	// Granted 100.00, two equal Executed of 33.33 each → Σ Executed = 66.66,
	// Base = min(100, 66.66) = 66.66. Since Executed == Base, Assigned == Executed.
	// The remainder rule matters when Base rounds unevenly across shares:
	// try Granted 100 with Executed 33.33 / 33.33 → shares are 50/50 of Base=66.66
	// → 33.33 each. Sum = 66.66 exactly. No leak.
	f1 := mkForecast(t, "CP25001", 7, "a2", "50.00")
	f2 := mkForecast(t, "CP25002", 7, "a2", "50.00")
	c, _ := model.NewConcession(2025, "A2-01", "a2", "concept", model.MoneyOf(100), model.MoneyOf(100))
	l1, _ := model.NewConcessionForecast(2025, "A2-01", "CP25001")
	l2, _ := model.NewConcessionForecast(2025, "A2-01", "CP25002")
	invA := mkInvoice(t, 10, 2025, "33.33", "33.33", "CP25001", "33.33")
	invB := mkInvoice(t, 11, 2025, "33.33", "33.33", "CP25002", "33.33")

	in := ReconciliationInput{
		Year: 2025, Forecasts: []model.ExpenseForecast{f1, f2},
		Concessions: []model.Concession{c}, Links: []model.ConcessionForecast{l1, l2},
		Invoices: []model.Invoice{invA, invB},
	}
	exec := executedAndPending(in)
	groups, assigned := assignForGroups(in, exec)

	base := groups["A2-01"].Base
	sum := assigned["CP25001"].Plus(assigned["CP25002"])
	if sum.Cmp(base) != 0 {
		t.Errorf("Σ Assigned = %s, want %s exactly (no rounding leak)", sum, base)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/domain/services/... -run TestReconciliation_Group -v && go test ./internal/domain/services/... -run TestReconciliation_LargestRemainder -v`
Expected: FAIL — `undefined: assignForGroups`.

- [ ] **Step 3: Implement the helper**

Append to `internal/domain/services/reconciliation.go`:

```go
import (
	"github.com/shopspring/decimal"
	// keep existing imports
)

// groupResult carries a Concessió group's Base (=min(Granted, Executed_g)) and
// its Assigned total (equals Base — kept as a separate field so the roll-ups
// task in Task 5 can just sum without recomputing).
type groupResult struct {
	Base     model.Money
	Assigned model.Money
	Executed model.Money // Σ Executed_i for forecasts in group (used later)
}

// assignForGroups computes Base_g = min(Granted_g, Executed_g) for every
// Concessió, then prorates Base_g across the group's forecasts by each
// forecast's share of Executed_g. Uses largest-remainder to close the cent so
// Σ Assigned_i = Base_g exactly. Forecasts not in any group (or in a group
// with Executed_g == 0) get Assigned = 0.
func assignForGroups(in ReconciliationInput, exec map[string]forecastExec) (map[string]groupResult, map[string]model.Money) {
	// forecastID → groupCode (one concession per forecast per Phase 1 PK).
	forecastGroup := make(map[string]string, len(in.Links))
	// groupCode → []forecastID
	groupForecasts := make(map[string][]string, len(in.Concessions))
	for _, l := range in.Links {
		forecastGroup[l.ForecastID()] = l.GroupCode()
		groupForecasts[l.GroupCode()] = append(groupForecasts[l.GroupCode()], l.ForecastID())
	}

	groups := make(map[string]groupResult, len(in.Concessions))
	assigned := make(map[string]model.Money, len(exec))
	for id := range exec {
		assigned[id] = model.ZeroMoney()
	}

	for _, c := range in.Concessions {
		ids := groupForecasts[c.GroupCode()]
		// Σ Executed_g across the group's forecasts (only enabled ones survive
		// in the exec map; unknown ids are skipped).
		execG := model.ZeroMoney()
		for _, id := range ids {
			if fx, ok := exec[id]; ok {
				execG = execG.Plus(fx.Executed)
			}
		}
		var base model.Money
		if execG.Cmp(c.GrantedAmount()) < 0 {
			base = execG
		} else {
			base = c.GrantedAmount()
		}
		groups[c.GroupCode()] = groupResult{Base: base, Assigned: base, Executed: execG}

		if execG.IsZero() {
			continue // all Assigned_i stay at 0
		}
		// Largest-remainder: compute each forecast's fractional Assigned as
		// Base * Executed_i / Executed_g, take the floor at cent precision,
		// then distribute the remaining cents to the largest fractional parts.
		type share struct {
			id       string
			floor    model.Money
			fraction decimal.Decimal // fractional cents lost to floor
		}
		shares := make([]share, 0, len(ids))
		baseCents := base.Decimal().Mul(decimal.NewFromInt(100)) // ×100 → cent scale
		execGDec := execG.Decimal()
		var floorSumCents decimal.Decimal
		for _, id := range ids {
			fx, ok := exec[id]
			if !ok {
				continue
			}
			// exact_i (in cents) = Base * Executed_i / Executed_g * 100
			exactCents := baseCents.Mul(fx.Executed.Decimal()).Div(execGDec)
			floorCents := exactCents.Floor()
			frac := exactCents.Sub(floorCents)
			floor := model.MoneyFromDecimalCents(floorCents)
			shares = append(shares, share{id: id, floor: floor, fraction: frac})
			floorSumCents = floorSumCents.Add(floorCents)
		}
		// Distribute remainder cents (base_cents − Σ floor_cents) to the largest fractions.
		remaining := baseCents.Sub(floorSumCents).IntPart()
		// Stable sort by fraction desc; tie-break by id asc.
		sort.SliceStable(shares, func(i, j int) bool {
			if !shares[i].fraction.Equal(shares[j].fraction) {
				return shares[i].fraction.GreaterThan(shares[j].fraction)
			}
			return shares[i].id < shares[j].id
		})
		oneCent, _ := model.MoneyFromString("0.01")
		for i := range shares {
			assign := shares[i].floor
			if int64(i) < remaining {
				assign = assign.Plus(oneCent)
			}
			assigned[shares[i].id] = assign
		}
	}
	return groups, assigned
}
```

*New helper on `Money`:* the code above uses `model.MoneyFromDecimalCents(decimal.Decimal)` — a constructor that takes cent-scale decimal and returns Money at scale 2. If this helper doesn't exist in `internal/domain/model/money.go`, add it as a small addition in this same task:

```go
// MoneyFromDecimalCents constructs a Money from a cent-scale decimal (e.g.
// 12345 → 123.45). Used by services for largest-remainder cent-close.
func MoneyFromDecimalCents(cents decimal.Decimal) Money {
	return Money{amount: cents.Div(decimal.NewFromInt(100)).Round(2)}
}
```

Add this in `internal/domain/model/money.go` (near the other constructors), preserving the file's existing style. Include this in the same commit.

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/domain/services/... -run TestReconciliation -v`
Expected: PASS (all Task 1-3 tests).

Then: `go vet ./... && go test ./... 2>&1 | tail -20` — full test suite must still pass (Money change ripples).

- [ ] **Step 5: Commit**

```bash
git add internal/domain/services/reconciliation.go internal/domain/services/reconciliation_test.go internal/domain/model/money.go
git commit -m "feat(services): reconciliation per-group cap + largest-remainder assignment"
```

---

### Task 4: Per-forecast status flags

**Files:**
- Modify: `internal/domain/services/reconciliation.go`
- Test: `internal/domain/services/reconciliation_test.go`

**Interfaces:**
- Consumes: `forecastExec` (Task 2), `groupResult` (Task 3), `ExpenseForecast.GrossAmount()`.
- Produces (internal): a helper `statusFor(f model.ExpenseForecast, fx forecastExec, g groupResult, hasGroup bool) ForecastReconStatus`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/domain/services/reconciliation_test.go`:

```go
func TestReconciliation_Status_NoInvoice(t *testing.T) {
	f := mkForecast(t, "CP25001", 7, "a2", "100.00")
	fx := forecastExec{Executed: model.ZeroMoney(), Pending: model.ZeroMoney()}
	got := statusFor(f, fx, groupResult{}, true)
	if got != StatusNoInvoice {
		t.Errorf("status = %v, want StatusNoInvoice", got)
	}
}

func TestReconciliation_Status_PaymentPending_WhenAnyLinkUnpaid(t *testing.T) {
	f := mkForecast(t, "CP25001", 7, "a2", "100.00")
	// Both paid and unpaid links → still PaymentPending
	pending, _ := model.MoneyFromString("30.00")
	execAmt, _ := model.MoneyFromString("70.00")
	fx := forecastExec{Executed: execAmt, Pending: pending, Invoices: []InvoiceContribution{{}, {}}}
	g := groupResult{Base: model.MoneyOf(100), Assigned: model.MoneyOf(100), Executed: execAmt}
	got := statusFor(f, fx, g, true)
	if got != StatusPaymentPending {
		t.Errorf("status = %v, want StatusPaymentPending", got)
	}
}

func TestReconciliation_Status_OverExecuted(t *testing.T) {
	f := mkForecast(t, "CP25001", 7, "a2", "50.00")
	// Paid Executed 80 > GrossAmount 50, no pending, group fully justified
	exec, _ := model.MoneyFromString("80.00")
	fx := forecastExec{Executed: exec, Pending: model.ZeroMoney(), Invoices: []InvoiceContribution{{}}}
	g := groupResult{Base: model.MoneyOf(100), Assigned: model.MoneyOf(100), Executed: exec}
	got := statusFor(f, fx, g, true)
	if got != StatusOverExecuted {
		t.Errorf("status = %v, want StatusOverExecuted", got)
	}
}

func TestReconciliation_Status_PartiallyJustified(t *testing.T) {
	f := mkForecast(t, "CP25001", 7, "a2", "100.00")
	// Executed 60 < GrossAmount 100, no pending, group Executed 60 < Granted 100
	exec, _ := model.MoneyFromString("60.00")
	granted, _ := model.MoneyFromString("100.00")
	fx := forecastExec{Executed: exec, Pending: model.ZeroMoney(), Invoices: []InvoiceContribution{{}}}
	g := groupResult{Base: exec, Assigned: exec, Executed: exec, Granted: granted}
	got := statusFor(f, fx, g, true /*hasGroup*/)
	if got != StatusPartiallyJustified {
		t.Errorf("status = %v, want StatusPartiallyJustified", got)
	}
}

func TestReconciliation_Status_FullyJustified(t *testing.T) {
	f := mkForecast(t, "CP25001", 7, "a2", "100.00")
	// Executed 100 == GrossAmount, group Executed >= Granted
	exec, _ := model.MoneyFromString("100.00")
	granted, _ := model.MoneyFromString("100.00")
	fx := forecastExec{Executed: exec, Pending: model.ZeroMoney(), Invoices: []InvoiceContribution{{}}}
	g := groupResult{Base: granted, Assigned: granted, Executed: exec, Granted: granted}
	got := statusFor(f, fx, g, true)
	if got != StatusFullyJustified {
		t.Errorf("status = %v, want StatusFullyJustified", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/domain/services/... -run TestReconciliation_Status -v`
Expected: FAIL — `undefined: statusFor`.

- [ ] **Step 3: Implement the helper**

The `statusFor` helper needs both the group's `Executed` and the group's `Granted` to distinguish `PartiallyJustified` from `FullyJustified`. Extend `groupResult` (defined in Task 3) to carry `Granted model.Money` too — do this by editing `assignForGroups` to populate it.

First, add the field and populate it. Edit the `groupResult` declaration and the loop in `assignForGroups` (Task 3's file):

```go
type groupResult struct {
	Base     model.Money
	Assigned model.Money
	Executed model.Money
	Granted  model.Money // Concession.GrantedAmount, needed by status precedence
}

// Inside assignForGroups, when constructing the groupResult:
groups[c.GroupCode()] = groupResult{
	Base: base, Assigned: base, Executed: execG, Granted: c.GrantedAmount(),
}
```

Then append the `statusFor` helper to `internal/domain/services/reconciliation.go`:

```go
// statusFor applies the precedence rule from the Phase 2 spec:
// 1. NoInvoice      — zero links total.
// 2. PaymentPending — has any unpaid link (Pending > 0).
// 3. OverExecuted   — paid Executed_i > GrossAmount_i.
// 4. PartiallyJustified — group Executed < Granted.
// 5. FullyJustified — group Executed ≥ Granted.
// A forecast not attached to any group (data hygiene issue) is treated as
// NoInvoice.
func statusFor(f model.ExpenseForecast, fx forecastExec, g groupResult, hasGroup bool) ForecastReconStatus {
	if len(fx.Invoices) == 0 || !hasGroup {
		return StatusNoInvoice
	}
	if !fx.Pending.IsZero() {
		return StatusPaymentPending
	}
	if fx.Executed.Cmp(f.GrossAmount()) > 0 {
		return StatusOverExecuted
	}
	if g.Executed.Cmp(g.Granted) < 0 {
		return StatusPartiallyJustified
	}
	return StatusFullyJustified
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/domain/services/... -run TestReconciliation -v`
Expected: PASS.

Then: `go vet ./... && go build ./...`.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/services/reconciliation.go internal/domain/services/reconciliation_test.go
git commit -m "feat(services): reconciliation per-forecast status flags"
```

---

### Task 5: Roll-ups, hierarchy, category-net deviation — finish `ComputeReconciliation`

**Files:**
- Modify: `internal/domain/services/reconciliation.go`
- Test: `internal/domain/services/reconciliation_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1-4.
- Produces: filled-in `ComputeReconciliation` returning the full `ReconciliationData` tree; empty categories/subtypes/concessions are omitted.

- [ ] **Step 1: Write the failing tests**

Append to `internal/domain/services/reconciliation_test.go`:

```go
// helpers to build taxonomy + partners for hierarchy tests
func taxTypesAndSubtypes(t *testing.T) ([]model.ExpenseType, []model.ExpenseSubtype) {
	t.Helper()
	tCurrent, _ := model.NewExpenseType(2025, "A", "Corrents", model.CategoryCurrent)
	tInvest, _ := model.NewExpenseType(2025, "B", "Inversió", model.CategoryInvestment)
	a2, _ := model.NewExpenseSubtype(2025, "a2", "Prom.", "A")
	a4, _ := model.NewExpenseSubtype(2025, "a4", "Preven.", "A")
	a6, _ := model.NewExpenseSubtype(2025, "a6", "Fert.", "A")
	b1, _ := model.NewExpenseSubtype(2025, "b1", "Maquin.", "B")
	b2, _ := model.NewExpenseSubtype(2025, "b2", "Etno.", "B")
	return []model.ExpenseType{tCurrent, tInvest}, []model.ExpenseSubtype{a2, a4, a6, b1, b2}
}

func TestReconciliation_Hierarchy_EmptyCategoryOmitted(t *testing.T) {
	// Only INVESTMENT concessions → output has 1 category
	f := mkForecast(t, "CP25001", 7, "b1", "100.00")
	c, _ := model.NewConcession(2025, "B1-01", "b1", "concept", model.MoneyOf(100), model.MoneyOf(100))
	l, _ := model.NewConcessionForecast(2025, "B1-01", "CP25001")
	inv := mkInvoice(t, 1, 2025, "100.00", "100.00", "CP25001", "100.00")
	types, subs := taxTypesAndSubtypes(t)

	in := ReconciliationInput{
		Year: 2025, Forecasts: []model.ExpenseForecast{f},
		Concessions: []model.Concession{c}, Links: []model.ConcessionForecast{l},
		Invoices: []model.Invoice{inv}, Types: types, Subtypes: subs,
	}
	got, err := ComputeReconciliation(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Categories) != 1 {
		t.Fatalf("Categories = %d, want 1", len(got.Categories))
	}
	if got.Categories[0].Category != model.CategoryInvestment {
		t.Errorf("Category = %v, want INVESTMENT", got.Categories[0].Category)
	}
}

func TestReconciliation_CategoryNetDeviation_A4OverA6Under(t *testing.T) {
	// a4 over-spent by 461, a6 under-spent by 879 → NetDeviation for CURRENT = 418
	fA4 := mkForecast(t, "CP25004", 7, "a4", "500.00")
	fA6 := mkForecast(t, "CP25006", 7, "a6", "1000.00")

	// a4 concession: Granted 500, Executed 961 → deviation −461
	cA4, _ := model.NewConcession(2025, "A4-01", "a4", "cA4",
		mustMoney(t, "500.00"), mustMoney(t, "500.00"))
	lA4, _ := model.NewConcessionForecast(2025, "A4-01", "CP25004")
	invA4 := mkInvoice(t, 1, 2025, "961.00", "961.00", "CP25004", "961.00")

	// a6 concession: Granted 1000, Executed 121 → deviation +879
	cA6, _ := model.NewConcession(2025, "A6-01", "a6", "cA6",
		mustMoney(t, "1000.00"), mustMoney(t, "1000.00"))
	lA6, _ := model.NewConcessionForecast(2025, "A6-01", "CP25006")
	invA6 := mkInvoice(t, 2, 2025, "121.00", "121.00", "CP25006", "121.00")

	types, subs := taxTypesAndSubtypes(t)
	in := ReconciliationInput{
		Year: 2025, Forecasts: []model.ExpenseForecast{fA4, fA6},
		Concessions: []model.Concession{cA4, cA6},
		Links:       []model.ConcessionForecast{lA4, lA6},
		Invoices:    []model.Invoice{invA4, invA6},
		Types:       types, Subtypes: subs,
	}
	got, _ := ComputeReconciliation(in)
	if len(got.Categories) != 1 {
		t.Fatalf("want 1 category, got %d", len(got.Categories))
	}
	cat := got.Categories[0]
	if cat.NetDeviation.String() != "418.00" {
		t.Errorf("NetDeviation = %s, want 418.00", cat.NetDeviation.String())
	}
	// Per-subtype deviations preserved (raw, not netted)
	subMap := map[string]SubtypeReconciliation{}
	for _, s := range cat.Subtypes {
		subMap[s.Code] = s
	}
	if got := subMap["a4"].Deviation.String(); got != "-461.00" {
		t.Errorf("a4 Deviation = %s, want -461.00", got)
	}
	if got := subMap["a6"].Deviation.String(); got != "879.00" {
		t.Errorf("a6 Deviation = %s, want 879.00", got)
	}
}

func TestReconciliation_DisabledForecastsSkipped(t *testing.T) {
	f := mkForecast(t, "CP25001", 7, "a2", "100.00")
	// A disabled forecast — build it directly with enabled=false
	planned := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	p, _ := model.NewPartner(7, "X", "Y", "V", "y@e.cat", "6", model.Productor, 1, planned, false)
	disabled, _ := model.NewSavedExpenseForecast("CP25002", p, "d", "", model.MoneyOf(50), model.ZeroMoney(),
		nil, planned, 2025, "a2", model.NewCommonScope(), planned, false /*enabled=false*/)
	c, _ := model.NewConcession(2025, "A2-01", "a2", "concept", model.MoneyOf(150), model.MoneyOf(150))
	l1, _ := model.NewConcessionForecast(2025, "A2-01", "CP25001")
	l2, _ := model.NewConcessionForecast(2025, "A2-01", "CP25002")
	invA := mkInvoice(t, 1, 2025, "100.00", "100.00", "CP25001", "100.00")
	invB := mkInvoice(t, 2, 2025, "50.00", "50.00", "CP25002", "50.00")

	types, subs := taxTypesAndSubtypes(t)
	in := ReconciliationInput{
		Year: 2025, Forecasts: []model.ExpenseForecast{f, disabled},
		Concessions: []model.Concession{c},
		Links:       []model.ConcessionForecast{l1, l2},
		Invoices:    []model.Invoice{invA, invB},
		Types:       types, Subtypes: subs,
	}
	got, _ := ComputeReconciliation(in)
	// CP25002 should not appear anywhere; its 50 shouldn't be in Executed either.
	for _, cat := range got.Categories {
		for _, s := range cat.Subtypes {
			for _, cn := range s.Concessions {
				for _, fr := range cn.Forecasts {
					if fr.ForecastID == "CP25002" {
						t.Fatal("disabled forecast leaked into output")
					}
				}
			}
		}
	}
}

// mustMoney is a small helper.
func mustMoney(t *testing.T, s string) model.Money {
	t.Helper()
	m, err := model.MoneyFromString(s)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/domain/services/... -run TestReconciliation_Hierarchy -v && go test ./internal/domain/services/... -run TestReconciliation_CategoryNet -v && go test ./internal/domain/services/... -run TestReconciliation_DisabledForecasts -v`
Expected: FAIL — the skeleton `ComputeReconciliation` returns empty `Categories`.

- [ ] **Step 3: Implement the full `ComputeReconciliation`**

Replace the skeleton body of `ComputeReconciliation` in `internal/domain/services/reconciliation.go` with the full implementation:

```go
func ComputeReconciliation(in ReconciliationInput) (ReconciliationData, error) {
	// Stage 1: paid vs pending per forecast, and their invoice contributions.
	exec := executedAndPending(in)

	// Stage 2: per-group Base cap + per-forecast Assigned proration.
	groups, assigned := assignForGroups(in, exec)

	// Lookups.
	forecastByID := make(map[string]model.ExpenseForecast, len(in.Forecasts))
	for _, f := range in.Forecasts {
		if f.Enabled() {
			forecastByID[f.ID()] = f
		}
	}
	partnerIDForForecast := make(map[string]int, len(in.Forecasts))
	for _, f := range in.Forecasts {
		// PartnerID comes off the forecast's own accessor. If ExpenseForecast
		// exposes it as PartnerID(), use that; otherwise adapt to the actual name.
		partnerIDForForecast[f.ID()] = f.PartnerID()
	}
	subtypeCategory := make(map[string]model.ExpenseCategory, len(in.Subtypes))
	subtypeLabel := make(map[string]string, len(in.Subtypes))
	typeCategory := make(map[string]model.ExpenseCategory, len(in.Types))
	for _, tp := range in.Types {
		typeCategory[tp.Code()] = tp.Category()
	}
	for _, st := range in.Subtypes {
		subtypeCategory[st.Code()] = typeCategory[st.TypeCode()]
		subtypeLabel[st.Code()] = st.Label()
	}

	// forecastID → groupCode (from Links).
	forecastGroup := make(map[string]string, len(in.Links))
	for _, l := range in.Links {
		forecastGroup[l.ForecastID()] = l.GroupCode()
	}

	// Build ConcessionReconciliation for each Concessió (only if it has
	// enabled forecasts).
	concessionsBySubtype := make(map[string][]ConcessionReconciliation, len(in.Concessions))
	for _, c := range in.Concessions {
		g := groups[c.GroupCode()]
		forecastRecs := forecastsForGroup(c.GroupCode(), in.Links, forecastByID, exec, assigned, partnerIDForForecast, g)
		if len(forecastRecs) == 0 {
			continue // no enabled forecasts in this group → skip
		}
		diff := c.GrantedAmount().Minus(g.Executed)
		concessionsBySubtype[c.SubtypeCode()] = append(concessionsBySubtype[c.SubtypeCode()], ConcessionReconciliation{
			GroupCode:  c.GroupCode(),
			Concept:    c.Concept(),
			Requested:  c.RequestedTotal(),
			Granted:    c.GrantedAmount(),
			Executed:   g.Executed,
			Assigned:   g.Assigned,
			Difference: diff,
			Forecasts:  forecastRecs,
		})
	}

	// Roll up concessions → subtypes.
	subtypesByCategory := make(map[model.ExpenseCategory][]SubtypeReconciliation, 2)
	for _, st := range in.Subtypes {
		concs := concessionsBySubtype[st.Code()]
		if len(concs) == 0 {
			continue
		}
		sort.Slice(concs, func(i, j int) bool { return concs[i].GroupCode < concs[j].GroupCode })

		var req, gr, ex, as model.Money = model.ZeroMoney(), model.ZeroMoney(), model.ZeroMoney(), model.ZeroMoney()
		for _, cn := range concs {
			req = req.Plus(cn.Requested)
			gr = gr.Plus(cn.Granted)
			ex = ex.Plus(cn.Executed)
			as = as.Plus(cn.Assigned)
		}
		dev := gr.Minus(ex)
		cat := subtypeCategory[st.Code()]
		subtypesByCategory[cat] = append(subtypesByCategory[cat], SubtypeReconciliation{
			Code:        st.Code(),
			Label:       st.Label(),
			Requested:   req,
			Granted:     gr,
			Executed:    ex,
			Assigned:    as,
			Deviation:   dev,
			Concessions: concs,
		})
	}

	// Roll up subtypes → categories, in CURRENT-then-INVESTMENT order.
	order := []model.ExpenseCategory{model.CategoryCurrent, model.CategoryInvestment}
	out := ReconciliationData{Year: in.Year}
	for _, cat := range order {
		subs := subtypesByCategory[cat]
		if len(subs) == 0 {
			continue
		}
		sort.Slice(subs, func(i, j int) bool { return subs[i].Code < subs[j].Code })

		var req, gr, ex, as, netDev model.Money = model.ZeroMoney(), model.ZeroMoney(), model.ZeroMoney(), model.ZeroMoney(), model.ZeroMoney()
		for _, s := range subs {
			req = req.Plus(s.Requested)
			gr = gr.Plus(s.Granted)
			ex = ex.Plus(s.Executed)
			as = as.Plus(s.Assigned)
			netDev = netDev.Plus(s.Deviation)
		}
		out.Categories = append(out.Categories, CategoryReconciliation{
			Category:     cat,
			Requested:    req,
			Granted:      gr,
			Executed:     ex,
			Assigned:     as,
			NetDeviation: netDev,
			Subtypes:     subs,
		})
	}
	return out, nil
}

// forecastsForGroup builds the sorted ForecastReconciliation slice for one
// Concessió, only including forecasts that are enabled and have a forecastExec
// entry.
func forecastsForGroup(
	groupCode string,
	links []model.ConcessionForecast,
	forecastByID map[string]model.ExpenseForecast,
	exec map[string]forecastExec,
	assigned map[string]model.Money,
	partnerIDForForecast map[string]int,
	g groupResult,
) []ForecastReconciliation {
	var out []ForecastReconciliation
	for _, l := range links {
		if l.GroupCode() != groupCode {
			continue
		}
		f, ok := forecastByID[l.ForecastID()]
		if !ok {
			continue // disabled or unknown
		}
		fx := exec[f.ID()]
		out = append(out, ForecastReconciliation{
			ForecastID:     f.ID(),
			PartnerID:      partnerIDForForecast[f.ID()],
			Concept:        f.Concept(),
			GrossAmount:    f.GrossAmount(),
			ApprovedAmount: f.ApprovedAmount(),
			Executed:       fx.Executed,
			Pending:        fx.Pending,
			Assigned:       assigned[f.ID()],
			Status:         statusFor(f, fx, g, true),
			Invoices:       fx.Invoices,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ForecastID < out[j].ForecastID })
	return out
}
```

*Note on `ExpenseForecast.PartnerID()`:* if the accessor is named differently in the actual model (e.g. wrapped inside `Partner()`), replace `f.PartnerID()` with the correct call. The type is `int`.

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/domain/services/... -run TestReconciliation -v`
Expected: PASS — all Task 1-5 unit tests.

Then: `go vet ./... && go test ./... 2>&1 | tail -30` — full suite must still pass.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/services/reconciliation.go internal/domain/services/reconciliation_test.go
git commit -m "feat(services): reconciliation roll-ups, hierarchy, category-net deviation"
```

---

### Task 6: Application orchestrator `ReconciliationService.Compute(ctx, year)`

**Files:**
- Modify: `internal/application/reconciliation_service.go`
- Test: `internal/application/reconciliation_service_test.go`

**Interfaces:**
- Consumes: `services.ComputeReconciliation`, `services.ReconciliationInput`, `services.ReconciliationData`, `ports.RepoSet` reads.
- Produces: public method `(s *ReconciliationService) Compute(ctx context.Context, year int) (services.ReconciliationData, error)`.

- [ ] **Step 1: Write the failing test**

Append to `internal/application/reconciliation_service_test.go`:

```go
func TestReconciliationService_Compute_HappyPath(t *testing.T) {
	world := newReconWorld(t) // seeds 2025 window, subtype a6, partner 7, one forecast CP25001
	svc := application.NewReconciliationService(world.tx)
	ctx := context.Background()

	// Seed a concession + a fully-paid invoice via AdminImport so we exercise
	// the same import path used in production.
	in := application.ReconciliationImport{
		Year: 2025,
		Concessions: []application.ConcessionInput{{
			Year: 2025, GroupCode: "A6-01", SubtypeCode: "a6", Concept: "Adob",
			RequestedTotal: model.MoneyOf(500), GrantedAmount: model.MoneyOf(500),
			ForecastIDs:    []string{world.forecastID},
		}},
		Invoices: []application.InvoiceInput{{
			Year: 2025, Issuer: "Sup", Nif: "B1", Number: "F1",
			IssueDate: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC), NetAmount: model.MoneyOf(500),
			Payments: []application.PaymentInput{{PaidOn: time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC), Amount: model.MoneyOf(500)}},
			Links:    []application.LinkInput{{ForecastID: world.forecastID, Amount: model.MoneyOf(500)}},
		}},
	}
	if _, err := svc.AdminImport(ctx, in); err != nil {
		t.Fatalf("seed AdminImport: %v", err)
	}

	got, err := svc.Compute(ctx, 2025)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if got.Year != 2025 {
		t.Errorf("Year = %d, want 2025", got.Year)
	}
	if len(got.Categories) != 1 {
		t.Fatalf("Categories = %d, want 1", len(got.Categories))
	}
	// The world helper builds only subtype a6 (CURRENT).
	if got.Categories[0].Category != model.CategoryCurrent {
		t.Errorf("Category = %v, want CURRENT", got.Categories[0].Category)
	}
	if got.Categories[0].Assigned.String() != "500.00" {
		t.Errorf("Assigned = %s, want 500.00", got.Categories[0].Assigned.String())
	}
}
```

*Note:* this reuses the existing `newReconWorld` helper in `internal/application/reconciliation_service_test.go` (unchanged). Import block additions expected: `time`, `model`, `application` — mostly already present.

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/application/... -run TestReconciliationService_Compute -v`
Expected: FAIL — `undefined: (*ReconciliationService).Compute`.

- [ ] **Step 3: Implement the orchestrator**

Append to `internal/application/reconciliation_service.go`:

```go
import (
	"github.com/pjover/espigol/internal/domain/services"
	// keep other imports
)

// Compute produces the year's reconciliation snapshot: per-forecast
// AssignedSubsidy plus subtype/category roll-ups and category-net deviations.
// Read-only — runs inside a single WithinTx for a consistent snapshot but
// never writes. No window-state gate: reconciliation is a year-keyed overlay
// editable in any window state (matches the rest of ReconciliationService).
func (s *ReconciliationService) Compute(ctx context.Context, year int) (services.ReconciliationData, error) {
	var out services.ReconciliationData
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		forecasts, err := r.Forecasts.ListByYear(ctx, year)
		if err != nil {
			return err
		}
		concessions, err := r.Concessions.ListByYear(ctx, year)
		if err != nil {
			return err
		}
		links, err := r.Concessions.ListForecastLinksByYear(ctx, year)
		if err != nil {
			return err
		}
		invoices, err := r.Invoices.ListByYear(ctx, year)
		if err != nil {
			return err
		}
		subtypes, err := r.Taxonomy.ListSubtypes(ctx, year)
		if err != nil {
			return err
		}
		types, err := r.Taxonomy.ListTypes(ctx, year)
		if err != nil {
			return err
		}

		// Load only the partners actually referenced by these forecasts.
		partnerIDs := map[int]bool{}
		for _, f := range forecasts {
			partnerIDs[f.PartnerID()] = true
		}
		partners := make([]model.Partner, 0, len(partnerIDs))
		for id := range partnerIDs {
			p, ok, err := r.Partners.FindByID(ctx, id)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("partner %d referenced by forecast not found", id)
			}
			partners = append(partners, p)
		}

		out, err = services.ComputeReconciliation(services.ReconciliationInput{
			Year:        year,
			Forecasts:   forecasts,
			Concessions: concessions,
			Links:       links,
			Invoices:    invoices,
			Subtypes:    subtypes,
			Types:       types,
			Partners:    partners,
		})
		return err
	})
	return out, err
}
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/application/... -run TestReconciliationService_Compute -v`
Expected: PASS.

Then: `go vet ./... && go test ./... 2>&1 | tail -30` — full suite must still pass.

- [ ] **Step 5: Commit**

```bash
git add internal/application/reconciliation_service.go internal/application/reconciliation_service_test.go
git commit -m "feat(application): ReconciliationService.Compute orchestrator"
```

---

### Task 7: Golden 2025 fixture integration test (skips if private/ absent)

**Files:**
- Test: `internal/application/reconciliation_service_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1-6.
- Produces: nothing (test only). Validates the algorithm against the real 2025 workbook figures.

- [ ] **Step 1: Write the test**

Append to `internal/application/reconciliation_service_test.go`:

```go
// TestReconciliation2025Fixture_ComputeMatchesWorkbook drives the end-to-end
// pipeline (importer.LoadReconciliation → AdminImport → Compute) against the
// real 2025 payload in private/export-reconciliation.json (gitignored). Skips
// when the file isn't present (dev machines without the private data).
func TestReconciliation2025Fixture_ComputeMatchesWorkbook(t *testing.T) {
	// Repo root is two directories up from internal/application.
	path := filepath.Join("..", "..", "private", "export-reconciliation.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("private fixture missing: %s", path)
	}

	world := new2025World(t) // helper defined below
	svc := application.NewReconciliationService(world.tx)
	ctx := context.Background()

	in, err := importer.LoadReconciliation(path, 2025)
	if err != nil {
		t.Fatalf("LoadReconciliation: %v", err)
	}
	if _, err := svc.AdminImport(ctx, in); err != nil {
		t.Fatalf("AdminImport: %v", err)
	}

	got, err := svc.Compute(ctx, 2025)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}

	// Assert per-subtype Executed against workbook figures.
	wantExec := map[string]string{
		"a2": "5989.00", "a3": "0.00", "a4": "1381.11",
		"a6": "18672.09", "b1": "52752.80", "b2": "1460.00",
	}
	haveExec := map[string]string{}
	for _, cat := range got.Categories {
		for _, st := range cat.Subtypes {
			haveExec[st.Code] = st.Executed.String()
		}
	}
	for code, want := range wantExec {
		if got := haveExec[code]; got != want {
			t.Errorf("subtype %s Executed = %s, want %s", code, got, want)
		}
	}

	// Assert grand total Executed = 80255.00.
	total := model.ZeroMoney()
	for _, cat := range got.Categories {
		total = total.Plus(cat.Executed)
	}
	if total.String() != "80255.00" {
		t.Errorf("grand total Executed = %s, want 80255.00", total.String())
	}

	// Every forecast has a defined status (no zero value that isn't
	// intentionally StatusFullyJustified).
	for _, cat := range got.Categories {
		for _, st := range cat.Subtypes {
			for _, cn := range st.Concessions {
				for _, fr := range cn.Forecasts {
					// Any of the 5 declared values is fine; guard against
					// bogus large ints creeping in.
					if fr.Status < services.StatusFullyJustified || fr.Status > services.StatusNoInvoice {
						t.Errorf("forecast %s: invalid status %d", fr.ForecastID, fr.Status)
					}
				}
			}
		}
	}

	// B2-01 Arreglar marges: partially justified per workbook.
	// Granted 1766.12, Executed 1460.00, Assigned 1460.00, PartiallyJustified.
	var b201 *services.ConcessionReconciliation
	for i := range got.Categories {
		for j := range got.Categories[i].Subtypes {
			for k := range got.Categories[i].Subtypes[j].Concessions {
				c := &got.Categories[i].Subtypes[j].Concessions[k]
				if c.GroupCode == "B2-01" {
					b201 = c
				}
			}
		}
	}
	if b201 == nil {
		t.Fatal("B2-01 concession not found in output")
	}
	if b201.Granted.String() != "1766.12" || b201.Executed.String() != "1460.00" || b201.Assigned.String() != "1460.00" {
		t.Errorf("B2-01 mismatch: granted=%s executed=%s assigned=%s",
			b201.Granted, b201.Executed, b201.Assigned)
	}
	if len(b201.Forecasts) != 1 || b201.Forecasts[0].Status != services.StatusPartiallyJustified {
		t.Errorf("B2-01 forecast status = %v, want StatusPartiallyJustified", b201.Forecasts[0].Status)
	}
}

// new2025World seeds a scratch SQLite DB with the 2025 taxonomy (a2/a3/a4/a6/b1/b2
// + their CURRENT/INVESTMENT types), section "oliva", partners 1/2/4/5/6/7/8/9,
// an OPEN 2025 window, and 38 forecasts CP25001..CP25038 with the concepts and
// gross amounts that match the workbook. Reads the concepts + gross amounts
// from private/export-forecasts.json.
func new2025World(t *testing.T) reconWorld {
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
	sr := persistence.NewSectionRepository(q)
	fr := persistence.NewForecastRepository(conn, q)

	// OPEN 2025 window (required for forecast import).
	planned := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	w, _ := model.NewSubmissionWindow(2025, model.WindowOpen, &planned, nil,
		time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	_ = win.Save(ctx, w)

	// Taxonomy — A (CURRENT), B (INVESTMENT), and 6 subtypes.
	tA, _ := model.NewExpenseType(2025, "A", "Corrents", model.CategoryCurrent)
	tB, _ := model.NewExpenseType(2025, "B", "Inversió", model.CategoryInvestment)
	_ = tax.SaveType(ctx, tA)
	_ = tax.SaveType(ctx, tB)
	for _, code := range []string{"a2", "a3", "a4", "a6"} {
		st, _ := model.NewExpenseSubtype(2025, code, code, "A")
		_ = tax.SaveSubtype(ctx, st)
	}
	for _, code := range []string{"b1", "b2"} {
		st, _ := model.NewExpenseSubtype(2025, code, code, "B")
		_ = tax.SaveSubtype(ctx, st)
	}

	// Section "oliva".
	sec, _ := model.NewSection("oliva", "Secció Oliva", 1, true, planned)
	_ = sr.Save(ctx, sec)

	// Partners 1/2/4/5/6/7/8/9.
	for _, pid := range []int{1, 2, 4, 5, 6, 7, 8, 9} {
		p, _ := model.NewPartner(pid, fmt.Sprintf("P%d", pid), "S", "V",
			fmt.Sprintf("p%d@e.cat", pid), "6", model.Productor, 1, planned, false)
		_ = pr.Save(ctx, p)
	}

	// Import 38 forecasts from private/export-forecasts.json via the same
	// path production uses.
	fcPath := filepath.Join("..", "..", "private", "export-forecasts.json")
	fs := application.NewForecastService(persistence.NewTxManager(conn), clock.System{})
	entries, err := importer.Load(fcPath, 2025)
	if err != nil {
		t.Fatalf("LoadForecasts: %v", err)
	}
	if _, err := fs.AdminImport(ctx, "admin@espigol.test", 2025, entries); err != nil {
		t.Fatalf("forecast AdminImport: %v", err)
	}
	_ = fr // silence unused; kept for symmetry with newReconWorld

	return reconWorld{tx: persistence.NewTxManager(conn)}
}
```

*Note:* the exact `application.NewForecastService(...)` and `clock.System{}` constructor calls must match how the production code composes ForecastService. If the signatures differ (e.g. `clock` lives at a different import path), adapt to the real names — do not change the production code.

Add any missing imports to the test file: `filepath`, `os`, `fmt`, `application`, `importer`, `db`, `sqlc`, `persistence`, `clock`, `model`, `services`.

- [ ] **Step 2: Run the test**

Run: `go test ./internal/application/... -run TestReconciliation2025Fixture -v`

Expected outcomes:
- If `private/export-reconciliation.json` is present: PASS (or a specific assertion failure that points to a real bug in the algorithm — fix and re-run).
- If the private files are absent: SKIP with message `private fixture missing: …`.

- [ ] **Step 3: Full-suite gate**

Run: `go vet ./... && go test ./... 2>&1 | tail -30`
Expected: all packages pass.

- [ ] **Step 4: Commit**

```bash
git add internal/application/reconciliation_service_test.go
git commit -m "test(application): 2025 fixture golden test for reconciliation Compute"
```

---

## Self-Review

**Spec coverage:**

- Payment gating (all-or-nothing at fully paid) — Task 2 (`invoiceFullyPaid`, `executedAndPending`).
- Per-group `min(Granted, Executed)` cap — Task 3 (`assignForGroups`).
- Largest-remainder cent-close — Task 3.
- Per-forecast status precedence (NoInvoice → PaymentPending → OverExecuted → PartiallyJustified → FullyJustified) — Task 4 (`statusFor`).
- Enabled-forecast filter — Task 2 (`executedAndPending`) + Task 5 (`forecastByID` map).
- Category-net deviation as reporting only — Task 5 (`ComputeReconciliation` rolls up subtype deviations; never touches `Assigned_i`).
- Empty-category / empty-subtype omission — Task 5 (skip branches).
- JSON-serialisable snapshot — Task 1 (all fields have `json:` tags; no pointers except `PaidOn`).
- Return-type field names in English, Catalan display deferred — Task 1.
- Application orchestrator inside `TxManager.WithinTx`, read-only — Task 6.
- Golden 2025 fixture with `t.Skip` guard — Task 7.
- No writes, no migrations, no new ports, no new repos, no TUI changes — nothing added in any task.

**Type / signature consistency:** all references to `ComputeReconciliation`, `ReconciliationInput`, `ReconciliationData`, `forecastExec`, `groupResult`, `statusFor`, `assignForGroups`, `executedAndPending`, `invoiceFullyPaid`, `latestPaidOn`, `forecastsForGroup` use the same names across tasks. `groupResult` gains a `Granted` field in Task 4 (explicit edit called out); every downstream reference uses the extended shape.

**Placeholder scan:** no TBD / TODO / "similar to task N" / vague error handling. Every step has concrete code or a specific command.

**Ambiguity check:**

- `Money.MoneyFromDecimalCents` — new helper added in Task 3 with explicit code. If the codebase already has an equivalent constructor with a different name, use that instead and drop the new helper.
- `ExpenseForecast.PartnerID()` — flagged in Tasks 5 and 6 with an "adapt if named differently" note.
- Model constructor names (`NewSavedExpenseForecast`, `NewConcession`, `NewConcessionForecast`, `NewInvoice`, `NewInvoicePayment`, `NewForecastInvoice`) are Phase-1 assumptions — the implementer should use whatever constructors Phase 1 actually shipped.
