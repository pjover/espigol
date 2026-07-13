# Consorci Project Reports Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Generate two Markdown documents from a year's enabled forecasts — a "Projecte d'actuació" narrative skeleton and an "F1-Pressupost" budget — for the Consorci subsidy application, with every line carrying its CP code(s).

**Architecture:** One pure domain computation (`services.ComputeProjecte` → `services.ProjecteData`) groups enabled forecasts by tipus → apartat → concept (merged by name, summed, CP codes collected). Two report renderers consume that structure; an exporter writes both `.md` files. A thin application service gathers the DB inputs; a new Admin `p` key triggers it. Mirrors the existing reconciliation-report pattern.

**Tech Stack:** Go, hexagonal layout; `shopspring/decimal`-backed `model.Money`; Bubble Tea TUI; standard `testing`.

## Global Constraints

- **Money**: never `float64`; use `model.Money` (sum via `.Plus`, zero via `model.ZeroMoney()`). Currency format `1.234,56 €` via the existing `formatEuro` (report package).
- **All user-facing text is Catalan.**
- **Domain imports nothing from `adapters/`.** `services.ComputeProjecte` is pure (no I/O).
- **Source data**: only `Enabled()` forecasts for the selected year; all scopes summed together.
- **Ordering**: tipus CURRENT (A) before INVESTMENT (B); apartats by subtype code ascending (`a2,a3,a4,a6,b1,b2`); concepts alphabetical by name within an apartat; CP codes ascending within a concept.
- **Label formats**: apartat `a2` → `a.2. <subtype label>`; tipus `A` → `A. <type label>`.
- **Filenames** (reports output dir): `Projecte d'actuació <year>.md`, `Pressupost del projecte d'actuació <year>.md`.
- Run the full suite with `go test ./...`; build with `make build`; format is applied by `go fmt` in `make build`.

---

### Task 1: Domain — `ProjecteData` + `ComputeProjecte`

**Files:**
- Create: `internal/domain/services/projecte.go`
- Test: `internal/domain/services/projecte_test.go`

**Interfaces:**
- Consumes: `model.ExpenseForecast` (`.Enabled()`, `.SubtypeCode()`, `.Concept()`, `.GrossAmount()`, `.ID()`), `model.ExpenseType` (`.Code()`, `.Label()`, `.Category()`), `model.ExpenseSubtype` (`.Code()`, `.Label()`, `.TypeCode()`), `model.Money`, `model.ExpenseCategory` (`model.CategoryCurrent`, `model.CategoryInvestment`).
- Produces: types `services.ProjecteData`, `services.TipusProjecte`, `services.ApartatProjecte`, `services.ConcepteProjecte`, `services.ProjecteInput`; function `services.ComputeProjecte(in ProjecteInput) ProjecteData`.

- [ ] **Step 1: Write the failing test**

Create `internal/domain/services/projecte_test.go`:

