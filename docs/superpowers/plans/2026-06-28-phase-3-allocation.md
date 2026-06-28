# Phase 3 — Allocation Algorithm Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A pure domain service that computes the full `ReportData` for a year's forecasts — porting espigol-java's `AllocationService` + `FairShareAllocator`, generalized from 2 hardcoded sections to N data-driven sections — validated by synthetic unit tests and a committed, fully-anonymized golden-value test.

**Architecture:** New `internal/domain/model/report` package holds the immutable `ReportData` value-struct tree (plain exported-field structs — they are computed outputs). New `internal/domain/services` package holds the pure `Compute(AllocationInput) (report.ReportData, error)` and the unexported `distribute` fair-share helper. Everything is a pure function over Phase-2 domain types; no DB, no ports, no I/O.

**Tech Stack:** Go 1.26, `github.com/shopspring/decimal` (via the Phase-2 `model.Money`), standard library only.

## Global Constraints

- **Module path:** `github.com/pjover/espigol`. Go **1.26**. CGO-free.
- **Pure domain:** `internal/domain/services` and `internal/domain/model/report` import only `internal/domain/model` (+ stdlib). **No `database/sql`, no `adapters`, no ports.**
- **No `float64`.** All money is the Phase-2 `model.Money` (scale 2, HALF_UP). Ratios use `decimal.Decimal` only as an intermediate for proration, never stored.
- **Faithful port** of `espigol-java` `domain/services/AllocationService.java` + `FairShareAllocator.java` (the golden-validated authority). Only intentional change: 2 hardcoded sections → N data-driven sections (`SectionDetail{code,label}`, `WarningData{Category, Rows []SectionWarning}`).
- **Rounding (verbatim):** mean `= budgetLeft.DividedBy(nUnfixed)` (HALF_UP scale-2); per-item proration `ratio = allocated.Decimal().Div(requested.Decimal())` then `gross.TimesRatio(ratio)` (rounds scale-2 HALF_UP); all magnitude comparisons via `Money.Cmp`; fair-share convergence threshold `0.01`; iteration cap `100`; non-positive pool caps at the (≤0) mean with **no clamp to zero**.
- **Determinism:** never depend on Go map iteration order for outputs — aggregate into maps but emit lists in the spec'd sort order (detail items by concept; partner allocations/details by name; subtype totals by code; sections by display order).
- **Phase-2 `model.Money` API available:** `MoneyFromString(string)(Money,error)`, `MoneyOf(int64)`, `ZeroMoney()`, `Plus`, `Minus`, `Times(int)`, `TimesRatio(decimal.Decimal)`, `DividedBy(int)`, `Cmp(Money)int`, `Decimal() decimal.Decimal`, `String() string`.
- **Phase-2 domain accessors:** `ExpenseForecast`: `ID()`, `PartnerID()`, `Concept()`, `Description()`, `GrossAmount()`, `SubtypeCode()`, `Scope() model.ExpenseScope`. `ExpenseScope`: `Kind() model.ScopeKind`, `SectionCode() string` (consts `model.ScopeCommon/ScopeSection/ScopePartner`). `Partner`: `ID()`, `Name()`, `PartnerType()` (`model.Productor`). `Section`: `Code()`, `Label()`, `DisplayOrder()`, `Active()`. `PartnerSection`: `PartnerID()`, `SectionCode()`. `ExpenseCategory` consts `model.CategoryCurrent/CategoryInvestment`.
- **TDD:** every behavioral change starts with a failing test. Commit after each green step.

---

### Task 1: ReportData model structs

**Files:**
- Create: `internal/domain/model/report/report.go`
- Test: `internal/domain/model/report/report_test.go`

**Interfaces:**
- Consumes: `internal/domain/model` (`Money`, `ExpenseCategory`).
- Produces: the `report` package value structs used by all later tasks: `ReportData`, `CategoryReportData`, `CommonData`, `SectionsData`, `SectionDetail`, `WarningData`, `SectionWarning`, `PartnersData`, `PartnerAllocation`, `PartnerDetail`, `DetailItem`, `SubtypeTotal` (exact fields below).

- [ ] **Step 1: Write the failing test**

Create `internal/domain/model/report/report_test.go`:
```go
package report

import (
	"testing"

	"github.com/pjover/espigol/internal/domain/model"
)

func TestReportDataStructsCompose(t *testing.T) {
	rd := ReportData{
		Year:                2026,
		HasNegativeRemainder: false,
		Categories: []CategoryReportData{{
			Category: model.CategoryCurrent,
			Common: CommonData{
				Available: model.MoneyOf(30000),
				Total:     model.MoneyOf(2880),
				Remainder: model.MoneyOf(27120),
				Items: []DetailItem{{
					CpCode: "CP26023", Concept: "x", Description: "",
					RequestedAmount: model.MoneyOf(2880), ApprovedAmount: model.MoneyOf(2880),
				}},
			},
			Sections: SectionsData{
				Available:  model.MoneyOf(27120),
				Total:      model.MoneyOf(27111),
				Remainder:  model.MoneyOf(9),
				SectionDetails: []SectionDetail{{SectionCode: "oliva", Label: "Secció d'oliva", Total: model.MoneyOf(19721)}},
				Partners: PartnersData{
					GrandTotal:    model.ZeroMoney(),
					FinalRemainder: model.MoneyOf(9),
					Allocations:   []PartnerAllocation{},
					SubtypeTotals: []SubtypeTotal{},
					PartnerDetails: []PartnerDetail{},
				},
			},
			Warning: nil,
		}},
	}
	if rd.Categories[0].Common.Total.String() != "2880.00" {
		t.Errorf("Common.Total = %q", rd.Categories[0].Common.Total.String())
	}
	if rd.Categories[0].Sections.SectionDetails[0].Label != "Secció d'oliva" {
		t.Errorf("section label wrong")
	}
	// SectionWarning shape compiles
	_ = WarningData{Category: model.CategoryCurrent, Rows: []SectionWarning{{
		SectionCode: "oliva", Label: "Secció d'oliva", Producers: 3,
		Allowed: model.ZeroMoney(), Requested: model.ZeroMoney(), Adjustment: model.ZeroMoney(),
	}}}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/model/report/ -v`
Expected: FAIL — undefined types.

- [ ] **Step 3: Write the structs**

