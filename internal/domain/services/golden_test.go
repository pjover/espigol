package services

import (
	"testing"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/model/report"
)

type reportCategory = report.CategoryReportData

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