```go
package services

import (
	"testing"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
)

var projTime = time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)

// pf builds an enabled/disabled ExpenseForecast for the projecte tests, letting
// each test set the id, concept, subtype, amount and enabled flag directly.
func pf(t *testing.T, id, concept, subtype, gross string, enabled bool) model.ExpenseForecast {
	t.Helper()
	p, err := model.NewPartner(1, "Soci", "Soci", "", "", "s@e.test", "", model.Productor, 0, projTime, false)
	if err != nil {
		t.Fatal(err)
	}
	g, err := model.MoneyFromString(gross)
	if err != nil {
		t.Fatal(err)
	}
	f, err := model.NewExpenseForecast(id, p, concept, "", g, model.ZeroMoney(), nil,
		projTime, 2025, subtype, model.NewCommonScope(), projTime, enabled)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func projFixture(t *testing.T) ProjecteData {
	t.Helper()
	tA, _ := model.NewExpenseType(2025, "A", "Despeses corrents", model.CategoryCurrent)
	tB, _ := model.NewExpenseType(2025, "B", "Despeses d'inversió", model.CategoryInvestment)
	sa2, _ := model.NewExpenseSubtype(2025, "a2", "Activitats d'informació i promoció", "A")
	sa6, _ := model.NewExpenseSubtype(2025, "a6", "Despeses de fertilitzants", "A")
	sb1, _ := model.NewExpenseSubtype(2025, "b1", "Despeses d'adquisició de maquinària i materials", "B")

	forecasts := []model.ExpenseForecast{
		pf(t, "CP25001", "Projecte de disseny i comunicació", "a2", "8189.00", true),
		pf(t, "CP25005", "Adob foliar", "a6", "1488.73", true),
		pf(t, "CP25007", "Adob orgànic", "a6", "7300.00", true),
		pf(t, "CP25006", "Adob orgànic", "a6", "6580.00", true),
		pf(t, "CP25028", "Carretilla transportadora", "b1", "6610.74", true),
		pf(t, "CP25099", "Exclosa", "a2", "100.00", false), // disabled → excluded
	}
	return ComputeProjecte(ProjecteInput{
		Year:      2025,
		Forecasts: forecasts,
		Types:     []model.ExpenseType{tB, tA}, // deliberately out of order
		Subtypes:  []model.ExpenseSubtype{sb1, sa6, sa2},
	})
}

func TestComputeProjecte_GroupingOrderingAndTotals(t *testing.T) {
	d := projFixture(t)

	if d.Year != 2025 {
		t.Errorf("Year = %d, want 2025", d.Year)
	}
	if d.Total.String() != "30168.47" {
		t.Errorf("grand total = %s, want 30168.47", d.Total.String())
	}
	if len(d.Tipus) != 2 {
		t.Fatalf("len(Tipus) = %d, want 2", len(d.Tipus))
	}

	// Tipus[0] = A (CURRENT) before B (INVESTMENT).
	a := d.Tipus[0]
	if a.Code != "A" || a.Label != "Despeses corrents" || a.Category != model.CategoryCurrent {
		t.Errorf("Tipus[0] = %+v, want A/Despeses corrents/CURRENT", a)
	}
	if a.Total.String() != "23557.73" {
		t.Errorf("Tipus A total = %s, want 23557.73", a.Total.String())
	}
	if len(a.Apartats) != 2 || a.Apartats[0].Code != "a2" || a.Apartats[1].Code != "a6" {
		t.Fatalf("Tipus A apartats = %+v, want [a2 a6]", a.Apartats)
	}
	a6 := a.Apartats[1]
	if a6.Label != "Despeses de fertilitzants" || a6.Total.String() != "15368.73" {
		t.Errorf("a6 = %+v, want label/total 'Despeses de fertilitzants'/15368.73", a6)
	}
	if len(a6.Concepts) != 2 {
		t.Fatalf("a6 concepts = %d, want 2", len(a6.Concepts))
	}
	// Concepts alphabetical: "Adob foliar" before "Adob orgànic".
	if a6.Concepts[0].Name != "Adob foliar" || a6.Concepts[0].Total.String() != "1488.73" {
		t.Errorf("a6.Concepts[0] = %+v, want Adob foliar/1488.73", a6.Concepts[0])
	}
	org := a6.Concepts[1]
	if org.Name != "Adob orgànic" || org.Total.String() != "13880.00" {
		t.Errorf("a6.Concepts[1] = %+v, want Adob orgànic/13880.00", org)
	}
	// Merged CPs, sorted ascending.
	if len(org.CPs) != 2 || org.CPs[0] != "CP25006" || org.CPs[1] != "CP25007" {
		t.Errorf("Adob orgànic CPs = %v, want [CP25006 CP25007]", org.CPs)
	}

	// Tipus[1] = B.
	b := d.Tipus[1]
	if b.Code != "B" || b.Category != model.CategoryInvestment || b.Total.String() != "6610.74" {
		t.Errorf("Tipus[1] = %+v, want B/INVESTMENT/6610.74", b)
	}
	if len(b.Apartats) != 1 || b.Apartats[0].Code != "b1" || len(b.Apartats[0].Concepts) != 1 {
		t.Fatalf("Tipus B apartats = %+v, want one b1 with one concept", b.Apartats)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/services/ -run TestComputeProjecte -v`
Expected: FAIL — `undefined: ComputeProjecte` / `ProjecteInput` (compile error).

- [ ] **Step 3: Write minimal implementation**

Create `internal/domain/services/projecte.go`:

```go
package services

import (
	"sort"

	"github.com/pjover/espigol/internal/domain/model"
)

// ProjecteData is the grouped view of a year's enabled forecasts shared by both
// Consorci documents: Tipus (A/B) -> Apartat (subtype) -> Concept (forecasts
// merged by concept name, summed, with the contributing CP codes).
type ProjecteData struct {
	Year  int
	Tipus []TipusProjecte
	Total model.Money
}

type TipusProjecte struct {
	Code     string
	Label    string
	Category model.ExpenseCategory
	Apartats []ApartatProjecte
	Total    model.Money
}

type ApartatProjecte struct {
	Code     string
	Label    string
	Concepts []ConcepteProjecte
	Total    model.Money
}

type ConcepteProjecte struct {
	Name  string
	CPs   []string
	Total model.Money
}

// ProjecteInput is everything ComputeProjecte needs (assembled by the caller).
type ProjecteInput struct {
	Year      int
	Forecasts []model.ExpenseForecast
	Types     []model.ExpenseType
	Subtypes  []model.ExpenseSubtype
}

// ComputeProjecte groups the enabled forecasts by subtype -> concept, sums the
// amounts, collects+sorts CP codes, resolves apartat/tipus labels from the
// taxonomy, and orders tipus (CURRENT then INVESTMENT), apartats (by code) and
// concepts (by name). Pure: no I/O.
func ComputeProjecte(in ProjecteInput) ProjecteData {
	subLabel := map[string]string{}
	subType := map[string]string{}
	for _, s := range in.Subtypes {
		subLabel[s.Code()] = s.Label()
		subType[s.Code()] = s.TypeCode()
	}
	typeLabel := map[string]string{}
	typeCat := map[string]model.ExpenseCategory{}
	for _, t := range in.Types {
		typeLabel[t.Code()] = t.Label()
		typeCat[t.Code()] = t.Category()
	}

	type acc struct {
		total model.Money
		cps   []string
	}
	bySubtype := map[string]map[string]*acc{}
	for _, f := range in.Forecasts {
		if !f.Enabled() {
			continue
		}
		sc := f.SubtypeCode()
		concepts, ok := bySubtype[sc]
		if !ok {
			concepts = map[string]*acc{}
			bySubtype[sc] = concepts
		}
		a, ok := concepts[f.Concept()]
		if !ok {
			a = &acc{total: model.ZeroMoney()}
			concepts[f.Concept()] = a
		}
		a.total = a.total.Plus(f.GrossAmount())
		a.cps = append(a.cps, f.ID())
	}

	apartatBySub := map[string]ApartatProjecte{}
	for sc, concepts := range bySubtype {
		names := make([]string, 0, len(concepts))
		for n := range concepts {
			names = append(names, n)
		}
		sort.Strings(names)
		ap := ApartatProjecte{Code: sc, Label: labelOr(subLabel, sc), Total: model.ZeroMoney()}
		for _, n := range names {
			a := concepts[n]
			cps := append([]string(nil), a.cps...)
			sort.Strings(cps)
			ap.Concepts = append(ap.Concepts, ConcepteProjecte{Name: n, CPs: cps, Total: a.total})
			ap.Total = ap.Total.Plus(a.total)
		}
		apartatBySub[sc] = ap
	}

	subs := make([]string, 0, len(apartatBySub))
	for sc := range apartatBySub {
		subs = append(subs, sc)
	}
	sort.Strings(subs)

	tipusByCode := map[string]*TipusProjecte{}
	tipusOrder := []string{}
	for _, sc := range subs {
		tc := subType[sc]
		tp, ok := tipusByCode[tc]
		if !ok {
			tp = &TipusProjecte{Code: tc, Label: labelOr(typeLabel, tc), Category: typeCat[tc], Total: model.ZeroMoney()}
			tipusByCode[tc] = tp
			tipusOrder = append(tipusOrder, tc)
		}
		ap := apartatBySub[sc]
		tp.Apartats = append(tp.Apartats, ap)
		tp.Total = tp.Total.Plus(ap.Total)
	}

	sort.SliceStable(tipusOrder, func(i, j int) bool {
		ti, tj := tipusByCode[tipusOrder[i]], tipusByCode[tipusOrder[j]]
		if ri, rj := categoryRank(ti.Category), categoryRank(tj.Category); ri != rj {
			return ri < rj
		}
		return ti.Code < tj.Code
	})

	out := ProjecteData{Year: in.Year, Total: model.ZeroMoney()}
	for _, tc := range tipusOrder {
		tp := tipusByCode[tc]
		out.Tipus = append(out.Tipus, *tp)
		out.Total = out.Total.Plus(tp.Total)
	}
	return out
}

func labelOr(m map[string]string, code string) string {
	if l, ok := m[code]; ok && l != "" {
		return l
	}
	return code
}

func categoryRank(c model.ExpenseCategory) int {
	if c == model.CategoryInvestment {
		return 1
	}
	return 0
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/services/ -run TestComputeProjecte -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/services/projecte.go internal/domain/services/projecte_test.go
git commit -m "feat(services): ComputeProjecte groups forecasts into ProjecteData"
```

---

### Task 2: Pressupost (budget) Markdown renderer

**Files:**
- Create: `internal/adapters/report/pressupost_renderer.go`
- Test: `internal/adapters/report/pressupost_renderer_test.go`

**Interfaces:**
- Consumes: `services.ProjecteData` (Task 1); existing report-package helpers `formatEuro` (format.go), `writeMarkdownTable` + types `Table`/`Row` (markdown_renderer.go / layout.go).
- Produces: type `report.PressupostRenderer` with method `Render(d services.ProjecteData) []byte`; shared label helpers `apartatPrefix(code string) string`, `apartatHeading(code, label string) string`, `tipusHeading(code, label string) string` (used by Task 3 too).

- [ ] **Step 1: Write the failing test**

Create `internal/adapters/report/pressupost_renderer_test.go`:

```go
package report

import (
	"strings"
	"testing"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/services"
)

func projData2025(t *testing.T) services.ProjecteData {
	t.Helper()
	m := func(s string) model.Money {
		v, err := model.MoneyFromString(s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	return services.ProjecteData{
		Year:  2025,
		Total: m("30168.47"),
		Tipus: []services.TipusProjecte{
			{
				Code: "A", Label: "Despeses corrents", Category: model.CategoryCurrent, Total: m("23557.73"),
				Apartats: []services.ApartatProjecte{
					{Code: "a2", Label: "Activitats d'informació i promoció", Total: m("8189.00"),
						Concepts: []services.ConcepteProjecte{
							{Name: "Projecte de disseny i comunicació", CPs: []string{"CP25001"}, Total: m("8189.00")},
						}},
					{Code: "a6", Label: "Despeses de fertilitzants", Total: m("15368.73"),
						Concepts: []services.ConcepteProjecte{
							{Name: "Adob foliar", CPs: []string{"CP25005"}, Total: m("1488.73")},
							{Name: "Adob orgànic", CPs: []string{"CP25006", "CP25007"}, Total: m("13880.00")},
						}},
				},
			},
			{
				Code: "B", Label: "Despeses d'inversió", Category: model.CategoryInvestment, Total: m("6610.74"),
				Apartats: []services.ApartatProjecte{
					{Code: "b1", Label: "Despeses d'adquisició de maquinària i materials", Total: m("6610.74"),
						Concepts: []services.ConcepteProjecte{
							{Name: "Carretilla transportadora", CPs: []string{"CP25028"}, Total: m("6610.74")},
						}},
				},
			},
		},
	}
}

func TestPressupostRenderer_ContainsSummaryBreakdownAndCPs(t *testing.T) {
	out := string(PressupostRenderer{}.Render(projData2025(t)))

	mustContain(t, out, "# Pressupost del projecte d'actuació 2025")

	// Summary + grand total.
	mustContain(t, out, "## Resum per tipus de despesa")
	mustContain(t, out, "| A. Despeses corrents | a.2. Activitats d'informació i promoció | 8.189,00 € |")
	mustContain(t, out, "**Total general**")
	mustContain(t, out, "30.168,47 €")

	// Corrents breakdown: apartat heading, merged concept with both CPs.
	mustContain(t, out, "## Desglossament per conceptes de despeses corrents")
	mustContain(t, out, "### a.6. Despeses de fertilitzants")
	mustContain(t, out, "| Adob orgànic | CP25006, CP25007 | 13.880,00 € |")
	mustContain(t, out, "**Total a.6.**")

	// Inversions breakdown.
	mustContain(t, out, "## Desglossament per conceptes d'inversions")
	mustContain(t, out, "| Carretilla transportadora | CP25028 | 6.610,74 € |")
}

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("output missing %q\n---\n%s", needle, haystack)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/report/ -run TestPressupostRenderer -v`
Expected: FAIL — `undefined: PressupostRenderer`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/adapters/report/pressupost_renderer.go`:

```go
package report

