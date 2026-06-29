# Phase 5 — Reports (PDF + Markdown) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render the computed `report.ReportData` snapshot to PDF (maroto v1) and Markdown, sharing one renderer-agnostic block layout so both have identical sections/tables (incl. a final `Resum` summary), plus a `ReportExporter` that writes the stored PDF BLOB and the rendered MD to the output dir.

**Architecture:** A single `buildLayout(ReportData)` produces an ordered `[]Block`; `PDFRenderer` (implements `ports.ReportRenderer`, returns `[]byte` via maroto's in-memory `Output()`) and `MarkdownRenderer` both consume those blocks. A `ReportExporter` writes the BLOB to a `.pdf` file and the rendered MD to a `.md` file. All in `internal/adapters/report`.

**Tech Stack:** Go 1.26, `github.com/johnfercher/maroto v0.33.0` (v1, CGO-free), `github.com/shopspring/decimal` (via `model.Money`), stdlib.

## Global Constraints

- **Module:** `github.com/pjover/espigol`. Go **1.26**. CGO-free (maroto v1 verified building with `CGO_ENABLED=0`).
- **All in `internal/adapters/report`** (package `report`, beside the existing `NoopRenderer`). Adapter layer: imports domain (`model`, `model/report`, `ports`) + stdlib + maroto. **Must NOT import `internal/application`.**
- **No `float64`** in any amount path; format from `model.Money`.
- **Catalan, verbatim:** category labels `Despesa corrent` / `Despesa d'inversió`; structural labels (`Comú`, `Seccions`, `Socis`, `Total`, `Total comú`, `Total seccions`, `Total socis`, `Disponible`, `Remanent`, `Import disponible`, `Import remanent`, `Previst`, `Aprovat`, `Concepte`, `Brut`, `CP`, `Àmbit`, `Subtipus de despesa`, `Sol·licitat`, `Assignat`, `Soci`, `Secció`, `Socis productors`, `Ajust`, `Import màxim autoritzat`, `Detall per secció i soci`, `Resum`, `⚠ AVÍS`). Never translate.
- **EU currency:** `formatEuro` → `1.234,56 €` (period thousands, comma decimal, symbol after a space, always 2 decimals; negative keeps leading `-`).
- **`ports.ReportRenderer`** (Phase 4, unchanged): `Render(rd report.ReportData, generatedAt time.Time) ([]byte, error)`. `PDFRenderer` implements it.
- **maroto v1 API:** `pdf.NewMaroto(consts.Portrait, consts.A4)`; `m.Row/Col/Text/Line`; `props.Text{Top,Style,Size,Align,Color}`; `consts.Bold/Normal/Italic`; `color.Color{Red,Green,Blue}`; `m.RegisterHeader/RegisterFooter`; `m.AddPage()`; `out, err := m.Output()` → `bytes.Buffer`.
- **`report.ReportData` shape (Phase 3):** `ReportData{Year int; HasNegativeRemainder bool; Categories []CategoryReportData}`; `CategoryReportData{Category model.ExpenseCategory; Common CommonData; Sections SectionsData; Warning *WarningData}`; `CommonData{Available,Total,Remainder model.Money; Items []DetailItem}`; `SectionsData{Available,Total,Remainder model.Money; SectionDetails []SectionDetail; Partners PartnersData}`; `SectionDetail{SectionCode,Label string; Items []DetailItem; Total model.Money}`; `WarningData{Category model.ExpenseCategory; Rows []SectionWarning}`; `SectionWarning{SectionCode,Label string; Producers int; Allowed,Requested,Adjustment model.Money}`; `PartnersData{SubtypeTotals []SubtypeTotal; GrandTotal model.Money; HasExcess bool; FinalRemainder model.Money; Allocations []PartnerAllocation; PartnerDetails []PartnerDetail}`; `PartnerAllocation{PartnerID int; PartnerName string; Requested,Allocated model.Money}`; `PartnerDetail{Name string; Items []DetailItem; Total model.Money; IsCapped bool; MaxAuthorized model.Money}`; `DetailItem{CpCode,Concept,Description string; RequestedAmount,ApprovedAmount model.Money}`; `SubtypeTotal{SubtypeCode string; Amount model.Money}`.
- **`model.Money`:** `String()`, `Decimal() decimal.Decimal`, `Plus`, `Cmp`, `MoneyFromString`, `ZeroMoney`, `MoneyOf`.
- **`model.CategoryCurrent` / `model.CategoryInvestment`.**
- **Phase-3 golden fixture for tests:** `services.Compute` on the anonymized 2026 fixture. The Phase-3 golden test built that input in `internal/domain/services/golden_test.go` (function `goldenInput(t)`); it is package-private to `services`. For Phase-5 tests, build the same `report.ReportData` by constructing a **small exported test helper** OR by reconstructing a minimal equivalent input in the report test (see Task 2 — it defines a local `goldenReportData(t)` helper that calls `services.Compute` with the same anonymized inputs). Use that helper across Phase-5 tests.
- **TDD:** failing test first; commit after each green step.

---

### Task 1: EU currency + Catalan label formatting

**Files:**
- Create: `internal/adapters/report/format.go`
- Test: `internal/adapters/report/format_test.go`

**Interfaces:**
- Produces: `formatEuro(m model.Money) string`; `categoryLabel(c model.ExpenseCategory) string`.

- [ ] **Step 1: Write the failing test**

Create `internal/adapters/report/format_test.go`:
```go
package report

import (
	"testing"

	"github.com/pjover/espigol/internal/domain/model"
)

func TestFormatEuro(t *testing.T) {
	cases := map[string]string{
		"1234.56":  "1.234,56 €",
		"0":        "0,00 €",
		"-9":       "-9,00 €",
		"31900":    "31.900,00 €",
		"1322.22":  "1.322,22 €",
		"1000000":  "1.000.000,00 €",
		"-1234.5":  "-1.234,50 €",
	}
	for in, want := range cases {
		m, err := model.MoneyFromString(in)
		if err != nil {
			t.Fatalf("money %q: %v", in, err)
		}
		if got := formatEuro(m); got != want {
			t.Errorf("formatEuro(%s) = %q, want %q", in, got, want)
		}
	}
}

func TestCategoryLabel(t *testing.T) {
	if categoryLabel(model.CategoryCurrent) != "Despesa corrent" {
		t.Errorf("current label wrong")
	}
	if categoryLabel(model.CategoryInvestment) != "Despesa d'inversió" {
		t.Errorf("investment label wrong")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/report/ -run 'TestFormatEuro|TestCategoryLabel' -v`
Expected: FAIL — undefined `formatEuro`.

- [ ] **Step 3: Write the implementation**

Create `internal/adapters/report/format.go`:
```go
package report

import (
	"strings"

	"github.com/pjover/espigol/internal/domain/model"
)

// formatEuro renders Money as EU currency: "1.234,56 €" (period thousands,
// comma decimal, symbol after a space, always two decimals).
func formatEuro(m model.Money) string {
	s := m.String() // canonical "-1234.56" / "31900.00"
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	intPart, decPart := s, "00"
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, decPart = s[:i], s[i+1:]
	}

	// group the integer part with '.' every three digits
	var b strings.Builder
	n := len(intPart)
	for i := 0; i < n; i++ {
		if i > 0 && (n-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteByte(intPart[i])
	}

	out := b.String() + "," + decPart + " €"
	if neg {
		out = "-" + out
	}
	return out
}

// categoryLabel returns the Catalan label for an expense category.
func categoryLabel(c model.ExpenseCategory) string {
	switch c {
	case model.CategoryInvestment:
		return "Despesa d'inversió"
	default:
		return "Despesa corrent"
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/report/ -run 'TestFormatEuro|TestCategoryLabel' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/report/format.go internal/adapters/report/format_test.go
git commit -m "feat(report): EU currency + Catalan category label formatting"
```

---

### Task 2: Shared Block layout + buildLayout

**Files:**
- Create: `internal/adapters/report/layout.go`
- Test: `internal/adapters/report/layout_test.go`

**Interfaces:**
- Consumes: `formatEuro`, `categoryLabel`; `report.ReportData`.
- Produces:
  - block types `SectionTitle{Text string}`, `Row{Cells []string; Bold bool; Red bool}`, `Table{Title string; Headers []string; Widths []uint; Rows []Row}`, `PageBreak{}` (all implement the empty marker interface `Block`).
  - `buildLayout(rd report.ReportData, generatedAt time.Time) []Block`.
  - test helper `goldenReportData(t *testing.T) report.ReportData` (reused by Tasks 3 & 5).

- [ ] **Step 1: Write the failing test**

Create `internal/adapters/report/layout_test.go`:
```go
package report

import (
	"testing"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/services"
)

// goldenReportData computes the ReportData for the anonymized 2026 golden
// scenario (same numbers as the Phase-3 golden test, anonymized text).
func goldenReportData(t *testing.T) report.ReportDataAlias { return buildGolden(t) }

// (helper bodies are below in this file)

func tables(blocks []Block) []Table {
	var out []Table
	for _, b := range blocks {
		if tb, ok := b.(Table); ok {
			out = append(out, tb)
		}
	}
	return out
}

func TestBuildLayout_StructureAndResum(t *testing.T) {
	rd := buildGolden(t)
	blocks := buildLayout(rd, time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC))

	// both category labels appear as section titles
	titles := map[string]bool{}
	for _, b := range blocks {
		if st, ok := b.(SectionTitle); ok {
			titles[st.Text] = true
		}
	}
	if !titles["Despesa corrent"] || !titles["Despesa d'inversió"] {
		t.Errorf("missing category section titles: %v", titles)
	}
	if !titles["Resum"] {
		t.Errorf("missing final Resum section title")
	}

	// the Resum tables must carry the golden numbers, EU-formatted.
	joined := ""
	for _, tb := range tables(blocks) {
		for _, r := range tb.Rows {
			for _, c := range r.Cells {
				joined += c + "|"
			}
		}
	}
	for _, want := range []string{"2.880,00 €", "27.111,00 €", "23.498,96 €", "11.203,04 €", "9,00 €", "31.900,00 €"} {
		if !contains(joined, want) {
			t.Errorf("layout missing expected amount %q", want)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}
func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
```

Note: the awkward `goldenReportData`/`report.ReportDataAlias` line above is a stray — **delete it**. The real helper is `buildGolden(t)`. Add `buildGolden` to this test file (it is the shared golden helper; Tasks 3 and 5 reuse it from the same package's `_test.go` scope — keep it in a non-`_test`-suffixed export-free helper file if other test files need it; simplest: put `buildGolden` in `layout_test.go` and let `markdown_test.go`/`pdf_test.go` call it since they are the same package `report`). Implement `buildGolden`:
```go
func buildGolden(t *testing.T) report.ReportData {
	t.Helper()
	d := func(s string) model.Money { m, err := model.MoneyFromString(s); if err != nil { t.Fatal(err) }; return m }
	com := model.NewCommonScope()
	par := model.NewPartnerScope()
	oliva, _ := model.NewSectionScope("oliva")
	ram, _ := model.NewSectionScope("ramaderia")
	mk := func(id string, pid int, gross string, scope model.ExpenseScope, sub string) model.ExpenseForecast {
		planned := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
		f, err := model.NewExpenseForecast(id, pid, "Concepte "+id, "", d(gross), model.ZeroMoney(), nil, planned, 2026, sub, scope, planned, true)
		if err != nil { t.Fatal(err) }
		return f
	}
	forecasts := []model.ExpenseForecast{
		mk("CP26023", 7, "2880.00", com, "a1"),
		mk("CP26025", 1, "1200.00", oliva, "a1"), mk("CP26026", 1, "380.00", oliva, "a1"),
		mk("CP26027", 1, "4304.00", oliva, "a1"), mk("CP26028", 1, "13187.00", oliva, "a1"),
		mk("CP26029", 1, "650.00", oliva, "a1"),
		mk("CP26033", 1, "5640.00", ram, "a1"), mk("CP26034", 1, "1750.00", ram, "a1"),
		mk("CP26024", 7, "31900.00", com, "b1"), mk("CP26054", 1, "3398.00", oliva, "b1"),
		mk("CP26051", 11, "1800.00", par, "b1"), mk("CP26053", 11, "1585.00", par, "b1"),
		mk("CP26046", 2, "400.00", par, "b1"), mk("CP26052", 2, "3085.00", par, "b1"),
		mk("CP26048", 2, "1962.00", par, "b1"), mk("CP26049", 2, "3270.00", par, "b1"),
		mk("CP26047", 2, "450.00", par, "b1"),
		mk("CP26044", 5, "70.00", par, "b1"), mk("CP26041", 5, "124.00", par, "b1"),
		mk("CP26039", 5, "1455.00", par, "b1"), mk("CP26043", 5, "191.00", par, "b1"),
		mk("CP26040", 5, "760.00", par, "b1"), mk("CP26042", 5, "148.00", par, "b1"),
		mk("CP26045", 6, "3719.00", par, "b1"), mk("CP26035", 4, "1322.22", par, "b1"),
		mk("CP26036", 7, "700.00", par, "b1"), mk("CP26037", 7, "638.74", par, "b1"),
		mk("CP26038", 8, "1819.00", par, "b1"),
	}
	var partners []model.Partner
	for _, id := range []int{1, 2, 4, 5, 6, 7, 8, 11} {
		p, err := model.NewPartner(id, "Soci", "", "", "s@e.test", "", model.Productor, 0, time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), false)
		if err != nil { t.Fatal(err) }
		partners = append(partners, p)
	}
	sOliva, _ := model.NewSection("oliva", "Secció d'oliva", true, 1)
	sRam, _ := model.NewSection("ramaderia", "Secció de ramaderia", true, 2)
	rd, err := services.Compute(services.AllocationInput{
		Year: 2026, Forecasts: forecasts, Partners: partners,
		Sections: []model.Section{sOliva, sRam},
		SubtypeCategory: map[string]model.ExpenseCategory{"a1": model.CategoryCurrent, "b1": model.CategoryInvestment},
		CurrentLimit: model.MoneyOf(30000), InvestmentLimit: model.MoneyOf(70000),
	})
	if err != nil { t.Fatal(err) }
	return rd
}
```
(Delete the stray `goldenReportData` line; keep `buildGolden`, `tables`, `contains`, `indexOf`, and the test. You may use `strings.Contains` instead of the hand-rolled `contains`/`indexOf` — simpler; import `strings`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/report/ -run TestBuildLayout -v`
Expected: FAIL — undefined `buildLayout`, `Block`, `Table`, etc.

- [ ] **Step 3: Write the layout model + builder**

Create `internal/adapters/report/layout.go`:
```go
package report

import (
	"fmt"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/model/report"
)

// Block is one renderable unit of a report (sealed: SectionTitle | Table | PageBreak).
type Block interface{ isBlock() }

// SectionTitle is a prominent heading.
type SectionTitle struct{ Text string }

// Row is one table row; Bold renders emphasized, Red flags an over-budget/capped value.
type Row struct {
	Cells []string
	Bold  bool
	Red   bool
}

// Table is a titled table. Widths are maroto grid units (sum 12); Markdown ignores them.
type Table struct {
	Title   string
	Headers []string
	Widths  []uint
	Rows    []Row
}

// PageBreak forces a new page (PDF); Markdown renders a horizontal rule.
type PageBreak struct{}

func (SectionTitle) isBlock() {}
func (Table) isBlock()        {}
func (PageBreak) isBlock()    {}

// buildLayout walks the computed ReportData and emits the shared block sequence
// consumed identically by the PDF and Markdown renderers.
func buildLayout(rd report.ReportData, generatedAt time.Time) []Block {
	var blocks []Block
	for i, cat := range rd.Categories {
		blocks = append(blocks, categoryBlocks(rd.Year, cat)...)
		if i < len(rd.Categories)-1 {
			blocks = append(blocks, PageBreak{})
		}
	}
	blocks = append(blocks, PageBreak{}, SectionTitle{Text: "Resum"})
	for _, cat := range rd.Categories {
		blocks = append(blocks, resumTable(rd.Year, cat))
	}
	return blocks
}

func categoryBlocks(year int, cat report.CategoryReportData) []Block {
	label := categoryLabel(cat.Category)
	var blocks []Block

	// 1. Common
	commonRows := make([]Row, 0, len(cat.Common.Items)+1)
	for _, it := range cat.Common.Items {
		commonRows = append(commonRows, Row{Cells: []string{it.CpCode, it.Concept, formatEuro(it.RequestedAmount)}})
	}
	commonRows = append(commonRows, Row{Cells: []string{"", "Total comú", formatEuro(cat.Common.Total)}, Bold: true})
	blocks = append(blocks, Table{Title: label + " — Comú", Headers: []string{"CP", "Concepte", "Brut"}, Widths: []uint{2, 7, 3}, Rows: commonRows})

	// 2. Sections
	secRows := make([]Row, 0)
	for _, sd := range cat.Sections.SectionDetails {
		secRows = append(secRows, Row{Cells: []string{sd.Label, "", ""}, Bold: true})
		for _, it := range sd.Items {
			secRows = append(secRows, Row{Cells: []string{it.CpCode, it.Concept, formatEuro(it.RequestedAmount)}})
		}
		secRows = append(secRows, Row{Cells: []string{"", "Total " + sd.Label, formatEuro(sd.Total)}, Bold: true})
	}
	secRows = append(secRows, Row{Cells: []string{"", "Total seccions", formatEuro(cat.Sections.Total)}, Bold: true})
	blocks = append(blocks, Table{Title: label + " — Seccions", Headers: []string{"CP", "Concepte", "Brut"}, Widths: []uint{2, 7, 3}, Rows: secRows})

	// 3. Remainder summary
	categoryTotal := cat.Common.Total.Plus(cat.Sections.Total)
	remRows := []Row{
		{Cells: []string{fmt.Sprintf("Disponible any %d", year), formatEuro(cat.Common.Available)}},
		{Cells: []string{"Total comú", formatEuro(cat.Common.Total)}},
		{Cells: []string{"Disponible per seccions", formatEuro(cat.Sections.Available)}},
		{Cells: []string{"Total seccions", formatEuro(cat.Sections.Total)}},
		{Cells: []string{"Total " + label, formatEuro(categoryTotal)}},
		{Cells: []string{"Remanent", formatEuro(cat.Sections.Remainder)}, Bold: true},
	}
	blocks = append(blocks, Table{Title: "Remanent de " + label, Headers: []string{"", ""}, Widths: []uint{8, 4}, Rows: remRows})

	// 4. Warning (only when over budget)
	if cat.Warning != nil {
		wRows := make([]Row, 0, len(cat.Warning.Rows))
		for _, w := range cat.Warning.Rows {
			wRows = append(wRows, Row{
				Cells: []string{w.Label, fmt.Sprintf("%d", w.Producers), formatEuro(w.Allowed), formatEuro(w.Adjustment)},
				Red:   true,
			})
		}
		blocks = append(blocks, Table{Title: "⚠ AVÍS: Ajust necessari per " + label, Headers: []string{"Secció", "Socis productors", "Disponible", "Ajust"}, Widths: []uint{4, 2, 3, 3}, Rows: wRows})
	}

	// 5. Partners (subtype totals + adjustment or final remainder)
	pdata := cat.Sections.Partners
	pRows := make([]Row, 0, len(pdata.SubtypeTotals)+2)
	for _, st := range pdata.SubtypeTotals {
		pRows = append(pRows, Row{Cells: []string{st.SubtypeCode, formatEuro(st.Amount)}})
	}
	pRows = append(pRows, Row{Cells: []string{"Total socis", formatEuro(pdata.GrandTotal)}, Bold: true})
	if !pdata.HasExcess {
		pRows = append(pRows, Row{Cells: []string{"Remanent final", formatEuro(pdata.FinalRemainder)}, Bold: true})
	}
	blocks = append(blocks, Table{Title: label + " — Socis", Headers: []string{"Subtipus de despesa", "Brut"}, Widths: []uint{8, 4}, Rows: pRows})

	if pdata.HasExcess {
		adjRows := make([]Row, 0, len(pdata.Allocations)+1)
		for _, a := range pdata.Allocations {
			adjRows = append(adjRows, Row{
				Cells: []string{a.PartnerName, formatEuro(a.Requested), formatEuro(a.Allocated)},
				Red:   a.Allocated.Cmp(a.Requested) < 0,
			})
		}
		blocks = append(blocks, Table{Title: "Ajust de despeses per soci (" + label + ")", Headers: []string{"Soci", "Sol·licitat", "Assignat"}, Widths: []uint{5, 4, 3}, Rows: adjRows})
	}

	// 6. Detail per scope and per partner
	blocks = append(blocks, SectionTitle{Text: "Detall per secció i soci — " + label})
	// common detail
	if len(cat.Common.Items) > 0 {
		blocks = append(blocks, detailTable("Comú", cat.Common.Items))
	}
	for _, sd := range cat.Sections.SectionDetails {
		blocks = append(blocks, detailTable(sd.Label, sd.Items))
	}
	for _, pd := range pdata.PartnerDetails {
		rows := make([]Row, 0, len(pd.Items)+2)
		for _, it := range pd.Items {
			rows = append(rows, Row{Cells: []string{it.CpCode, it.Concept, formatEuro(it.ApprovedAmount)}})
		}
		rows = append(rows, Row{Cells: []string{"", "Total", formatEuro(pd.Total)}, Bold: true})
		if pd.IsCapped {
			rows = append(rows, Row{Cells: []string{"", "Import màxim autoritzat", formatEuro(pd.MaxAuthorized)}, Bold: true, Red: true})
		}
		blocks = append(blocks, Table{Title: pd.Name, Headers: []string{"CP", "Concepte", "Brut"}, Widths: []uint{2, 7, 3}, Rows: rows})
	}

	return blocks
}

func detailTable(title string, items []report.DetailItem) Table {
	rows := make([]Row, 0, len(items)+1)
	total := model.ZeroMoney()
	for _, it := range items {
		rows = append(rows, Row{Cells: []string{it.CpCode, it.Concept, formatEuro(it.RequestedAmount)}})
		total = total.Plus(it.RequestedAmount)
	}
	rows = append(rows, Row{Cells: []string{"", "Total", formatEuro(total)}, Bold: true})
	return Table{Title: title, Headers: []string{"CP", "Concepte", "Brut"}, Widths: []uint{2, 7, 3}, Rows: rows}
}

func resumTable(year int, cat report.CategoryReportData) Table {
	label := categoryLabel(cat.Category)
	limit := cat.Common.Available
	socisApproved := model.ZeroMoney()
	for _, a := range cat.Sections.Partners.Allocations {
		socisApproved = socisApproved.Plus(a.Allocated)
	}
	socisRequested := cat.Sections.Partners.GrandTotal
	totalReq := cat.Common.Total.Plus(cat.Sections.Total).Plus(socisRequested)
	totalApp := cat.Common.Total.Plus(cat.Sections.Total).Plus(socisApproved)
	remReq := subtract(limit, totalReq)
	remApp := subtract(limit, totalApp)
	rows := []Row{
		{Cells: []string{"Import disponible", formatEuro(limit), formatEuro(limit)}, Bold: true},
		{Cells: []string{"Comú", formatEuro(cat.Common.Total), formatEuro(cat.Common.Total)}},
		{Cells: []string{"Seccions", formatEuro(cat.Sections.Total), formatEuro(cat.Sections.Total)}},
		{Cells: []string{"Socis", formatEuro(socisRequested), formatEuro(socisApproved)}},
		{Cells: []string{"Total", formatEuro(totalReq), formatEuro(totalApp)}, Bold: true},
		{Cells: []string{"Import remanent", formatEuro(remReq), formatEuro(remApp)}, Bold: true},
	}
	return Table{Title: label, Headers: []string{"Concepte", "Previst", "Aprovat"}, Widths: []uint{6, 3, 3}, Rows: rows}
}

// subtract returns a - b using Money arithmetic (a.Plus of b negated via decimal).
func subtract(a, b model.Money) model.Money {
	m, _ := model.MoneyFromString(a.Decimal().Sub(b.Decimal()).StringFixed(2))
	return m
}
```
Note: `model.Money` has no exported `Minus` available outside the domain? It does (`Minus` is exported on the value type) — prefer `a.Minus(b)` if available. Check `internal/domain/model/money.go`: if `Minus` exists, replace the `subtract` helper with `a.Minus(b)` everywhere and delete `subtract`. (Phase-2 Money has `Minus`.) Use `cat.Common.Total.Plus(...)` and `limit.Minus(totalReq)` directly.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/report/ -run TestBuildLayout -v`
Expected: PASS (category titles, Resum title, golden amounts present).

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/report/layout.go internal/adapters/report/layout_test.go
git commit -m "feat(report): shared block layout from ReportData (incl. final Resum)"
```

---

### Task 3: MarkdownRenderer

**Files:**
- Create: `internal/adapters/report/markdown_renderer.go`
- Test: `internal/adapters/report/markdown_test.go`

**Interfaces:**
- Consumes: `buildLayout`, the block types, `buildGolden` (test helper from Task 2).
- Produces: `type MarkdownRenderer struct{}`; `(MarkdownRenderer) Render(rd report.ReportData) []byte`.

- [ ] **Step 1: Write the failing test**

Create `internal/adapters/report/markdown_test.go`:
```go
package report

import (
	"strings"
	"testing"

	"github.com/pjover/espigol/internal/domain/model/report"
)

func TestMarkdownRenderer_StructureAndNumbers(t *testing.T) {
	rd := buildGolden(t)
	md := string(MarkdownRenderer{}.Render(rd))

	if !strings.HasPrefix(md, "# Previsions de despeses 2026") {
		t.Errorf("missing H1 title; got start: %.40q", md)
	}
	for _, want := range []string{
		"## Resum",
		"### Despesa corrent",
		"### Despesa d'inversió",
		"| CP | Concepte | Brut |", // a detail/common table header
		"2.880,00 €", "27.111,00 €", "23.498,96 €", "11.203,04 €",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q", want)
		}
	}
	// well-formed table: a header separator row exists
	if !strings.Contains(md, "| --- |") && !strings.Contains(md, "|---|") {
		t.Errorf("no markdown table separator found")
	}
}

var _ = report.ReportData{} // ensure import used
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/report/ -run TestMarkdownRenderer -v`
Expected: FAIL — undefined `MarkdownRenderer`.

- [ ] **Step 3: Write the renderer**

Create `internal/adapters/report/markdown_renderer.go`:
```go
package report

import (
	"fmt"
	"strings"
	"time"

	"github.com/pjover/espigol/internal/domain/model/report"
)

// MarkdownRenderer renders ReportData to Markdown using the shared block layout,
// so its sections and tables match the PDF exactly.
type MarkdownRenderer struct{}

// Render returns the Markdown document for rd.
func (MarkdownRenderer) Render(rd report.ReportData) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# Previsions de despeses %d\n\n", rd.Year)
	for _, blk := range buildLayout(rd, time.Time{}) {
		switch v := blk.(type) {
		case SectionTitle:
			fmt.Fprintf(&b, "## %s\n\n", v.Text)
		case PageBreak:
			b.WriteString("---\n\n")
		case Table:
			writeMarkdownTable(&b, v)
		}
	}
	return []byte(b.String())
}

func writeMarkdownTable(b *strings.Builder, t Table) {
	if t.Title != "" {
		fmt.Fprintf(b, "### %s\n\n", t.Title)
	}
	headers := t.Headers
	// drop fully-empty header sets (e.g. the remainder summary) into a 2-col blank header
	fmt.Fprintf(b, "| %s |\n", strings.Join(headers, " | "))
	sep := make([]string, len(headers))
	for i := range sep {
		sep[i] = "---"
	}
	fmt.Fprintf(b, "| %s |\n", strings.Join(sep, " | "))
	for _, row := range t.Rows {
		cells := make([]string, len(row.Cells))
		for i, c := range row.Cells {
			cell := escapePipes(c)
			if (row.Bold || row.Red) && cell != "" {
				cell = "**" + cell + "**"
			}
			cells[i] = cell
		}
		fmt.Fprintf(b, "| %s |\n", strings.Join(cells, " | "))
	}
	b.WriteString("\n")
}

func escapePipes(s string) string { return strings.ReplaceAll(s, "|", "\\|") }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/report/ -run TestMarkdownRenderer -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/report/markdown_renderer.go internal/adapters/report/markdown_test.go
git commit -m "feat(report): Markdown renderer over the shared block layout"
```

---

### Task 4: maroto PDF scaffolding ([]byte output)

**Files:**
- Create: `internal/adapters/report/pdf_doc.go`
- Test: `internal/adapters/report/pdf_doc_test.go`

**Interfaces:**
- Consumes: maroto v1; the block types.
- Produces: `renderDocument(title, footer, businessName, logoPath string, blocks []Block) ([]byte, error)` — builds a maroto A4 portrait doc (logo+business-name header, italic footer, centered title, then each block) and returns PDF bytes via `m.Output()`.

- [ ] **Step 1: Add the maroto dependency**

Run:
```bash
go get github.com/johnfercher/maroto@v0.33.0
go mod tidy
```
Expected: `go.mod`/`go.sum` updated; module still builds.

- [ ] **Step 2: Write the failing test**

Create `internal/adapters/report/pdf_doc_test.go`:
```go
package report

import (
	"bytes"
	"testing"
)

func TestRenderDocument_ProducesPDFBytes(t *testing.T) {
	blocks := []Block{
		SectionTitle{Text: "Secció"},
		Table{Title: "Taula", Headers: []string{"A", "B"}, Widths: []uint{8, 4}, Rows: []Row{
			{Cells: []string{"x", "1,00 €"}},
			{Cells: []string{"Total", "1,00 €"}, Bold: true},
			{Cells: []string{"Ajust", "-1,00 €"}, Red: true},
		}},
		PageBreak{},
	}
	out, err := renderDocument("Títol", "29/06/2026", "Cooperativa d'Estellencs", "/nonexistent/logo.png", blocks)
	if err != nil {
		t.Fatalf("renderDocument: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("empty PDF output")
	}
	if !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Errorf("output is not a PDF (no %%PDF header): %.8q", out)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/adapters/report/ -run TestRenderDocument -v`
Expected: FAIL — undefined `renderDocument`.

- [ ] **Step 4: Write the scaffolding**

Create `internal/adapters/report/pdf_doc.go`. Adapt espigol-cmd's `report_pdf.go` to render blocks to `[]byte`:
```go
package report

import (
	"fmt"
	"os"

	"github.com/johnfercher/maroto/pkg/color"
	"github.com/johnfercher/maroto/pkg/consts"
	"github.com/johnfercher/maroto/pkg/pdf"
	"github.com/johnfercher/maroto/pkg/props"
)

var headingColor = color.Color{Red: 0, Green: 51, Blue: 51}
var redColor = color.Color{Red: 200, Green: 0, Blue: 0}

// renderDocument builds an A4 portrait PDF from the blocks and returns its bytes.
func renderDocument(title, footer, businessName, logoPath string, blocks []Block) ([]byte, error) {
	m := pdf.NewMaroto(consts.Portrait, consts.A4)
	m.SetPageMargins(15, 10, 15)

	registerHeader(m, businessName, logoPath)
	registerFooter(m, footer)
	docTitle(m, title)

	for _, blk := range blocks {
		switch v := blk.(type) {
		case SectionTitle:
			sectionTitle(m, v.Text)
		case PageBreak:
			m.AddPage()
		case Table:
			renderTable(m, v)
		}
	}

	out, err := m.Output()
	if err != nil {
		return nil, fmt.Errorf("rendering PDF: %w", err)
	}
	return out.Bytes(), nil
}

func registerHeader(m pdf.Maroto, businessName, logoPath string) {
	hasLogo := false
	if logoPath != "" {
		if _, err := os.Stat(logoPath); err == nil {
			hasLogo = true
		}
	}
	m.RegisterHeader(func() {
		m.Row(20, func() {
			if hasLogo {
				m.Col(3, func() { _ = m.FileImage(logoPath, props.Rect{Left: 2, Center: true, Percent: 80}) })
				m.ColSpace(5)
				m.Col(4, func() {
					m.Text(businessName, props.Text{Style: consts.BoldItalic, Size: 10, Align: consts.Left})
				})
			} else {
				m.Col(12, func() {
					m.Text(businessName, props.Text{Style: consts.BoldItalic, Size: 10, Align: consts.Left})
				})
			}
		})
	})
}

func registerFooter(m pdf.Maroto, footer string) {
	if footer == "" {
		return
	}
	m.RegisterFooter(func() {
		m.Row(4, func() {
			m.Col(12, func() {
				m.Text(footer, props.Text{Top: 4, Style: consts.Italic, Size: 8, Align: consts.Right})
			})
		})
	})
}

func docTitle(m pdf.Maroto, title string) {
	m.Row(20, func() {
		m.Col(12, func() {
			m.Text(title, props.Text{Top: 4, Style: consts.Bold, Align: consts.Center, Color: headingColor, Size: 18})
		})
	})
}

func sectionTitle(m pdf.Maroto, text string) {
	m.Row(14, func() {
		m.Col(12, func() {
			m.Text(text, props.Text{Top: 6, Style: consts.Bold, Align: consts.Left, Color: headingColor, Size: 13})
		})
	})
}

func renderTable(m pdf.Maroto, t Table) {
	if t.Title != "" {
		m.Row(12, func() {
			m.Col(12, func() {
				m.Text(t.Title, props.Text{Top: 4, Style: consts.Bold, Align: consts.Left, Color: headingColor, Size: 11})
			})
		})
	}
	// header
	if hasNonEmpty(t.Headers) {
		m.Row(6, func() {
			for i, h := range t.Headers {
				w := t.Widths[i]
				m.Col(w, func() {
					m.Text(h, props.Text{Top: 1, Style: consts.Bold, Size: 9, Align: consts.Left})
				})
			}
		})
	}
	// rows
	for _, row := range t.Rows {
		rowCopy := row
		m.Row(6, func() {
			style := consts.Normal
			if rowCopy.Bold {
				style = consts.Bold
			}
			for i, cell := range rowCopy.Cells {
				w := t.Widths[i]
				align := consts.Left
				if i == len(rowCopy.Cells)-1 {
					align = consts.Right
				}
				cellText := cell
				m.Col(w, func() {
					tp := props.Text{Top: 1, Style: style, Size: 9, Align: align}
					if rowCopy.Red {
						tp.Color = redColor
					}
					m.Text(cellText, tp)
				})
			}
		})
	}
}

func hasNonEmpty(ss []string) bool {
	for _, s := range ss {
		if s != "" {
			return true
		}
	}
	return false
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/adapters/report/ -run TestRenderDocument -v && go build ./...`
Expected: PASS, build clean.

- [ ] **Step 6: Commit**

```bash
git add internal/adapters/report/pdf_doc.go internal/adapters/report/pdf_doc_test.go go.mod go.sum
git commit -m "feat(report): maroto v1 PDF scaffolding rendering blocks to []byte"
```

---

### Task 5: PDFRenderer (implements ports.ReportRenderer)

**Files:**
- Create: `internal/adapters/report/pdf_renderer.go`
- Test: `internal/adapters/report/pdf_renderer_test.go`

**Interfaces:**
- Consumes: `buildLayout`, `renderDocument`, `buildGolden` (test helper), `ports.ReportRenderer`.
- Produces: `type PDFRenderer struct { BusinessName, LogoPath string }`; `(PDFRenderer) Render(rd report.ReportData, generatedAt time.Time) ([]byte, error)`; satisfies `ports.ReportRenderer`.

- [ ] **Step 1: Write the failing test**

Create `internal/adapters/report/pdf_renderer_test.go`:
```go
package report

import (
	"bytes"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/domain/ports"
)

func TestPDFRenderer_SmokeOnGolden(t *testing.T) {
	var r ports.ReportRenderer = PDFRenderer{BusinessName: "Cooperativa d'Estellencs", LogoPath: ""}
	out, err := r.Render(buildGolden(t), time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if len(out) == 0 || !bytes.HasPrefix(out, []byte("%PDF")) {
		t.Errorf("expected non-empty PDF bytes, got %d bytes prefix %.8q", len(out), out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/report/ -run TestPDFRenderer -v`
Expected: FAIL — undefined `PDFRenderer`.

- [ ] **Step 3: Write the renderer**

Create `internal/adapters/report/pdf_renderer.go`:
```go
package report

import (
	"fmt"
	"time"

	"github.com/pjover/espigol/internal/domain/model/report"
	"github.com/pjover/espigol/internal/domain/ports"
)

// PDFRenderer renders ReportData to a PDF using the shared block layout and the
// maroto scaffolding. It implements ports.ReportRenderer (used by WindowService.Close
// to store the Report.pdf BLOB).
type PDFRenderer struct {
	BusinessName string
	LogoPath     string
}

// Render returns the PDF bytes for rd.
func (r PDFRenderer) Render(rd report.ReportData, generatedAt time.Time) ([]byte, error) {
	title := fmt.Sprintf("Previsions de despeses %d", rd.Year)
	footer := generatedAt.Format("02/01/2006")
	return renderDocument(title, footer, r.BusinessName, r.LogoPath, buildLayout(rd, generatedAt))
}

var _ ports.ReportRenderer = PDFRenderer{}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/report/ -run TestPDFRenderer -v && go build ./...`
Expected: PASS, build clean (interface assertion holds).

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/report/pdf_renderer.go internal/adapters/report/pdf_renderer_test.go
git commit -m "feat(report): PDFRenderer implementing ports.ReportRenderer"
```

---

### Task 6: ReportExporter

**Files:**
- Create: `internal/adapters/report/exporter.go`
- Test: `internal/adapters/report/exporter_test.go`

**Interfaces:**
- Consumes: `MarkdownRenderer`, `model.Report`, `report.ReportData` (via `encoding/json`).
- Produces: `type ReportExporter struct { md MarkdownRenderer }`; `NewReportExporter() ReportExporter`; `(ReportExporter) Export(rep model.Report, outputDir string) error`.

- [ ] **Step 1: Write the failing test**

Create `internal/adapters/report/exporter_test.go`:
```go
package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
)

func TestReportExporter_WritesPdfAndMd(t *testing.T) {
	rd := buildGolden(t)
	snapshot, err := json.Marshal(rd)
	if err != nil {
		t.Fatal(err)
	}
	pdfBytes := []byte("%PDF-1.7 fake")
	rep, err := model.NewReport(1, 2026, time.Now().UTC(), string(snapshot), pdfBytes, nil)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := NewReportExporter().Export(rep, dir); err != nil {
		t.Fatalf("export: %v", err)
	}

	pdfPath := filepath.Join(dir, "Previsions de despeses 2026.pdf")
	mdPath := filepath.Join(dir, "Previsions de despeses 2026.md")
	gotPdf, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("pdf not written: %v", err)
	}
	if string(gotPdf) != string(pdfBytes) {
		t.Errorf("pdf file bytes != BLOB")
	}
	gotMd, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("md not written: %v", err)
	}
	if !containsString(string(gotMd), "# Previsions de despeses 2026") || !containsString(string(gotMd), "23.498,96 €") {
		t.Errorf("md content unexpected")
	}
}

func containsString(h, n string) bool { return indexOf(h, n) >= 0 }
```
(If Task 2 used `strings.Contains` instead of the local `indexOf`, use `strings.Contains` here too and drop `containsString`/`indexOf`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/report/ -run TestReportExporter -v`
Expected: FAIL — undefined `NewReportExporter`.

- [ ] **Step 3: Write the exporter**

Create `internal/adapters/report/exporter.go`:
```go
package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pjover/espigol/internal/domain/model"
	domreport "github.com/pjover/espigol/internal/domain/model/report"
)

// ReportExporter writes a stored Report's PDF BLOB and a freshly rendered
// Markdown document to the output directory. It does NOT re-render the PDF, so
// the .pdf file is byte-identical to the stored BLOB.
type ReportExporter struct {
	md MarkdownRenderer
}

func NewReportExporter() ReportExporter { return ReportExporter{} }

// Export writes "Previsions de despeses <year>.pdf" (the BLOB) and ".md"
// (rendered from the snapshot) into outputDir.
func (e ReportExporter) Export(rep model.Report, outputDir string) error {
	dir := expandTilde(outputDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating output dir %q: %w", dir, err)
	}

	base := fmt.Sprintf("Previsions de despeses %d", rep.Year())
	pdfPath := filepath.Join(dir, base+".pdf")
	if err := os.WriteFile(pdfPath, rep.Pdf(), 0o644); err != nil {
		return fmt.Errorf("writing %q: %w", pdfPath, err)
	}

	var rd domreport.ReportData
	if err := json.Unmarshal([]byte(rep.SnapshotJSON()), &rd); err != nil {
		return fmt.Errorf("decoding report snapshot: %w", err)
	}
	mdPath := filepath.Join(dir, base+".md")
	if err := os.WriteFile(mdPath, e.md.Render(rd), 0o644); err != nil {
		return fmt.Errorf("writing %q: %w", mdPath, err)
	}
	return nil
}

func expandTilde(p string) string {
	if len(p) < 2 || p[:2] != "~/" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, p[2:])
}
```

- [ ] **Step 4: Run tests + full suite**

Run: `go test ./internal/adapters/report/ -v && go vet ./... && go test ./...`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/report/exporter.go internal/adapters/report/exporter_test.go
git commit -m "feat(report): ReportExporter writes PDF BLOB + rendered MD to output dir"
```

---

## Self-Review

**Spec coverage (against the Phase 5 design):**
- §2.1 Block model + buildLayout → Task 2.
- §3 report structure (per-category tables 1–6 + final Resum) → Task 2 (`categoryBlocks`, `resumTable`, `detailTable`).
- §4.1 PDFRenderer (maroto, []byte, ports.ReportRenderer) → Tasks 4 (scaffolding) + 5 (renderer).
- §4.2 MarkdownRenderer (same blocks) → Task 3.
- §4.3 ReportExporter (BLOB→.pdf, snapshot→MD→.md, in-adapter json) → Task 6.
- formatEuro + Catalan labels → Task 1.
- §5 tests (formatEuro; buildLayout on anonymized golden incl. Resum numbers; MD structure+numbers; PDF smoke %PDF; exporter writes both, pdf==BLOB) → Tasks 1–6.
- §6 scope (no Close/TUI wiring; no-op stays) → respected; no change to WindowService.

**Placeholder scan:** No "TBD". The Task 2 test contains an explicitly-flagged stray line (`goldenReportData`/`ReportDataAlias`) with a delete instruction; the `subtract` helper has a flagged "use `Money.Minus` if available" instruction (Phase-2 Money has `Minus`, so the implementer replaces `subtract(a,b)` with `a.Minus(b)` and deletes the helper). These are reconciliation notes, not silent gaps.

**Type consistency:** `Block`/`Table`/`Row`/`SectionTitle`/`PageBreak` are defined in Task 2 and consumed identically in Tasks 3–5. `buildGolden` (Task 2 test helper, package `report`) is reused by Tasks 3 & 5 tests (same package). `report.ReportData` field traversal matches the Phase-3 shape in Global Constraints. `PDFRenderer.Render` signature matches `ports.ReportRenderer`. `model.Report` accessors (`Year()`, `Pdf()`, `SnapshotJSON()`) match Phase 2. `model.Money.Minus`/`Plus`/`Cmp`/`Decimal` per Phase 2.

**Adapter purity:** `internal/adapters/report` imports only domain (`model`, `model/report`, `ports`) + stdlib + maroto — never `internal/application` (the exporter deserializes the snapshot with `encoding/json` directly). Confirmed across all files.

**Determinism:** rendering order follows `ReportData`'s already-sorted slices (Phase 3 sorts items/allocations/subtypes); no map iteration in the renderers.