Create `internal/domain/model/report/report.go`:
```go
// Package report holds the computed ReportData value tree produced by the
// allocation service. These are plain immutable-by-convention data structs
// (computed outputs assembled in one place), built on the domain Money type.
package report

import "github.com/pjover/espigol/internal/domain/model"

// DetailItem is one forecast line in a detail table.
type DetailItem struct {
	CpCode          string
	Concept         string
	Description     string
	RequestedAmount model.Money
	ApprovedAmount  model.Money
}

// SubtypeTotal is the gross total of partner-scope forecasts for one subtype.
type SubtypeTotal struct {
	SubtypeCode string
	Amount      model.Money
}

// PartnerAllocation is the fair-share result for one partner.
type PartnerAllocation struct {
	PartnerID   int
	PartnerName string
	Requested   model.Money
	Allocated   model.Money
}

// PartnerDetail is one partner's per-item breakdown with proration applied.
type PartnerDetail struct {
	Name          string
	Items         []DetailItem
	Total         model.Money
	IsCapped      bool
	MaxAuthorized model.Money
}

// CommonData is the COMMON-scope block of a category.
type CommonData struct {
	Available model.Money
	Total     model.Money
	Remainder model.Money
	Items     []DetailItem
}

// SectionDetail is one section's block (data-driven; code+label, not an enum).
type SectionDetail struct {
	SectionCode string
	Label       string
	Items       []DetailItem
	Total       model.Money
}

// SectionWarning is one section's row in the proportional-adjustment warning.
type SectionWarning struct {
	SectionCode string
	Label       string
	Producers   int
	Allowed     model.Money
	Requested   model.Money
	Adjustment  model.Money
}

// WarningData is the proportional-adjustment warning for a category (N sections).
type WarningData struct {
	Category model.ExpenseCategory
	Rows     []SectionWarning
}

// PartnersData is the Soci-scope block of a category.
type PartnersData struct {
	SubtypeTotals  []SubtypeTotal
	GrandTotal     model.Money
	HasExcess      bool
	FinalRemainder model.Money
	Allocations    []PartnerAllocation
	PartnerDetails []PartnerDetail
}

// SectionsData is the sections block (all sections + the Soci block).
type SectionsData struct {
	Available      model.Money
	Total          model.Money
	Remainder      model.Money
	SectionDetails []SectionDetail
	Partners       PartnersData
}

// CategoryReportData is one expense category's computed report.
type CategoryReportData struct {
	Category model.ExpenseCategory
	Common   CommonData
	Sections SectionsData
	Warning  *WarningData // nil unless this category's sections remainder < 0
}

// ReportData is the full computed report for a year (CURRENT then INVESTMENT).
type ReportData struct {
	Year                 int
	HasNegativeRemainder bool
	Categories           []CategoryReportData
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/model/report/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/model/report/
git commit -m "feat(report): ReportData value structs generalized to N sections"
```

---

### Task 2: FairShareAllocator

**Files:**
- Create: `internal/domain/services/fairshare.go`
- Test: `internal/domain/services/fairshare_test.go`

**Interfaces:**
- Consumes: `model.Money`, `report.PartnerAllocation`.
- Produces (package-internal): `type fairShareResult struct { allocations []report.PartnerAllocation; finalRemainder model.Money }` and `func distribute(remainder model.Money, partnerTotals map[int]model.Money, partnerNames map[int]string) fairShareResult`. Used by Task 3.

- [ ] **Step 1: Write the failing test**

Create `internal/domain/services/fairshare_test.go`:
```go
package services

import (
	"strconv"
	"testing"

	"github.com/pjover/espigol/internal/domain/model"
)

func names(ids ...int) map[int]string {
	m := map[int]string{}
	for _, id := range ids {
		m[id] = "P" + itoa(id)
	}
	return m
}

// itoa is the shared test helper for partner-id strings (handles multi-digit ids, e.g. 11).
func itoa(i int) string { return strconv.Itoa(i) }

func allocByID(r fairShareResult) map[int]string {
	out := map[int]string{}
	for _, a := range r.allocations {
		out[a.PartnerID] = a.Allocated.String()
	}
	return out
}

func TestDistribute_NoExcess_EveryoneFull(t *testing.T) {
	totals := map[int]model.Money{1: model.MoneyOf(100), 2: model.MoneyOf(200)}
	r := distribute(model.MoneyOf(1000), totals, names(1, 2))
	got := allocByID(r)
	if got[1] != "100.00" || got[2] != "200.00" {
		t.Errorf("allocations = %v, want full", got)
	}
	if r.finalRemainder.String() != "700.00" {
		t.Errorf("finalRemainder = %q, want 700.00", r.finalRemainder.String())
	}
}

func TestDistribute_Excess_CapsHighRequesters(t *testing.T) {
	// budget 300, three partners want 100/100/400 (total 600 > 300).
	// Round 1: mean=100. p1,p2 (=100) fixed, budget 100 left, 1 unfixed.
	// Round 2: mean=100. p3 alloc 400 > 100 -> none newly fixed -> cap p3 at 100.
	totals := map[int]model.Money{1: model.MoneyOf(100), 2: model.MoneyOf(100), 3: model.MoneyOf(400)}
	r := distribute(model.MoneyOf(300), totals, names(1, 2, 3))
	got := allocByID(r)
	if got[1] != "100.00" || got[2] != "100.00" || got[3] != "100.00" {
		t.Errorf("allocations = %v, want 100/100/100", got)
	}
	if r.finalRemainder.String() != "0.00" {
		t.Errorf("finalRemainder = %q, want 0.00", r.finalRemainder.String())
	}
}

func TestDistribute_AllAboveMean_CappedEqually(t *testing.T) {
	// budget 300, two partners want 400/500 -> both above mean 150 -> cap both at 150.
	totals := map[int]model.Money{1: model.MoneyOf(400), 2: model.MoneyOf(500)}
	r := distribute(model.MoneyOf(300), totals, names(1, 2))
	got := allocByID(r)
	if got[1] != "150.00" || got[2] != "150.00" {
		t.Errorf("allocations = %v, want 150/150", got)
	}
}

func TestDistribute_NonPositivePool_NoClamp(t *testing.T) {
	// negative remainder: everyone capped at a negative mean, no clamp to zero.
	totals := map[int]model.Money{1: model.MoneyOf(100), 2: model.MoneyOf(100)}
	r := distribute(model.MoneyOf(-10), totals, names(1, 2))
	got := allocByID(r)
	if got[1] != "-5.00" || got[2] != "-5.00" {
		t.Errorf("allocations = %v, want -5.00/-5.00 (no clamp)", got)
	}
}

func TestDistribute_Empty(t *testing.T) {
	r := distribute(model.MoneyOf(50), map[int]model.Money{}, map[int]string{})
	if len(r.allocations) != 0 || r.finalRemainder.String() != "50.00" {
		t.Errorf("empty: allocations=%d finalRemainder=%q", len(r.allocations), r.finalRemainder.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/services/ -run TestDistribute -v`
Expected: FAIL — undefined `distribute`.

- [ ] **Step 3: Write the implementation**

