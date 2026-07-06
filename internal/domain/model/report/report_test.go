package report

import (
	"testing"

	"github.com/pjover/espigol/internal/domain/model"
)

func TestReportDataStructsCompose(t *testing.T) {
	rd := ReportData{
		Year:                 2026,
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
				Available:      model.MoneyOf(27120),
				Total:          model.MoneyOf(27111),
				Remainder:      model.MoneyOf(9),
				SectionDetails: []SectionDetail{{SectionCode: "oliva", Label: "Secció d'oliva", Total: model.MoneyOf(19721)}},
				Partners: PartnersData{
					GrandTotal:     model.ZeroMoney(),
					FinalRemainder: model.MoneyOf(9),
					Allocations:    []PartnerAllocation{},
					SubtypeTotals:  []SubtypeTotal{},
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
