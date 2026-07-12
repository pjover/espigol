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
	p, err := model.NewPartner(id, "Soci "+itoa(id), "Soci "+itoa(id), "", "", "x@x.test", "", model.Patrocinador, 0,
		modelTime(), false)
	if err != nil {
		t.Fatalf("partner: %v", err)
	}
	return p
}
