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
