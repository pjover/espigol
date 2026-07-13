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