import (
	"fmt"
	"strings"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/services"
)

// PressupostRenderer renders the F1 budget document (Markdown) from ProjecteData:
// a summary table by tipus/apartat plus per-concept breakdowns for corrents and
// inversions, each concept line carrying its CP code(s).
type PressupostRenderer struct{}

func (PressupostRenderer) Render(d services.ProjecteData) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# Pressupost del projecte d'actuació %d\n\n", d.Year)
	b.WriteString("El pressupost de les actuacions per a les quals es demana subvenció és el següent:\n\n")

	// Resum per tipus de despesa.
	b.WriteString("## Resum per tipus de despesa\n\n")
	resum := Table{Headers: []string{"Tipus", "Apartat", "Brut"}}
	for _, tp := range d.Tipus {
		for i, ap := range tp.Apartats {
			tipusCell := ""
			if i == 0 {
				tipusCell = tipusHeading(tp.Code, tp.Label)
			}
			resum.Rows = append(resum.Rows, Row{Cells: []string{tipusCell, apartatHeading(ap.Code, ap.Label), formatEuro(ap.Total)}})
		}
		resum.Rows = append(resum.Rows, Row{Cells: []string{"Total " + tipusHeading(tp.Code, tp.Label), "", formatEuro(tp.Total)}, Bold: true})
	}
	resum.Rows = append(resum.Rows, Row{Cells: []string{"Total general", "", formatEuro(d.Total)}, Bold: true})
	writeMarkdownTable(&b, resum)

	// Desglossament per conceptes, one section per tipus.
	for _, tp := range d.Tipus {
		fmt.Fprintf(&b, "## %s\n\n", desglossamentTitle(tp))
		for _, ap := range tp.Apartats {
			tbl := Table{Title: apartatHeading(ap.Code, ap.Label), Headers: []string{"Concepte", "CP", "Brut"}}
			for _, c := range ap.Concepts {
				tbl.Rows = append(tbl.Rows, Row{Cells: []string{c.Name, strings.Join(c.CPs, ", "), formatEuro(c.Total)}})
			}
			tbl.Rows = append(tbl.Rows, Row{Cells: []string{"Total " + apartatPrefix(ap.Code), "", formatEuro(ap.Total)}, Bold: true})
			writeMarkdownTable(&b, tbl)
		}
		fmt.Fprintf(&b, "**Total general: %s**\n\n", formatEuro(tp.Total))
	}
	return []byte(b.String())
}

func desglossamentTitle(tp services.TipusProjecte) string {
	if tp.Category == model.CategoryInvestment {
		return "Desglossament per conceptes d'inversions"
	}
	return "Desglossament per conceptes de despeses corrents"
}

// apartatPrefix turns a subtype code into its dotted apartat prefix: "a2" -> "a.2.".
func apartatPrefix(code string) string {
	if len(code) < 2 {
		return code + "."
	}
	return code[:1] + "." + code[1:] + "."
}

// apartatHeading is the full apartat label, e.g. "a.2. Activitats d'informació…".
func apartatHeading(code, label string) string { return apartatPrefix(code) + " " + label }

// tipusHeading is the full tipus label, e.g. "A. Despeses corrents".
func tipusHeading(code, label string) string { return code + ". " + label }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/report/ -run TestPressupostRenderer -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/report/pressupost_renderer.go internal/adapters/report/pressupost_renderer_test.go
git commit -m "feat(report): Pressupost (budget) Markdown renderer"
```

---

### Task 3: Projecte d'actuació (narrative) Markdown renderer

**Files:**
- Create: `internal/adapters/report/projecte_actuacio_renderer.go`
- Test: `internal/adapters/report/projecte_actuacio_renderer_test.go`

**Interfaces:**
- Consumes: `services.ProjecteData`; the label helper `apartatHeading` from Task 2.
- Produces: type `report.ProjecteActuacioRenderer` with method `Render(d services.ProjecteData) []byte`.

- [ ] **Step 1: Write the failing test**

Create `internal/adapters/report/projecte_actuacio_renderer_test.go`:

```go
package report

import (
	"strings"
	"testing"
)

