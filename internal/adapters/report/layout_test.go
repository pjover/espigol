package report

import (
	"strings"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/model/report"
	"github.com/pjover/espigol/internal/domain/services"
)

// buildGolden computes the ReportData for the anonymized 2026 golden
// scenario (same numbers as the Phase-3 golden test, anonymized text).
func buildGolden(t *testing.T) report.ReportData {
	t.Helper()
	d := func(s string) model.Money {
		m, err := model.MoneyFromString(s)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	com := model.NewCommonScope()
	par := model.NewPartnerScope()
	oliva, _ := model.NewSectionScope("oliva")
	ram, _ := model.NewSectionScope("ramaderia")
	mk := func(id string, pid int, gross string, scope model.ExpenseScope, sub string) model.ExpenseForecast {
		planned := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
		p, err := model.NewPartner(pid, "Soci", "", "", "soci@e.test", "", model.Productor, 0, planned, false)
		if err != nil {
			t.Fatal(err)
		}
		f, err := model.NewExpenseForecast(id, p, "Concepte "+id, "", d(gross), model.ZeroMoney(), nil, planned, 2026, sub, scope, planned, true)
		if err != nil {
			t.Fatal(err)
		}
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
		if err != nil {
			t.Fatal(err)
		}
		partners = append(partners, p)
	}
	sOliva, _ := model.NewSection("oliva", "Secció d'oliva", true, 1)
	sRam, _ := model.NewSection("ramaderia", "Secció de ramaderia", true, 2)
	rd, err := services.Compute(services.AllocationInput{
		Year: 2026, Forecasts: forecasts, Partners: partners,
		Sections:        []model.Section{sOliva, sRam},
		SubtypeCategory: map[string]model.ExpenseCategory{"a1": model.CategoryCurrent, "b1": model.CategoryInvestment},
		CurrentLimit:    model.MoneyOf(30000), InvestmentLimit: model.MoneyOf(70000),
	})
	if err != nil {
		t.Fatal(err)
	}
	return rd
}

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
	blocks := buildLayout(rd)

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
		if !strings.Contains(joined, want) {
			t.Errorf("layout missing expected amount %q", want)
		}
	}
}