Create `internal/domain/services/fairshare.go`:
```go
package services

import (
	"sort"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/model/report"
)

const (
	maxIterations        = 100
	convergenceThreshold = "0.01"
)

type fairShareResult struct {
	allocations    []report.PartnerAllocation
	finalRemainder model.Money
}

// distribute runs the iterative fair-share distribution: partners requesting at
// most the per-head mean keep their request; the rest are capped at the mean.
// Ported verbatim from espigol-java FairShareAllocator.
func distribute(remainder model.Money, partnerTotals map[int]model.Money, partnerNames map[int]string) fairShareResult {
	if len(partnerTotals) == 0 {
		return fairShareResult{allocations: []report.PartnerAllocation{}, finalRemainder: remainder}
	}

	ids := sortedIDs(partnerTotals)
	requested := map[int]model.Money{}
	allocated := map[int]model.Money{}
	fixed := map[int]bool{}
	for _, id := range ids {
		requested[id] = partnerTotals[id]
		allocated[id] = partnerTotals[id]
		fixed[id] = false
	}

	totalRequested := sumMoney(values(requested, ids))

	// Case 1: no excess — everyone gets their full request.
	if totalRequested.Cmp(remainder) <= 0 {
		return fairShareResult{
			allocations:    buildAllocations(ids, requested, requested, partnerNames),
			finalRemainder: remainder.Minus(totalRequested),
		}
	}

	// Case 2: iterative fair share.
	threshold := mustMoney(convergenceThreshold)
	budgetLeft := remainder
	for iter := 0; iter < maxIterations; iter++ {
		nUnfixed := 0
		for _, id := range ids {
			if !fixed[id] {
				nUnfixed++
			}
		}
		if nUnfixed == 0 {
			break
		}
		mean := budgetLeft.DividedBy(nUnfixed)

		newlyFixed := false
		for _, id := range ids {
			if fixed[id] {
				continue
			}
			if allocated[id].Cmp(mean) <= 0 {
				fixed[id] = true
				budgetLeft = budgetLeft.Minus(allocated[id])
				newlyFixed = true
			}
		}
		if !newlyFixed {
			// All remaining requests exceed the mean — cap them at the mean.
			for _, id := range ids {
				if !fixed[id] {
					allocated[id] = mean
					fixed[id] = true
				}
			}
			break
		}
		diff := absDecimal(remainder.Decimal().Sub(sumMoney(values(allocated, ids)).Decimal()))
		if diff.Cmp(threshold.Decimal()) < 0 {
			break
		}
	}

	return fairShareResult{
		allocations:    buildAllocations(ids, requested, allocated, partnerNames),
		finalRemainder: remainder.Minus(sumMoney(values(allocated, ids))),
	}
}

func buildAllocations(ids []int, requested, allocated map[int]model.Money, nameByID map[int]string) []report.PartnerAllocation {
	out := make([]report.PartnerAllocation, 0, len(ids))
	for _, id := range ids {
		out = append(out, report.PartnerAllocation{
			PartnerID:   id,
			PartnerName: nameByID[id],
			Requested:   requested[id],
			Allocated:   allocated[id],
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].PartnerName < out[j].PartnerName })
	return out
}

func sortedIDs(m map[int]model.Money) []int {
	ids := make([]int, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

func values(m map[int]model.Money, ids []int) []model.Money {
	out := make([]model.Money, 0, len(ids))
	for _, id := range ids {
		out = append(out, m[id])
	}
	return out
}

func sumMoney(ms []model.Money) model.Money {
	total := model.ZeroMoney()
	for _, m := range ms {
		total = total.Plus(m)
	}
	return total
}
```

Also add two tiny shared helpers used here and in Task 3. Create `internal/domain/services/util.go`:
```go
package services

import "github.com/shopspring/decimal"

import "github.com/pjover/espigol/internal/domain/model"

func mustMoney(s string) model.Money {
	m, err := model.MoneyFromString(s)
	if err != nil {
		panic("services: invalid money literal " + s)
	}
	return m
}

func absDecimal(d decimal.Decimal) decimal.Decimal { return d.Abs() }
```