func TestProjecteActuacioRenderer_SkeletonAndActivities(t *testing.T) {
	out := string(ProjecteActuacioRenderer{}.Render(projData2025(t)))

	mustContain(t, out, "# Projecte d'actuació 2025")
	mustContain(t, out, "_[Introducció")
	mustContain(t, out, "## Objectius")
	mustContain(t, out, "### a.2. Activitats d'informació i promoció")
	mustContain(t, out, "_[Objectius d'aquest apartat]_")
	mustContain(t, out, "## Activitats")
	mustContain(t, out, "### a.6. Despeses de fertilitzants")
	// Activity line = Concept (CP…), no amount.
	mustContain(t, out, "- Adob orgànic (CP25006, CP25007)")
	mustContain(t, out, "- Carretilla transportadora (CP25028)")
	mustContain(t, out, "President")

	// No euro amounts leak into the narrative.
	if strings.Contains(out, "€") {
		t.Errorf("narrative must not contain amounts, got:\n%s", out)
	}
}
```

(`projData2025` and `mustContain` come from `pressupost_renderer_test.go`, same package.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/report/ -run TestProjecteActuacioRenderer -v`
Expected: FAIL — `undefined: ProjecteActuacioRenderer`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/adapters/report/projecte_actuacio_renderer.go`:

```go
package report

import (
	"fmt"
	"strings"

	"github.com/pjover/espigol/internal/domain/services"
)

// ProjecteActuacioRenderer renders the "Projecte d'actuació" narrative skeleton
// (Markdown): editable placeholders for the intro and each Objectius apartat,
// plus a populated Activitats section (one bullet per concept with its CP code).
type ProjecteActuacioRenderer struct{}

