// Package services holds pure domain services. AllocationService.Compute is the
// allocation algorithm: it computes the full ReportData for a year's forecasts,
// ported from espigol-java AllocationService, generalized to N data-driven sections.
package services

import (
	"fmt"
	"sort"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/model/report"
)

// AllocationInput is the complete input to Compute (assembled by the caller).
type AllocationInput struct {
	Year            int
	Forecasts       []model.ExpenseForecast // the year's ENABLED forecasts
	Partners        []model.Partner
	Sections        []model.Section // ACTIVE sections, in display order
	Memberships     []model.PartnerSection
	SubtypeCategory map[string]model.ExpenseCategory
	CurrentLimit    model.Money
	InvestmentLimit model.Money
}

// Compute runs the allocation waterfall per category and returns the full ReportData.
func Compute(in AllocationInput) (report.ReportData, error) {
	if in.SubtypeCategory == nil {
		return report.ReportData{}, fmt.Errorf("SubtypeCategory must not be nil")
	}
	partnerByID := map[int]model.Partner{}
	for _, p := range in.Partners {
		partnerByID[p.ID()] = p
	}

	cats := []model.ExpenseCategory{model.CategoryCurrent, model.CategoryInvestment}
	limits := []model.Money{in.CurrentLimit, in.InvestmentLimit}

	categories := make([]report.CategoryReportData, 0, 2)
	hasNegative := false
	for i, cat := range cats {
		c := computeCategory(cat, limits[i], in, partnerByID)
		if c.Sections.Remainder.Cmp(model.ZeroMoney()) < 0 {
			hasNegative = true
		}
		categories = append(categories, c)
	}
	return report.ReportData{Year: in.Year, HasNegativeRemainder: hasNegative, Categories: categories}, nil
}

func computeCategory(cat model.ExpenseCategory, limit model.Money, in AllocationInput, partnerByID map[int]model.Partner) report.CategoryReportData {
	// Filter forecasts to this category.
	var forCat []model.ExpenseForecast
	for _, f := range in.Forecasts {
		if in.SubtypeCategory[f.SubtypeCode()] == cat {
			forCat = append(forCat, f)
		}
	}

	// Common scope.
	commonF := filterCommon(forCat)
	sortByConcept(commonF)
	commonTotal := sumForecasts(commonF)
	commonItems := make([]report.DetailItem, 0, len(commonF))
	for _, f := range commonF {
		commonItems = append(commonItems, report.DetailItem{
			CpCode: f.ID(), Concept: f.Concept(), Description: f.Description(),
			RequestedAmount: f.GrossAmount(), ApprovedAmount: f.GrossAmount(), // common: approved = gross
		})
	}
	common := report.CommonData{Available: limit, Total: commonTotal, Remainder: limit.Minus(commonTotal), Items: commonItems}

	// Section scopes (N, in display order; skip empty).
	sections := append([]model.Section(nil), in.Sections...)
	sort.SliceStable(sections, func(i, j int) bool { return sections[i].DisplayOrder() < sections[j].DisplayOrder() })
	var sectionDetails []report.SectionDetail
	sectionsTotal := model.ZeroMoney()
	for _, s := range sections {
		sf := filterSection(forCat, s.Code())
		if len(sf) == 0 {
			continue
		}
		sortByConcept(sf)
		sTotal := sumForecasts(sf)
		items := make([]report.DetailItem, 0, len(sf))
		for _, f := range sf {
			items = append(items, report.DetailItem{
				CpCode: f.ID(), Concept: f.Concept(), Description: f.Description(),
				RequestedAmount: f.GrossAmount(), ApprovedAmount: f.GrossAmount(),
			})
		}
		sectionDetails = append(sectionDetails, report.SectionDetail{SectionCode: s.Code(), Label: s.Label(), Items: items, Total: sTotal})
		sectionsTotal = sectionsTotal.Plus(sTotal)
	}
	availableForSections := limit.Minus(commonTotal)
	sectionsRemainder := availableForSections.Minus(sectionsTotal)

	// Warning (only when sections are over-budget).
	var warning *report.WarningData
	if sectionsRemainder.Cmp(model.ZeroMoney()) < 0 {
		warning = computeWarning(cat, availableForSections, sections, sectionDetails, in)
	}

	// Partner (Soci) scope.
	partnerF := filterPartner(forCat)
	partnerTotals := map[int]model.Money{}
	partnerNames := map[int]string{}
	var partnerOrder []int
	for _, f := range partnerF {
		if _, ok := partnerTotals[f.PartnerID()]; !ok {
			partnerOrder = append(partnerOrder, f.PartnerID())
			partnerNames[f.PartnerID()] = displayName(partnerByID, f.PartnerID())
		}
		partnerTotals[f.PartnerID()] = partnerTotals[f.PartnerID()].Plus(f.GrossAmount())
	}
	grandTotal := model.ZeroMoney()
	for _, id := range partnerOrder {
		grandTotal = grandTotal.Plus(partnerTotals[id])
	}
	hasExcess := grandTotal.Cmp(sectionsRemainder) > 0

	fair := distribute(sectionsRemainder, partnerTotals, partnerNames)
	allocByID := map[int]report.PartnerAllocation{}
	for _, a := range fair.allocations {
		allocByID[a.PartnerID] = a
	}

	partnersData := report.PartnersData{
		SubtypeTotals:  aggregateSubtypeTotals(partnerF),
		GrandTotal:     grandTotal,
		HasExcess:      hasExcess,
		FinalRemainder: fair.finalRemainder,
		Allocations:    fair.allocations,
		PartnerDetails: perPartnerDetails(partnerF, partnerByID, allocByID),
	}
	sectionsData := report.SectionsData{
		Available: availableForSections, Total: sectionsTotal, Remainder: sectionsRemainder,
		SectionDetails: sectionDetails, Partners: partnersData,
	}
	return report.CategoryReportData{Category: cat, Common: common, Sections: sectionsData, Warning: warning}
}

