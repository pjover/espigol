package report_test

import (
	"strings"
	"testing"

	reportpkg "github.com/pjover/espigol/internal/adapters/report"
	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/services"
)

func TestReconciliationMarkdownRenderer_ContainsTitleAndStatus(t *testing.T) {
	m := func(s string) model.Money {
		v, _ := model.MoneyFromString(s)
		return v
	}
	rd := services.ReconciliationData{
		Year: 2025,
		Categories: []services.CategoryReconciliation{
			{
				Category: model.CategoryCurrent,
				Subtypes: []services.SubtypeReconciliation{
					{
						Code:  "a6",
						Label: "[a6]",
						Concessions: []services.ConcessionReconciliation{
							{
								GroupCode: "A6-01",
								Concept:   "Adob",
								Forecasts: []services.ForecastReconciliation{
									{
										ForecastID:      "CP25001",
										PartnerNickName: "Soci 7",
										Concept:         "F1",
										GrossAmount:     m("1000.00"),
										Executed:        m("900.00"),
										Assigned:        m("900.00"),
										Status:          services.StatusFullyJustified,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	md := reportpkg.ReconciliationMarkdownRenderer{}.Render(rd)
	s := string(md)

	if !strings.Contains(s, "# Conciliació d'ajuts 2025") {
		t.Errorf("missing title; got start: %q", s[:min(100, len(s))])
	}
	if !strings.Contains(s, "Justificat") {
		t.Errorf("missing status label 'Justificat'")
	}
	if !strings.Contains(s, "900,00 €") {
		t.Errorf("missing money format '900,00 €'")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