func (ProjecteActuacioRenderer) Render(d services.ProjecteData) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# Projecte d'actuació %d\n\n", d.Year)
	b.WriteString("_[Introducció: convocatòria, BOIB, descripció de la cooperativa i activitats sol·licitades.]_\n\n")

	b.WriteString("## Objectius\n\n")
	for _, tp := range d.Tipus {
		for _, ap := range tp.Apartats {
			fmt.Fprintf(&b, "### %s\n\n", apartatHeading(ap.Code, ap.Label))
			b.WriteString("_[Objectius d'aquest apartat]_\n\n")
		}
	}

	b.WriteString("## Activitats\n\n")
	for _, tp := range d.Tipus {
		for _, ap := range tp.Apartats {
			fmt.Fprintf(&b, "### %s\n\n", apartatHeading(ap.Code, ap.Label))
			for _, c := range ap.Concepts {
				fmt.Fprintf(&b, "- %s (%s)\n", c.Name, strings.Join(c.CPs, ", "))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("_Estellencs, en data de la signatura._\n\n")
	b.WriteString("_Pere Jover Casasnovas, President_\n")
	return []byte(b.String())
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/report/ -run TestProjecteActuacioRenderer -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/report/projecte_actuacio_renderer.go internal/adapters/report/projecte_actuacio_renderer_test.go
git commit -m "feat(report): Projecte d'actuació narrative Markdown renderer"
```

---

### Task 4: Exporter — write both `.md` files

**Files:**
- Create: `internal/adapters/report/projecte_exporter.go`
- Test: `internal/adapters/report/projecte_exporter_test.go`

**Interfaces:**
- Consumes: `services.ProjecteData`; `ProjecteActuacioRenderer` (Task 3), `PressupostRenderer` (Task 2); existing `expandTilde` (exporter.go).
- Produces: type `report.ProjecteExporter` with constructor `NewProjecteExporter() ProjecteExporter` and method `Export(d services.ProjecteData, outputDir string) ([]string, error)` returning the two written paths (projecte first, pressupost second).

- [ ] **Step 1: Write the failing test**

Create `internal/adapters/report/projecte_exporter_test.go`:

```go
package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjecteExporter_WritesBothFiles(t *testing.T) {
	dir := t.TempDir()
	paths, err := NewProjecteExporter().Export(projData2025(t), dir)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("paths = %v, want 2", paths)
	}

	proj := filepath.Join(dir, "Projecte d'actuació 2025.md")
	press := filepath.Join(dir, "Pressupost del projecte d'actuació 2025.md")
	if paths[0] != proj || paths[1] != press {
		t.Errorf("paths = %v, want [%q %q]", paths, proj, press)
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %q to exist: %v", p, err)
		}
	}

	projBody, _ := os.ReadFile(proj)
	if !strings.Contains(string(projBody), "# Projecte d'actuació 2025") {
		t.Errorf("projecte file missing its title")
	}
	pressBody, _ := os.ReadFile(press)
	if !strings.Contains(string(pressBody), "## Resum per tipus de despesa") {
		t.Errorf("pressupost file missing its summary")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/report/ -run TestProjecteExporter -v`
Expected: FAIL — `undefined: NewProjecteExporter`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/adapters/report/projecte_exporter.go`:

```go
package report

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pjover/espigol/internal/domain/services"
)

// ProjecteExporter renders and writes the two Consorci Markdown documents for a
// year into outputDir, returning their paths (Projecte first, Pressupost second).
type ProjecteExporter struct {
	projecte   ProjecteActuacioRenderer
	pressupost PressupostRenderer
}

func NewProjecteExporter() ProjecteExporter { return ProjecteExporter{} }

func (e ProjecteExporter) Export(d services.ProjecteData, outputDir string) ([]string, error) {
	dir := expandTilde(outputDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating output dir %q: %w", dir, err)
	}
	projPath := filepath.Join(dir, fmt.Sprintf("Projecte d'actuació %d.md", d.Year))
	if err := os.WriteFile(projPath, e.projecte.Render(d), 0o644); err != nil {
		return nil, fmt.Errorf("writing %q: %w", projPath, err)
	}
	pressPath := filepath.Join(dir, fmt.Sprintf("Pressupost del projecte d'actuació %d.md", d.Year))
	if err := os.WriteFile(pressPath, e.pressupost.Render(d), 0o644); err != nil {
		return nil, fmt.Errorf("writing %q: %w", pressPath, err)
	}
	return []string{projPath, pressPath}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/adapters/report/ -run TestProjecteExporter -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/report/projecte_exporter.go internal/adapters/report/projecte_exporter_test.go
git commit -m "feat(report): ProjecteExporter writes both Consorci .md files"
```

---

### Task 5: Application service — `ProjecteService.Compute`

**Files:**
- Create: `internal/application/projecte_service.go`
- Test: `internal/application/projecte_service_test.go`

**Interfaces:**
- Consumes: `ports.TxManager`, `ports.RepoSet` (`r.Forecasts.ListByYear`, `r.Taxonomy.ListTypes`, `r.Taxonomy.ListSubtypes`); `services.ComputeProjecte`/`ProjecteInput` (Task 1).
- Produces: type `application.ProjecteService` with constructor `NewProjecteService(tx ports.TxManager) *ProjecteService` and method `Compute(ctx context.Context, year int) (services.ProjecteData, error)`.

- [ ] **Step 1: Write the failing test**

Create `internal/application/projecte_service_test.go`:

```go
package application_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

func TestProjecteService_ComputeReadsForecastsAndTaxonomy(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "proj.db"))
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

	w, _ := model.NewSubmissionWindow(2025, model.WindowOpen, nil, nil,
		time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	_ = win.Save(ctx, w)
	ta, _ := model.NewExpenseType(2025, "A", "Despeses corrents", model.CategoryCurrent)
	_ = tax.SaveType(ctx, ta)
	sa, _ := model.NewExpenseSubtype(2025, "a6", "Despeses de fertilitzants", "A")
	_ = tax.SaveSubtype(ctx, sa)
	planned := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	p, _ := model.NewPartner(7, "X", "X", "Y", "V", "x@e.cat", "6", model.Productor, 1, planned, false)
	_ = pr.Save(ctx, p)
	uf, _ := model.NewUnsavedExpenseForecast(p, "Adob orgànic", "d", model.MoneyOf(6580), model.ZeroMoney(),
		nil, planned, 2025, "a6", model.NewCommonScope(), planned, true)
	if _, err := fr.Create(ctx, uf); err != nil {
		t.Fatal(err)
	}

	svc := application.NewProjecteService(persistence.NewTxManager(conn))
	data, err := svc.Compute(ctx, 2025)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if data.Year != 2025 || data.Total.String() != "6580.00" {
		t.Errorf("Compute = year %d total %s, want 2025 / 6580.00", data.Year, data.Total.String())
	}
	if len(data.Tipus) != 1 || len(data.Tipus[0].Apartats) != 1 || data.Tipus[0].Apartats[0].Code != "a6" {
		t.Errorf("structure = %+v, want one tipus A with apartat a6", data.Tipus)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/application/ -run TestProjecteService -v`
Expected: FAIL — `undefined: application.NewProjecteService`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/application/projecte_service.go`:

```go
package application

import (
	"context"

	"github.com/pjover/espigol/internal/domain/ports"
	"github.com/pjover/espigol/internal/domain/services"
)

// ProjecteService assembles the year's forecasts + taxonomy and computes the
// grouped ProjecteData used by the two Consorci documents. Read-only.
type ProjecteService struct {
	tx ports.TxManager
}

func NewProjecteService(tx ports.TxManager) *ProjecteService {
	return &ProjecteService{tx: tx}
}

func (s *ProjecteService) Compute(ctx context.Context, year int) (services.ProjecteData, error) {
	var out services.ProjecteData
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		forecasts, err := r.Forecasts.ListByYear(ctx, year)
		if err != nil {
			return err
		}
		types, err := r.Taxonomy.ListTypes(ctx, year)
		if err != nil {
			return err
		}
		subtypes, err := r.Taxonomy.ListSubtypes(ctx, year)
		if err != nil {
			return err
		}
		out = services.ComputeProjecte(services.ProjecteInput{
			Year: year, Forecasts: forecasts, Types: types, Subtypes: subtypes,
		})
		return nil
	})
	return out, err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/application/ -run TestProjecteService -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/application/projecte_service.go internal/application/projecte_service_test.go
git commit -m "feat(application): ProjecteService.Compute gathers data + groups it"
```

---

### Task 6: Wire into TUI + new Admin `p` key

**Files:**
- Modify: `internal/adapters/tui/deps.go` (add two fields)
- Modify: `internal/wire/wire.go` (assemble them in `TUI`)
- Modify: `internal/adapters/tui/panel_admin.go` (msg + cmd + `p` case + Actions + Detail hint + doc comment)
- Modify: `internal/adapters/tui/panel_basic_test.go` (`testDeps` gets the two new deps)
- Test: `internal/adapters/tui/panel_admin_test.go` (new `p`-key test)

**Interfaces:**
- Consumes: `application.ProjecteService` (Task 5), `report.ProjecteExporter` (Task 4); existing `resultModalCmd`, `errDetail`, `openModalCmd`/`infoModal`, `Deps.Cfg.OutputDir`.
- Produces: `Deps.Projecte *application.ProjecteService`, `Deps.ProjecteExporter report.ProjecteExporter`; `projecteGeneratedMsg`; `generateProjecteCmd(deps Deps, year int) tea.Cmd`; Admin key `p`.

- [ ] **Step 1: Write the failing test**

Add to `internal/adapters/tui/panel_admin_test.go` (new function; reuses `testDeps`, `seedWindow`, `pKey`, `runCmd`, `infoModalMessage`, `testAdminEmail` already in the package). The forecast is seeded via `deps.Forecasts.AdminCreate` (which runs over the same wired DB), so no raw `*sql.DB` handle is needed. `ForecastInput` fields are exactly: `Concept, Description, GrossAmount, PlannedDate, SubtypeCode, ScopeKind (model.ScopeKind), SectionCode`.

```go
func TestAdminPanel_PKey_GeneratesConsorciDocs(t *testing.T) {
	deps, q := testDeps(t)
	ctx := context.Background()
	seedWindow(t, q, 2026, model.WindowOpen)
	tax := persistence.NewTaxonomyRepository(q)
	ta, _ := model.NewExpenseType(2026, "A", "Despeses corrents", model.CategoryCurrent)
	_ = tax.SaveType(ctx, ta)
	sa, _ := model.NewExpenseSubtype(2026, "a6", "Despeses de fertilitzants", "A")
	_ = tax.SaveSubtype(ctx, sa)
	p7, _ := model.NewPartner(7, "Soci", "Soci", "", "", "s7@e.test", "", model.Productor, 0,
		time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), false)
	_ = persistence.NewPartnerRepository(q).Save(ctx, p7)
	if _, err := deps.Forecasts.AdminCreate(ctx, testAdminEmail, 2026, 7, application.ForecastInput{
		Concept:     "Adob orgànic",
		Description: "d",
		GrossAmount: model.MoneyOf(6580),
		PlannedDate: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
		SubtypeCode: "a6",
		ScopeKind:   model.ScopeCommon,
	}); err != nil {
		t.Fatalf("seed forecast: %v", err)
	}

	p := NewAdminPanel(deps)
	p, cmd := p.Update(yearSelectedMsg{Year: 2026})
	if cmd != nil {
		runCmd(t, cmd)
	}
	_, cmd = p.Update(pKey("p"))
	genMsg, ok := runCmd(t, cmd).(projecteGeneratedMsg)
	if !ok {
		t.Fatalf("expected projecteGeneratedMsg, got %T", genMsg)
	}
	if genMsg.err != nil {
		t.Fatalf("generateProjecteCmd error: %v", genMsg.err)
	}
	_, cmd = p.Update(genMsg)

	outputDir := deps.Cfg.OutputDir
	for _, name := range []string{"Projecte d'actuació 2026.md", "Pressupost del projecte d'actuació 2026.md"} {
		if _, err := os.Stat(filepath.Join(outputDir, name)); err != nil {
			t.Errorf("expected %s to exist: %v", name, err)
		}
	}
	if msg := infoModalMessage(t, cmd); !strings.Contains(msg, "Documents Consorci generats") {
		t.Errorf("info modal = %q, want it to mention 'Documents Consorci generats'", msg)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/adapters/tui/ -run TestAdminPanel_PKey -v`
Expected: FAIL — `undefined: projecteGeneratedMsg` / `Deps` has no `Projecte`.

- [ ] **Step 3a: Add the Deps fields**

In `internal/adapters/tui/deps.go`, add after the `Backup` / `ActiveYear` fields:

```go
	Projecte         *application.ProjecteService
	ProjecteExporter report.ProjecteExporter
```

(The `application` and `report` packages are already imported in this file.)

- [ ] **Step 3b: Populate them in wire.TUI**

In `internal/wire/wire.go`, inside the `deps := tui.Deps{ … }` literal in `TUI(...)`, add:

```go
		Projecte:         application.NewProjecteService(txm),
		ProjecteExporter: reportadapter.NewProjecteExporter(),
```

(`reportadapter` is the existing import alias for `internal/adapters/report` in this file; `txm` is the already-declared `persistence.NewTxManager(conn)`.)

- [ ] **Step 3c: Add the msg + cmd + handling in panel_admin.go**

Add the message type near the other result messages (after `reconciliationGeneratedMsg`):

```go
// projecteGeneratedMsg carries the outcome of generateProjecteCmd.
type projecteGeneratedMsg struct {
	year  int
	paths []string
	err   error
}
```

Add the command near `generateReconciliationCmd`:

```go
// generateProjecteCmd computes the year's ProjecteData and writes the two
// Consorci Markdown documents (Projecte d'actuació + Pressupost) via
// ProjecteExporter. Live data, no window-state gate.
func generateProjecteCmd(deps Deps, year int) tea.Cmd {
	return func() tea.Msg {
		if deps.Projecte == nil || deps.Cfg == nil {
			return projecteGeneratedMsg{year: year, err: fmt.Errorf("documents Consorci no disponibles")}
		}
		data, err := deps.Projecte.Compute(context.Background(), year)
		if err != nil {
			return projecteGeneratedMsg{year: year, err: err}
		}
		paths, err := deps.ProjecteExporter.Export(data, deps.Cfg.OutputDir)
		return projecteGeneratedMsg{year: year, paths: paths, err: err}
	}
}
```

Add the Update case near `reconciliationGeneratedMsg`:

```go
	case projecteGeneratedMsg:
		if msg.year != p.year {
			return p, nil
		}
		var text string
		switch {
		case msg.err != nil:
			text = errDetail(msg.err)
		case len(msg.paths) == 0:
			text = "Documents Consorci generats (cap fitxer)."
		default:
			text = "Documents Consorci generats:\n  " + strings.Join(msg.paths, "\n  ")
		}
		return p, resultModalCmd(text, nil)
```

- [ ] **Step 3d: Add the key, action, hint, and doc comment**

In `handleKey`, add between the `k` and `c` cases:

```go
	case "p":
		return p, generateProjecteCmd(p.deps, p.year)
```

In `Actions()`, add between the `k` and `c` entries:

```go
		{Key: "p", Label: "documents Consorci"},
```

In `Detail()`, replace the hint string with:

```go
	return dimStyle.Render("h: informe previsions · i: informe conciliació · j: importa previsions · k: importa concessions i factures · p: documents Consorci · c: còpia · r: restaura")
```

Update the panel doc comment (top of the file) to insert the new key, e.g. change `…k import concessions + invoices / ajuts (no window gate), c backup…` to `…k import concessions + invoices / ajuts (no window gate), p generate the Consorci documents, c backup…`.

- [ ] **Step 3e: Give testDeps the new deps**

In `internal/adapters/tui/panel_basic_test.go`, inside the `deps := Deps{ … }` literal, add:

```go
		Projecte:         application.NewProjecteService(txm),
		ProjecteExporter: appreport.NewProjecteExporter(),
```

(`application` and `appreport` — the report package alias — are already imported in this test file; `txm` is already declared there.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/adapters/tui/ -run TestAdminPanel_PKey -v`
Expected: PASS.
Then the whole TUI package: `go test ./internal/adapters/tui/`
Expected: ok.

- [ ] **Step 5: Commit**

```bash
git add internal/adapters/tui/deps.go internal/wire/wire.go internal/adapters/tui/panel_admin.go internal/adapters/tui/panel_admin_test.go internal/adapters/tui/panel_basic_test.go
git commit -m "feat(tui): Admin 'p' key generates the two Consorci documents"
```

---

### Task 7: Full-suite gate + end-to-end verification

**Files:** none (verification only).

- [ ] **Step 1: Build, vet, full test suite**

Run:
```bash
make build
make vet
go test ./...
```
Expected: build succeeds, vet clean, all packages `ok`.

- [ ] **Step 2: Generate against a scratch copy of the 2025 data and check totals**

The real 2025 data lives in the Dropbox DB. Make a consistent scratch copy (never touch the live file), run the TUI, select 2025, press `p`, and inspect the budget totals.

Run:
```bash
SCRATCH=/private/tmp/claude-502/-Users-pere-dev-espigol/30fd1c58-6037-48dd-8f57-e96ae0605a08/scratchpad/esp_consorci
rm -rf "$SCRATCH"; mkdir -p "$SCRATCH"
sqlite3 "file:$HOME/Library/CloudStorage/Dropbox/Apps/espigol/espigol.db?mode=ro" ".backup '$SCRATCH/espigol.db'"
cp "$HOME/Library/CloudStorage/Dropbox/Apps/espigol/config.yaml" "$SCRATCH/config.yaml" 2>/dev/null || true
tmux kill-server 2>/dev/null || true
tmux new-session -d -s cs -x 140 -y 40 "ESPIGOL_HOME=$SCRATCH ./bin/espigol"
sleep 1.2
# Anys panel: select 2025 (earliest; press Up to reach the top row)
tmux send-keys -t cs '1'; sleep 0.3; tmux send-keys -t cs 'Up'; sleep 0.3
# Admin panel, generate Consorci docs
tmux send-keys -t cs '7'; sleep 0.3; tmux send-keys -t cs 'p'; sleep 1.5
tmux capture-pane -t cs -p | grep -i "Documents Consorci"
tmux send-keys -t cs 'Enter'; sleep 0.3; tmux send-keys -t cs 'q'; sleep 0.3
tmux kill-server 2>/dev/null || true
echo "=== budget totals ==="
grep -E "Total general|Total A|Total B|Total \(A|Total \(B" "$SCRATCH/reports/Pressupost del projecte d'actuació 2025.md"
```

Expected: the modal shows "Documents Consorci generats", and the budget's totals match the reference PDF:
- Total A. Despeses corrents = **29.860,73 €**
- Total B. Despeses d'inversió = **55.740,66 €**
- Total general = **85.601,39 €**

If any total differs, stop and investigate `ComputeProjecte` (grouping/summing) before proceeding — a numeric mismatch is a correctness bug, not a fixture update.

- [ ] **Step 3: Clean up the scratch dir**

Run: `rm -rf /private/tmp/claude-502/-Users-pere-dev-espigol/30fd1c58-6037-48dd-8f57-e96ae0605a08/scratchpad/esp_consorci`

---

## Notes for the implementer

- Follow the existing reconciliation-report code as the reference pattern (`internal/application/reconciliation_service.go`, `internal/adapters/report/reconciliation_*`).
- Keep all new user-facing strings in Catalan.
- The signature line and the fixed intro sentences are intentionally hardcoded (single-cooperative tool); the administrator edits the generated `.md` before submitting.
- `go fmt` runs as part of `make build`; keep gofmt-clean.