func aggregateSubtypeTotals(partnerF []model.ExpenseForecast) []report.SubtypeTotal {
	byCode := map[string]model.Money{}
	var order []string
	for _, f := range partnerF {
		if _, ok := byCode[f.SubtypeCode()]; !ok {
			order = append(order, f.SubtypeCode())
		}
		byCode[f.SubtypeCode()] = byCode[f.SubtypeCode()].Plus(f.GrossAmount())
	}
	out := make([]report.SubtypeTotal, 0, len(byCode))
	for _, code := range order {
		out = append(out, report.SubtypeTotal{SubtypeCode: code, Amount: byCode[code]})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SubtypeCode < out[j].SubtypeCode })
	return out
}

func perPartnerDetails(partnerF []model.ExpenseForecast, partnerByID map[int]model.Partner, allocByID map[int]report.PartnerAllocation) []report.PartnerDetail {
	byPartner := map[int][]model.ExpenseForecast{}
	var order []int
	for _, f := range partnerF {
		if _, ok := byPartner[f.PartnerID()]; !ok {
			order = append(order, f.PartnerID())
		}
		byPartner[f.PartnerID()] = append(byPartner[f.PartnerID()], f)
	}
	out := make([]report.PartnerDetail, 0, len(byPartner))
	for _, id := range order {
		pf := byPartner[id]
		sortByConcept(pf)
		alloc, hasAlloc := allocByID[id]
		total := model.ZeroMoney()
		items := make([]report.DetailItem, 0, len(pf))
		for _, f := range pf {
			approved := f.GrossAmount()
			if hasAlloc && alloc.Requested.Cmp(model.ZeroMoney()) > 0 {
				ratio := alloc.Allocated.Decimal().Div(alloc.Requested.Decimal())
				approved = f.GrossAmount().TimesRatio(ratio)
			}
			items = append(items, report.DetailItem{
				CpCode: f.ID(), Concept: f.Concept(), Description: f.Description(),
				RequestedAmount: f.GrossAmount(), ApprovedAmount: approved,
			})
			total = total.Plus(approved)
		}
		isCapped := hasAlloc && alloc.Allocated.Cmp(alloc.Requested) < 0
		maxAuth := model.ZeroMoney()
		if hasAlloc {
			maxAuth = alloc.Allocated
		}
		out = append(out, report.PartnerDetail{Name: displayName(partnerByID, id), Items: items, Total: total, IsCapped: isCapped, MaxAuthorized: maxAuth})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func displayName(partnerByID map[int]model.Partner, id int) string {
	p, ok := partnerByID[id]
	if !ok {
		return fmt.Sprintf("Unknown (%d)", id)
	}
	return fmt.Sprintf("%s (%d)", p.Name(), p.ID())
}

func filterCommon(in []model.ExpenseForecast) []model.ExpenseForecast {
	var out []model.ExpenseForecast
	for _, f := range in {
		if f.Scope().Kind() == model.ScopeCommon {
			out = append(out, f)
		}
	}
	return out
}

func filterSection(in []model.ExpenseForecast, code string) []model.ExpenseForecast {
	var out []model.ExpenseForecast
	for _, f := range in {
		if f.Scope().Kind() == model.ScopeSection && f.Scope().SectionCode() == code {
			out = append(out, f)
		}
	}
	return out
}

func filterPartner(in []model.ExpenseForecast) []model.ExpenseForecast {
	var out []model.ExpenseForecast
	for _, f := range in {
		if f.Scope().Kind() == model.ScopePartner {
			out = append(out, f)
		}
	}
	return out
}

func sortByConcept(fs []model.ExpenseForecast) {
	sort.SliceStable(fs, func(i, j int) bool { return fs[i].Concept() < fs[j].Concept() })
}

func sumForecasts(fs []model.ExpenseForecast) model.Money {
	total := model.ZeroMoney()
	for _, f := range fs {
		total = total.Plus(f.GrossAmount())
	}
	return total
}

// computeWarning splits availableForSections across the active sections in
// proportion to the number of PRODUCER members of each section. A producer who
// belongs to two sections counts in each (matching the reference).
func computeWarning(cat model.ExpenseCategory, availableForSections model.Money, sections []model.Section, sectionDetails []report.SectionDetail, in AllocationInput) *report.WarningData {
	requestedByCode := map[string]model.Money{}
	for _, sd := range sectionDetails {
		requestedByCode[sd.SectionCode] = sd.Total
	}
	producerByCode := producerCounts(in)

	denominator := 0
	for _, s := range sections {
		denominator += producerByCode[s.Code()]
	}

	rows := make([]report.SectionWarning, 0, len(sections))
	for _, s := range sections {
		n := producerByCode[s.Code()]
		allowed := model.ZeroMoney()
		if denominator > 0 {
			allowed = availableForSections.Times(n).DividedBy(denominator)
		}
		requested := requestedByCode[s.Code()]
		rows = append(rows, report.SectionWarning{
			SectionCode: s.Code(), Label: s.Label(), Producers: n,
			Allowed: allowed, Requested: requested, Adjustment: requested.Minus(allowed),
		})
	}
	return &report.WarningData{Category: cat, Rows: rows}
}

// producerCounts returns, per section code, the number of PRODUCER partners that
// are members of that section.
func producerCounts(in AllocationInput) map[string]int {
	isProducer := map[int]bool{}
	for _, p := range in.Partners {
		if p.PartnerType() == model.Productor {
			isProducer[p.ID()] = true
		}
	}
	counts := map[string]int{}
	for _, m := range in.Memberships {
		if isProducer[m.PartnerID()] {
			counts[m.SectionCode()]++
		}
	}
	return counts
}