Note: Go does not allow two `import` statements stacked like that — combine them into one block in `util.go`:
```go
package services

import (
	"github.com/shopspring/decimal"

	"github.com/pjover/espigol/internal/domain/model"
)
```
(Use this single import block; the two-line form above is illustrative only.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/services/ -run TestDistribute -v && go build ./...`
Expected: PASS, build clean.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/services/fairshare.go internal/domain/services/util.go internal/domain/services/fairshare_test.go
git commit -m "feat(services): fair-share allocator ported from Java reference"
```

---

### Task 3: AllocationService.Compute (common + sections + socis, no warning yet)

**Files:**
- Create: `internal/domain/services/allocation.go`
- Test: `internal/domain/services/allocation_test.go`

**Interfaces:**
- Consumes: `distribute` (Task 2); `model` types; `report` structs (Task 1).
- Produces:
  - `type AllocationInput struct { Year int; Forecasts []model.ExpenseForecast; Partners []model.Partner; Sections []model.Section; Memberships []model.PartnerSection; SubtypeCategory map[string]model.ExpenseCategory; CurrentLimit model.Money; InvestmentLimit model.Money }`
  - `func Compute(in AllocationInput) (report.ReportData, error)` — fills `Warning: nil` for now (Task 4 adds the warning).

- [ ] **Step 1: Write the failing test**

Create `internal/domain/services/allocation_test.go`:
```go
package services

import (
	"testing"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
)

func d(t *testing.T, s string) model.Money {
	t.Helper()
	m, err := model.MoneyFromString(s)
	if err != nil {
		t.Fatalf("money %q: %v", s, err)
	}
	return m
}

func mkForecast(t *testing.T, id string, partnerID int, gross string, scope model.ExpenseScope, subtype string) model.ExpenseForecast {
	t.Helper()
	planned := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	f, err := model.NewExpenseForecast(id, partnerID, "Concepte "+id, "", d(t, gross), model.ZeroMoney(),
		nil, planned, 2026, subtype, scope, planned, true)
	if err != nil {
		t.Fatalf("forecast %s: %v", id, err)
	}
	return f
}

func mkPartner(t *testing.T, id int) model.Partner {
	t.Helper()
	p, err := model.NewPartner(id, "Soci "+itoa(id), "", "", "soci@x.test", "", model.Productor, 0,
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), false)
	if err != nil {
		t.Fatalf("partner %d: %v", id, err)
	}
	return p
}

func section(t *testing.T, code, label string, order int) model.Section {
	t.Helper()
	s, err := model.NewSection(code, label, true, order)
	if err != nil {
		t.Fatalf("section %s: %v", code, err)
	}
	return s
}

// A small CURRENT-only scenario: common 100, one section 'a' total 30, socis 0.
// limit 200 -> common remainder 100; availableForSections 100; sectionsRemainder 70.
func TestCompute_CommonSectionsSocis_Basic(t *testing.T) {
	common := mkForecast(t, "CP26001", 1, "100.00", model.NewCommonScope(), "a1")
	secScope, _ := model.NewSectionScope("a")
	sec := mkForecast(t, "CP26002", 1, "30.00", secScope, "a1")
	in := AllocationInput{
		Year:            2026,
		Forecasts:       []model.ExpenseForecast{common, sec},
		Partners:        []model.Partner{mkPartner(t, 1)},
		Sections:        []model.Section{section(t, "a", "Secció A", 1)},
		Memberships:     nil,
		SubtypeCategory: map[string]model.ExpenseCategory{"a1": model.CategoryCurrent},
		CurrentLimit:    model.MoneyOf(200),
		InvestmentLimit: model.MoneyOf(0),
	}

	rd, err := Compute(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(rd.Categories) != 2 {
		t.Fatalf("want 2 categories, got %d", len(rd.Categories))
	}
	cur := rd.Categories[0]
	if cur.Category != model.CategoryCurrent {
		t.Errorf("first category should be CURRENT")
	}
	if cur.Common.Total.String() != "100.00" || cur.Common.Remainder.String() != "100.00" {
		t.Errorf("common: total=%s remainder=%s", cur.Common.Total, cur.Common.Remainder)
	}
	if cur.Sections.Available.String() != "100.00" || cur.Sections.Total.String() != "30.00" || cur.Sections.Remainder.String() != "70.00" {
		t.Errorf("sections: avail=%s total=%s rem=%s", cur.Sections.Available, cur.Sections.Total, cur.Sections.Remainder)
	}
	if len(cur.Sections.SectionDetails) != 1 || cur.Sections.SectionDetails[0].Label != "Secció A" {
		t.Errorf("section detail wrong: %+v", cur.Sections.SectionDetails)
	}
	if cur.Sections.Partners.GrandTotal.String() != "0.00" || cur.Sections.Partners.FinalRemainder.String() != "70.00" {
		t.Errorf("socis: grand=%s final=%s", cur.Sections.Partners.GrandTotal, cur.Sections.Partners.FinalRemainder)
	}
	if cur.Warning != nil {
		t.Errorf("warning should be nil for positive remainder")
	}
}

// Excess socis with capping + per-item proration.
func TestCompute_SocisCappedProration(t *testing.T) {
	// sectionsRemainder must be small so socis exceed it. No common, no sections.
	// limit 100 -> availableForSections 100, sectionsRemainder 100.
	// two partners: p1 wants 40 (one item), p2 wants 400 (one item). total 440 > 100.
	// round1 mean 50: p1(40)<=50 fixed, budget 60, 1 unfixed.
	// round2 mean 60: p2(400)>60 none newly fixed -> cap p2 at 60.
	pScope := model.NewPartnerScope()
	f1 := mkForecast(t, "CP26010", 1, "40.00", pScope, "a1")
	f2 := mkForecast(t, "CP26011", 2, "400.00", pScope, "a1")
	in := AllocationInput{
		Year:            2026,
		Forecasts:       []model.ExpenseForecast{f1, f2},
		Partners:        []model.Partner{mkPartner(t, 1), mkPartner(t, 2)},
		Sections:        nil,
		SubtypeCategory: map[string]model.ExpenseCategory{"a1": model.CategoryCurrent},
		CurrentLimit:    model.MoneyOf(100),
		InvestmentLimit: model.MoneyOf(0),
	}
	rd, err := Compute(in)
	if err != nil {
		t.Fatal(err)
	}
	pd := rd.Categories[0].Sections.Partners
	if !pd.HasExcess {
		t.Errorf("expected HasExcess true")
	}
	allocated := map[int]string{}
	for _, a := range pd.Allocations {
		allocated[a.PartnerID] = a.Allocated.String()
	}
	if allocated[1] != "40.00" || allocated[2] != "60.00" {
		t.Errorf("allocations = %v, want p1 40.00 p2 60.00", allocated)
	}
	// p2 is capped: its single item (gross 400) prorated by 60/400 -> approved 60.00
	var p2 *struct{}
	for _, det := range pd.PartnerDetails {
		if det.Name == "Soci 2 (2)" {
			if !det.IsCapped || det.MaxAuthorized.String() != "60.00" {
				t.Errorf("p2 detail: capped=%v max=%s", det.IsCapped, det.MaxAuthorized)
			}
			if det.Items[0].ApprovedAmount.String() != "60.00" {
				t.Errorf("p2 item approved = %s, want 60.00", det.Items[0].ApprovedAmount)
			}
			p2 = &struct{}{}
		}
	}
	if p2 == nil {
		t.Errorf("p2 detail not found")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/services/ -run TestCompute -v`
Expected: FAIL — undefined `Compute` / `AllocationInput`.

- [ ] **Step 3: Write the implementation**

Create `internal/domain/services/allocation.go`:
```go
// Package services holds pure domain services. AllocationService.Compute is the
// allocation algorithm: it computes the full ReportData for a year's forecasts,
// ported from espigol-java AllocationService, generalized to N data-driven sections.
package services

import (
	"fmt"
	"sort"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/model/report"
)

// AllocationInput is the complete input to Compute (assembled by the caller).
type AllocationInput struct {
	Year            int
	Forecasts       []model.ExpenseForecast // the year's ENABLED forecasts
	Partners        []model.Partner
	Sections        []model.Section // ACTIVE sections, in display order
	Memberships     []model.PartnerSection
	SubtypeCategory map[string]model.ExpenseCategory
	CurrentLimit    model.Money
	InvestmentLimit model.Money
}

// Compute runs the allocation waterfall per category and returns the full ReportData.
func Compute(in AllocationInput) (report.ReportData, error) {
	if in.SubtypeCategory == nil {
		return report.ReportData{}, fmt.Errorf("SubtypeCategory must not be nil")
	}
	partnerByID := map[int]model.Partner{}
	for _, p := range in.Partners {
		partnerByID[p.ID()] = p
	}

	cats := []model.ExpenseCategory{model.CategoryCurrent, model.CategoryInvestment}
	limits := []model.Money{in.CurrentLimit, in.InvestmentLimit}

	categories := make([]report.CategoryReportData, 0, 2)
	hasNegative := false
	for i, cat := range cats {
		c := computeCategory(cat, limits[i], in, partnerByID)
		if c.Sections.Remainder.Cmp(model.ZeroMoney()) < 0 {
			hasNegative = true
		}
		categories = append(categories, c)
	}
	return report.ReportData{Year: in.Year, HasNegativeRemainder: hasNegative, Categories: categories}, nil
}

func computeCategory(cat model.ExpenseCategory, limit model.Money, in AllocationInput, partnerByID map[int]model.Partner) report.CategoryReportData {
	// Filter forecasts to this category.
	var forCat []model.ExpenseForecast
	for _, f := range in.Forecasts {
		if in.SubtypeCategory[f.SubtypeCode()] == cat {
			forCat = append(forCat, f)
		}
	}

	// Common scope.
	commonF := filterCommon(forCat)
	sortByConcept(commonF)
	commonTotal := sumForecasts(commonF)
	commonItems := make([]report.DetailItem, 0, len(commonF))
	for _, f := range commonF {
		commonItems = append(commonItems, report.DetailItem{
			CpCode: f.ID(), Concept: f.Concept(), Description: f.Description(),
			RequestedAmount: f.GrossAmount(), ApprovedAmount: f.GrossAmount(), // common: approved = gross
		})
	}
	common := report.CommonData{Available: limit, Total: commonTotal, Remainder: limit.Minus(commonTotal), Items: commonItems}

	// Section scopes (N, in display order; skip empty).
	sections := append([]model.Section(nil), in.Sections...)
	sort.SliceStable(sections, func(i, j int) bool { return sections[i].DisplayOrder() < sections[j].DisplayOrder() })
	var sectionDetails []report.SectionDetail
	sectionsTotal := model.ZeroMoney()
	for _, s := range sections {
		sf := filterSection(forCat, s.Code())
		if len(sf) == 0 {
			continue
		}
		sortByConcept(sf)
		sTotal := sumForecasts(sf)
		items := make([]report.DetailItem, 0, len(sf))
		for _, f := range sf {
			items = append(items, report.DetailItem{
				CpCode: f.ID(), Concept: f.Concept(), Description: f.Description(),
				RequestedAmount: f.GrossAmount(), ApprovedAmount: f.GrossAmount(),
			})
		}
		sectionDetails = append(sectionDetails, report.SectionDetail{SectionCode: s.Code(), Label: s.Label(), Items: items, Total: sTotal})
		sectionsTotal = sectionsTotal.Plus(sTotal)
	}
	availableForSections := limit.Minus(commonTotal)
	sectionsRemainder := availableForSections.Minus(sectionsTotal)

	// Warning: added in Task 4 (nil for now).
	var warning *report.WarningData

	// Partner (Soci) scope.
	partnerF := filterPartner(forCat)
	partnerTotals := map[int]model.Money{}
	partnerNames := map[int]string{}
	var partnerOrder []int
	for _, f := range partnerF {
		if _, ok := partnerTotals[f.PartnerID()]; !ok {
			partnerOrder = append(partnerOrder, f.PartnerID())
			partnerNames[f.PartnerID()] = displayName(partnerByID, f.PartnerID())
		}
		partnerTotals[f.PartnerID()] = partnerTotals[f.PartnerID()].Plus(f.GrossAmount())
	}
	grandTotal := model.ZeroMoney()
	for _, id := range partnerOrder {
		grandTotal = grandTotal.Plus(partnerTotals[id])
	}
	hasExcess := grandTotal.Cmp(sectionsRemainder) > 0

	fair := distribute(sectionsRemainder, partnerTotals, partnerNames)
	allocByID := map[int]report.PartnerAllocation{}
	for _, a := range fair.allocations {
		allocByID[a.PartnerID] = a
	}

	partnersData := report.PartnersData{
		SubtypeTotals:  aggregateSubtypeTotals(partnerF),
		GrandTotal:     grandTotal,
		HasExcess:      hasExcess,
		FinalRemainder: fair.finalRemainder,
		Allocations:    fair.allocations,
		PartnerDetails: perPartnerDetails(partnerF, partnerByID, allocByID),
	}
	sectionsData := report.SectionsData{
		Available: availableForSections, Total: sectionsTotal, Remainder: sectionsRemainder,
		SectionDetails: sectionDetails, Partners: partnersData,
	}
	return report.CategoryReportData{Category: cat, Common: common, Sections: sectionsData, Warning: warning}
}

func aggregateSubtypeTotals(partnerF []model.ExpenseForecast) []report.SubtypeTotal {
	byCode := map[string]model.Money{}
	var order []string
	for _, f := range partnerF {
		if _, ok := byCode[f.SubtypeCode()]; !ok {
			order = append(order, f.SubtypeCode())
		}
		byCode[f.SubtypeCode()] = byCode[f.SubtypeCode()].Plus(f.GrossAmount())
	}
	out := make([]report.SubtypeTotal, 0, len(byCode))
	for _, code := range order {
		out = append(out, report.SubtypeTotal{SubtypeCode: code, Amount: byCode[code]})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SubtypeCode < out[j].SubtypeCode })
	return out
}

func perPartnerDetails(partnerF []model.ExpenseForecast, partnerByID map[int]model.Partner, allocByID map[int]report.PartnerAllocation) []report.PartnerDetail {
	byPartner := map[int][]model.ExpenseForecast{}
	var order []int
	for _, f := range partnerF {
		if _, ok := byPartner[f.PartnerID()]; !ok {
			order = append(order, f.PartnerID())
		}
		byPartner[f.PartnerID()] = append(byPartner[f.PartnerID()], f)
	}
	out := make([]report.PartnerDetail, 0, len(byPartner))
	for _, id := range order {
		pf := byPartner[id]
		sortByConcept(pf)
		alloc, hasAlloc := allocByID[id]
		total := model.ZeroMoney()
		items := make([]report.DetailItem, 0, len(pf))
		for _, f := range pf {
			approved := f.GrossAmount()
			if hasAlloc && alloc.Requested.Cmp(model.ZeroMoney()) > 0 {
				ratio := alloc.Allocated.Decimal().Div(alloc.Requested.Decimal())
				approved = f.GrossAmount().TimesRatio(ratio)
			}
			items = append(items, report.DetailItem{
				CpCode: f.ID(), Concept: f.Concept(), Description: f.Description(),
				RequestedAmount: f.GrossAmount(), ApprovedAmount: approved,
			})
			total = total.Plus(approved)
		}
		isCapped := hasAlloc && alloc.Allocated.Cmp(alloc.Requested) < 0
		maxAuth := model.ZeroMoney()
		if hasAlloc {
			maxAuth = alloc.Allocated
		}
		out = append(out, report.PartnerDetail{Name: displayName(partnerByID, id), Items: items, Total: total, IsCapped: isCapped, MaxAuthorized: maxAuth})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func displayName(partnerByID map[int]model.Partner, id int) string {
	p, ok := partnerByID[id]
	if !ok {
		return fmt.Sprintf("Unknown (%d)", id)
	}
	return fmt.Sprintf("%s (%d)", p.Name(), p.ID())
}

func filterCommon(in []model.ExpenseForecast) []model.ExpenseForecast {
	var out []model.ExpenseForecast
	for _, f := range in {
		if f.Scope().Kind() == model.ScopeCommon {
			out = append(out, f)
		}
	}
	return out
}

func filterSection(in []model.ExpenseForecast, code string) []model.ExpenseForecast {
	var out []model.ExpenseForecast
	for _, f := range in {
		if f.Scope().Kind() == model.ScopeSection && f.Scope().SectionCode() == code {
			out = append(out, f)
		}
	}
	return out
}

func filterPartner(in []model.ExpenseForecast) []model.ExpenseForecast {
	var out []model.ExpenseForecast
	for _, f := range in {
		if f.Scope().Kind() == model.ScopePartner {
			out = append(out, f)
		}
	}
	return out
}

func sortByConcept(fs []model.ExpenseForecast) {
	sort.SliceStable(fs, func(i, j int) bool { return fs[i].Concept() < fs[j].Concept() })
}

func sumForecasts(fs []model.ExpenseForecast) model.Money {
	total := model.ZeroMoney()
	for _, f := range fs {
		total = total.Plus(f.GrossAmount())
	}
	return total
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/services/ -v && go build ./...`
Expected: PASS (fair-share + compute), build clean.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/services/allocation.go internal/domain/services/allocation_test.go
git commit -m "feat(services): allocation Compute (common/sections/socis, N sections)"
```

---

### Task 4: Section-warning (N-generalized) wired into Compute

**Files:**
- Modify: `internal/domain/services/allocation.go`
- Test: `internal/domain/services/warning_test.go`

**Interfaces:**
- Consumes: `AllocationInput.Partners`, `AllocationInput.Memberships`, `model.Productor`.
- Produces: `computeWarning(...) *report.WarningData` and wires it into `computeCategory` (set on `CategoryReportData.Warning` when `sectionsRemainder < 0`).

- [ ] **Step 1: Write the failing test**

Create `internal/domain/services/warning_test.go`:
```go
package services

import (
	"testing"

	"github.com/pjover/espigol/internal/domain/model"
)

// Three sections, negative remainder triggers the warning; producer counts drive the split.
// availableForSections = 90. Producers: secA has 2, secB has 1, secC has 0 -> denominator 3.
// allowed: A = 90*2/3 = 60.00; B = 90*1/3 = 30.00; C = 0 (no producers).
func TestCompute_WarningProportionalSplit(t *testing.T) {
	secA, _ := model.NewSectionScope("a")
	secB, _ := model.NewSectionScope("b")
	secC, _ := model.NewSectionScope("c")
	// section requests exceed availableForSections (90): A 100, B 50, C 20 -> total 170.
	fA := mkForecast(t, "CP26060", 1, "100.00", secA, "a1")
	fB := mkForecast(t, "CP26061", 1, "50.00", secB, "a1")
	fC := mkForecast(t, "CP26062", 1, "20.00", secC, "a1")

	// producers: p1 in a, p2 in a, p3 in b. (p4 non-producer in a, ignored.)
	mem := []model.PartnerSection{
		mustMembership(t, 1, "a"), mustMembership(t, 2, "a"),
		mustMembership(t, 3, "b"), mustMembership(t, 4, "a"),
	}
	partners := []model.Partner{
		mkPartner(t, 1), mkPartner(t, 2), mkPartner(t, 3), mkNonProducer(t, 4),
	}
	in := AllocationInput{
		Year:            2026,
		Forecasts:       []model.ExpenseForecast{fA, fB, fC},
		Partners:        partners,
		Sections:        []model.Section{section(t, "a", "A", 1), section(t, "b", "B", 2), section(t, "c", "C", 3)},
		Memberships:     mem,
		SubtypeCategory: map[string]model.ExpenseCategory{"a1": model.CategoryCurrent},
		CurrentLimit:    model.MoneyOf(90), // common 0 -> availableForSections 90
		InvestmentLimit: model.MoneyOf(0),
	}
	rd, err := Compute(in)
	if err != nil {
		t.Fatal(err)
	}
	w := rd.Categories[0].Warning
	if w == nil {
		t.Fatal("expected a warning for negative sections remainder")
	}
	got := map[string]string{}
	prod := map[string]int{}
	for _, r := range w.Rows {
		got[r.SectionCode] = r.Allowed.String()
		prod[r.SectionCode] = r.Producers
	}
	if got["a"] != "60.00" || got["b"] != "30.00" || got["c"] != "0.00" {
		t.Errorf("allowed = %v, want a 60.00 b 30.00 c 0.00", got)
	}
	if prod["a"] != 2 || prod["b"] != 1 || prod["c"] != 0 {
		t.Errorf("producers = %v, want a 2 b 1 c 0", prod)
	}
	if !rd.HasNegativeRemainder {
		t.Errorf("HasNegativeRemainder should be true")
	}
}

func mustMembership(t *testing.T, partnerID int, code string) model.PartnerSection {
	t.Helper()
	m, err := model.NewPartnerSection(partnerID, code)
	if err != nil {
		t.Fatalf("membership: %v", err)
	}
	return m
}

func mkNonProducer(t *testing.T, id int) model.Partner {
	t.Helper()
	p, err := model.NewPartner(id, "Soci "+itoa(id), "", "", "x@x.test", "", model.Patrocinador, 0,
		modelTime(), false)
	if err != nil {
		t.Fatalf("partner: %v", err)
	}
	return p
}
```

Add this helper to the bottom of `internal/domain/services/allocation_test.go` (used by warning_test.go):
```go
func modelTime() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/services/ -run TestCompute_WarningProportionalSplit -v`
Expected: FAIL — warning is nil (not yet computed).

- [ ] **Step 3: Wire in the warning**

In `internal/domain/services/allocation.go`, replace the placeholder warning line in `computeCategory`:
```go
	// Warning: added in Task 4 (nil for now).
	var warning *report.WarningData
```
with:
```go
	// Warning (only when sections are over-budget).
	var warning *report.WarningData
	if sectionsRemainder.Cmp(model.ZeroMoney()) < 0 {
		warning = computeWarning(cat, availableForSections, sections, sectionDetails, in)
	}
```

Add `computeWarning` (and a producer-count helper) to `allocation.go`:
```go
// computeWarning splits availableForSections across the active sections in
// proportion to the number of PRODUCER members of each section. A producer who
// belongs to two sections counts in each (matching the reference).
func computeWarning(cat model.ExpenseCategory, availableForSections model.Money, sections []model.Section, sectionDetails []report.SectionDetail, in AllocationInput) *report.WarningData {
	requestedByCode := map[string]model.Money{}
	for _, sd := range sectionDetails {
		requestedByCode[sd.SectionCode] = sd.Total
	}
	producerByCode := producerCounts(in)

	denominator := 0
	for _, s := range sections {
		denominator += producerByCode[s.Code()]
	}

	rows := make([]report.SectionWarning, 0, len(sections))
	for _, s := range sections {
		n := producerByCode[s.Code()]
		allowed := model.ZeroMoney()
		if denominator > 0 {
			allowed = availableForSections.Times(n).DividedBy(denominator)
		}
		requested := requestedByCode[s.Code()]
		rows = append(rows, report.SectionWarning{
			SectionCode: s.Code(), Label: s.Label(), Producers: n,
			Allowed: allowed, Requested: requested, Adjustment: requested.Minus(allowed),
		})
	}
	return &report.WarningData{Category: cat, Rows: rows}
}

// producerCounts returns, per section code, the number of PRODUCER partners that
// are members of that section.
func producerCounts(in AllocationInput) map[string]int {
	isProducer := map[int]bool{}
	for _, p := range in.Partners {
		if p.PartnerType() == model.Productor {
			isProducer[p.ID()] = true
		}
	}
	counts := map[string]int{}
	for _, m := range in.Memberships {
		if isProducer[m.PartnerID()] {
			counts[m.SectionCode()]++
		}
	}
	return counts
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/services/ -v && go build ./...`
Expected: PASS (all services tests), build clean.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/services/allocation.go internal/domain/services/warning_test.go internal/domain/services/allocation_test.go
git commit -m "feat(services): N-section proportional warning"
```

---

### Task 5: Anonymized golden-value test

**Files:**
- Create: `internal/domain/services/golden_test.go`

**Interfaces:**
- Consumes: `Compute`, `AllocationInput`, the test helpers from Task 3 (`mkForecast`, `mkPartner`, `section`, `d`).
- Produces: a committed, self-contained golden test that asserts `Compute` reproduces the 2026 golden numbers. No new production code.

This fixture is the **anonymized golden 2026 dataset** (28 forecasts, 8 partners, 2 sections), derived from `espigol-java/private/report-examples/Previsions de despeses 2026.md`. Real numbers / partner ids / scopes are kept; partner names are `"Soci <id>"`, concepts are `"Concepte <CP>"`, descriptions empty. Subtypes are placeholders per category (`a1`=CURRENT, `b1`=INVESTMENT) — they do not affect the asserted totals. The golden 2026 data has positive section remainders, so no warning/capping fires here (those are covered by Tasks 2 & 4).

- [ ] **Step 1: Write the golden test**

Create `internal/domain/services/golden_test.go`:
```go
package services

import (
	"testing"

	"github.com/pjover/espigol/internal/domain/model"
)

func goldenInput(t *testing.T) AllocationInput {
	t.Helper()
	com := model.NewCommonScope()
	par := model.NewPartnerScope()
	oliva, _ := model.NewSectionScope("oliva")
	ram, _ := model.NewSectionScope("ramaderia")

	// CURRENT: common + sections (subtype a1).
	current := []model.ExpenseForecast{
		mkForecast(t, "CP26023", 7, "2880.00", com, "a1"),
		mkForecast(t, "CP26025", 1, "1200.00", oliva, "a1"),
		mkForecast(t, "CP26026", 1, "380.00", oliva, "a1"),
		mkForecast(t, "CP26027", 1, "4304.00", oliva, "a1"),
		mkForecast(t, "CP26028", 1, "13187.00", oliva, "a1"),
		mkForecast(t, "CP26029", 1, "650.00", oliva, "a1"),
		mkForecast(t, "CP26033", 1, "5640.00", ram, "a1"),
		mkForecast(t, "CP26034", 1, "1750.00", ram, "a1"),
	}
	// INVESTMENT: common + 1 section + socis (subtype b1).
	investment := []model.ExpenseForecast{
		mkForecast(t, "CP26024", 7, "31900.00", com, "b1"),
		mkForecast(t, "CP26054", 1, "3398.00", oliva, "b1"),
		// socis
		mkForecast(t, "CP26051", 11, "1800.00", par, "b1"),
		mkForecast(t, "CP26053", 11, "1585.00", par, "b1"),
		mkForecast(t, "CP26046", 2, "400.00", par, "b1"),
		mkForecast(t, "CP26052", 2, "3085.00", par, "b1"),
		mkForecast(t, "CP26048", 2, "1962.00", par, "b1"),
		mkForecast(t, "CP26049", 2, "3270.00", par, "b1"),
		mkForecast(t, "CP26047", 2, "450.00", par, "b1"),
		mkForecast(t, "CP26044", 5, "70.00", par, "b1"),
		mkForecast(t, "CP26041", 5, "124.00", par, "b1"),
		mkForecast(t, "CP26039", 5, "1455.00", par, "b1"),
		mkForecast(t, "CP26043", 5, "191.00", par, "b1"),
		mkForecast(t, "CP26040", 5, "760.00", par, "b1"),
		mkForecast(t, "CP26042", 5, "148.00", par, "b1"),
		mkForecast(t, "CP26045", 6, "3719.00", par, "b1"),
		mkForecast(t, "CP26035", 4, "1322.22", par, "b1"),
		mkForecast(t, "CP26036", 7, "700.00", par, "b1"),
		mkForecast(t, "CP26037", 7, "638.74", par, "b1"),
		mkForecast(t, "CP26038", 8, "1819.00", par, "b1"),
	}
	all := append(append([]model.ExpenseForecast{}, current...), investment...)

	partners := []model.Partner{}
	for _, id := range []int{1, 2, 4, 5, 6, 7, 8, 11} {
		partners = append(partners, mkPartner(t, id))
	}

	return AllocationInput{
		Year:            2026,
		Forecasts:       all,
		Partners:        partners,
		Sections:        []model.Section{section(t, "oliva", "Secció d'oliva", 1), section(t, "ramaderia", "Secció de ramaderia", 2)},
		Memberships:     nil, // no warning fires; memberships not needed
		SubtypeCategory: map[string]model.ExpenseCategory{"a1": model.CategoryCurrent, "b1": model.CategoryInvestment},
		CurrentLimit:    model.MoneyOf(30000),
		InvestmentLimit: model.MoneyOf(70000),
	}
}

func TestCompute_Golden2026(t *testing.T) {
	rd, err := Compute(goldenInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if rd.HasNegativeRemainder {
		t.Errorf("HasNegativeRemainder should be false for 2026 golden data")
	}

	cur := rd.Categories[0]
	if cur.Category != model.CategoryCurrent {
		t.Fatal("category 0 must be CURRENT")
	}
	assertMoney(t, "current common total", cur.Common.Total, "2880.00")
	assertMoney(t, "current common remainder", cur.Common.Remainder, "27120.00")
	assertMoney(t, "current sections available", cur.Sections.Available, "27120.00")
	assertMoney(t, "current sections total", cur.Sections.Total, "27111.00")
	assertMoney(t, "current sections remainder", cur.Sections.Remainder, "9.00")
	assertMoney(t, "current socis grandTotal", cur.Sections.Partners.GrandTotal, "0.00")
	assertMoney(t, "current socis finalRemainder", cur.Sections.Partners.FinalRemainder, "9.00")
	assertSectionTotal(t, cur, "oliva", "19721.00")
	assertSectionTotal(t, cur, "ramaderia", "7390.00")
	if cur.Warning != nil {
		t.Errorf("current warning should be nil")
	}

	inv := rd.Categories[1]
	if inv.Category != model.CategoryInvestment {
		t.Fatal("category 1 must be INVESTMENT")
	}
	assertMoney(t, "investment common total", inv.Common.Total, "31900.00")
	assertMoney(t, "investment common remainder", inv.Common.Remainder, "38100.00")
	assertMoney(t, "investment sections available", inv.Sections.Available, "38100.00")
	assertMoney(t, "investment sections total", inv.Sections.Total, "3398.00")
	assertMoney(t, "investment sections remainder", inv.Sections.Remainder, "34702.00")
	assertSectionTotal(t, inv, "oliva", "3398.00")
	assertMoney(t, "investment socis grandTotal", inv.Sections.Partners.GrandTotal, "23498.96")
	assertMoney(t, "investment socis finalRemainder", inv.Sections.Partners.FinalRemainder, "11203.04")
	if inv.Sections.Partners.HasExcess {
		t.Errorf("investment HasExcess should be false (23498.96 <= 34702)")
	}
	if inv.Warning != nil {
		t.Errorf("investment warning should be nil")
	}

	// Per-partner socis allocations (requested == allocated; no capping).
	wantAlloc := map[int]string{11: "3385.00", 2: "9167.00", 5: "2748.00", 6: "3719.00", 4: "1322.22", 7: "1338.74", 8: "1819.00"}
	gotAlloc := map[int]string{}
	for _, a := range inv.Sections.Partners.Allocations {
		gotAlloc[a.PartnerID] = a.Allocated.String()
	}
	for id, want := range wantAlloc {
		if gotAlloc[id] != want {
			t.Errorf("investment partner %d allocated = %q, want %q", id, gotAlloc[id], want)
		}
	}
	if len(inv.Sections.Partners.Allocations) != len(wantAlloc) {
		t.Errorf("investment allocations count = %d, want %d", len(inv.Sections.Partners.Allocations), len(wantAlloc))
	}
}

func assertMoney(t *testing.T, label string, got model.Money, want string) {
	t.Helper()
	if got.String() != want {
		t.Errorf("%s = %q, want %q", label, got.String(), want)
	}
}

func assertSectionTotal(t *testing.T, c interface {
}, code, want string) {
}
```

Note: replace the empty `assertSectionTotal` stub above with this real helper (placed in `golden_test.go`), which takes the category and finds the section by code:
```go
func assertSectionTotal(t *testing.T, c reportCategory, code, want string) {
	t.Helper()
	for _, sd := range c.Sections.SectionDetails {
		if sd.SectionCode == code {
			if sd.Total.String() != want {
				t.Errorf("section %s total = %q, want %q", code, sd.Total.String(), want)
			}
			return
		}
	}
	t.Errorf("section %s not found", code)
}
```
and add a type alias near the top of `golden_test.go` so the helper signature is concrete:
```go
import "github.com/pjover/espigol/internal/domain/model/report"

type reportCategory = report.CategoryReportData
```
(Delete the empty stub `assertSectionTotal(t, c interface{}{...})` — only the `reportCategory` version remains.)

- [ ] **Step 2: Run the golden test to verify it passes**

Run: `go test ./internal/domain/services/ -run TestCompute_Golden2026 -v`
Expected: PASS — all golden numbers match (current remainder 9,00; investment socis 23.498,96; remainder 11.203,04; per-partner allocations).

- [ ] **Step 3: Run the full suite + vet**

Run: `go vet ./... && go test ./...`
Expected: all green.

- [ ] **Step 4: Commit**

```bash
git add internal/domain/services/golden_test.go
git commit -m "test(services): anonymized golden 2026 allocation test"
```

---

## Self-Review

**Spec coverage (against the Phase 3 design):**
- §2 architecture / `AllocationInput` → Task 3.
- §3 ReportData model (generalized `SectionDetail`/`WarningData`/`SectionWarning`) → Task 1.
- §4 algorithm: per-category waterfall, common/sections/socis, ordering, approved=gross for common/section, proration when capped → Task 3; N-section warning + producer split → Task 4.
- §4.1 FairShareAllocator (no-excess, iterative cap, non-positive no-clamp, convergence 0.01, cap 100) → Task 2.
- §4.2 rounding (DividedBy HALF_UP mean, high-precision proration ratio then scale-2, Cmp) → Tasks 2 & 3.
- §5.1 synthetic unit tests → Tasks 2 (fair-share edges) & 4 (warning split, producer-in-two-sections via the `a`/`b`/`c` test, denominator-0 → allowed 0).
- §5.2 anonymized golden test (MD-sourced, real numbers, anonymized names/concepts, numbers-only assertions, no warning/capping) → Task 5.
- §6 scope (no DB/serialization/rendering) → respected; all pure.

**Placeholder scan:** No "TBD"/"implement later". Two stubs in Task 5's first code block (`assertSectionTotal`) are immediately followed by explicit replacement code with instructions to delete the stub — teaching markers, not silent placeholders. The Task 2 `util.go` double-`import` is explicitly flagged with the corrected single-import block to use.

**Type consistency:** `report.*` struct field names (Task 1) are used identically in Tasks 3–5. `distribute`/`fairShareResult` (Task 2) match their use in Task 3. `AllocationInput`/`Compute` (Task 3) match Tasks 4–5. `model.Money` and domain accessor names match the Phase-2 API listed in Global Constraints. The `computeWarning` signature added in Task 4 matches its call site edited into `computeCategory`.

**Determinism note for implementers:** all map aggregations emit sorted output (concept/name/code/display-order); the fair-share computation is set-based and order-independent, so Go's randomized map iteration cannot change results — but follow the sorted-emit pattern shown, do not range a map directly into an output slice.
