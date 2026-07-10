package application

import (
	"testing"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/model/report"
	"github.com/pjover/espigol/internal/domain/services"
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

func TestReconciliationSnapshotRoundTrip(t *testing.T) {
	rd := services.ReconciliationData{
		Year: 2025,
		Categories: []services.CategoryReconciliation{
			{
				Category:     model.CategoryCurrent,
				NetDeviation: model.MoneyOf(100),
			},
		},
	}

	s, err := ReconciliationSnapshotToJSON(rd)
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	if s == "" {
		t.Fatal("ToJSON returned empty string")
	}

	got, err := ReconciliationSnapshotFromJSON(s)
	if err != nil {
		t.Fatalf("FromJSON: %v", err)
	}
	if got.Year != 2025 {
		t.Errorf("Year = %d, want 2025", got.Year)
	}
	if len(got.Categories) != 1 {
		t.Fatalf("Categories len = %d, want 1", len(got.Categories))
	}
	if got.Categories[0].NetDeviation.String() != "100.00" {
		t.Errorf("NetDeviation = %s, want 100.00", got.Categories[0].NetDeviation)
	}
}
