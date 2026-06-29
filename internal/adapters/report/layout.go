package report

import (
	"fmt"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/model/report"
)

// Block is one renderable unit of a report (sealed: SectionTitle | Table | PageBreak).
type Block interface{ isBlock() }

// SectionTitle is a prominent heading.
type SectionTitle struct{ Text string }

// Row is one table row; Bold renders emphasized, Red flags an over-budget/capped value.
type Row struct {
	Cells []string
	Bold  bool
	Red   bool
}

// Table is a titled table. Widths are maroto grid units (sum 12); Markdown ignores them.
type Table struct {
	Title   string
	Headers []string
	Widths  []uint
	Rows    []Row
}

// PageBreak forces a new page (PDF); Markdown renders a horizontal rule.
type PageBreak struct{}

func (SectionTitle) isBlock() {}
func (Table) isBlock()        {}
func (PageBreak) isBlock()    {}

// buildLayout walks the computed ReportData and emits the shared block sequence
// consumed identically by the PDF and Markdown renderers.
func buildLayout(rd report.ReportData) []Block {
	var blocks []Block
	for i, cat := range rd.Categories {
		blocks = append(blocks, categoryBlocks(rd.Year, cat)...)
		if i < len(rd.Categories)-1 {
			blocks = append(blocks, PageBreak{})
		}
	}
	blocks = append(blocks, PageBreak{}, SectionTitle{Text: "Resum"})
	for _, cat := range rd.Categories {
		blocks = append(blocks, resumTable(cat))
	}
	return blocks
}

func categoryBlocks(year int, cat report.CategoryReportData) []Block {
	label := categoryLabel(cat.Category)
	var blocks []Block

	blocks = append(blocks, SectionTitle{Text: label})

	// 1. Common
	commonRows := make([]Row, 0, len(cat.Common.Items)+1)
	for _, it := range cat.Common.Items {
		commonRows = append(commonRows, Row{Cells: []string{it.CpCode, it.Concept, formatEuro(it.RequestedAmount)}})
	}
	commonRows = append(commonRows, Row{Cells: []string{"", "Total comú", formatEuro(cat.Common.Total)}, Bold: true})
	blocks = append(blocks, Table{Title: label + " — Comú", Headers: []string{"CP", "Concepte", "Brut"}, Widths: []uint{2, 7, 3}, Rows: commonRows})

	// 2. Sections
	secRows := make([]Row, 0)
	for _, sd := range cat.Sections.SectionDetails {
		secRows = append(secRows, Row{Cells: []string{sd.Label, "", ""}, Bold: true})
		for _, it := range sd.Items {
			secRows = append(secRows, Row{Cells: []string{it.CpCode, it.Concept, formatEuro(it.RequestedAmount)}})
		}
		secRows = append(secRows, Row{Cells: []string{"", "Total " + sd.Label, formatEuro(sd.Total)}, Bold: true})
	}
	secRows = append(secRows, Row{Cells: []string{"", "Total seccions", formatEuro(cat.Sections.Total)}, Bold: true})
	blocks = append(blocks, Table{Title: label + " — Seccions", Headers: []string{"CP", "Concepte", "Brut"}, Widths: []uint{2, 7, 3}, Rows: secRows})

	// 3. Remainder summary
	categoryTotal := cat.Common.Total.Plus(cat.Sections.Total)
	remRows := []Row{
		{Cells: []string{fmt.Sprintf("Disponible any %d", year), formatEuro(cat.Common.Available)}},
		{Cells: []string{"Total comú", formatEuro(cat.Common.Total)}},
		{Cells: []string{"Disponible per seccions", formatEuro(cat.Sections.Available)}},
		{Cells: []string{"Total seccions", formatEuro(cat.Sections.Total)}},
		{Cells: []string{"Total " + label, formatEuro(categoryTotal)}},
		{Cells: []string{"Remanent", formatEuro(cat.Sections.Remainder)}, Bold: true},
	}
	blocks = append(blocks, Table{Title: "Remanent de " + label, Headers: []string{"", ""}, Widths: []uint{8, 4}, Rows: remRows})

	// 4. Warning (only when over budget)
	if cat.Warning != nil {
		wRows := make([]Row, 0, len(cat.Warning.Rows))
		for _, w := range cat.Warning.Rows {
			wRows = append(wRows, Row{
				Cells: []string{w.Label, fmt.Sprintf("%d", w.Producers), formatEuro(w.Allowed), formatEuro(w.Adjustment)},
				Red:   true,
			})
		}
		blocks = append(blocks, Table{Title: "⚠ AVÍS: Ajust necessari per " + label, Headers: []string{"Secció", "Socis productors", "Disponible", "Ajust"}, Widths: []uint{4, 2, 3, 3}, Rows: wRows})
	}

	// 5. Partners (subtype totals + adjustment or final remainder)
	pdata := cat.Sections.Partners
	pRows := make([]Row, 0, len(pdata.SubtypeTotals)+2)
	for _, st := range pdata.SubtypeTotals {
		pRows = append(pRows, Row{Cells: []string{st.SubtypeCode, formatEuro(st.Amount)}})
	}
	pRows = append(pRows, Row{Cells: []string{"Total socis", formatEuro(pdata.GrandTotal)}, Bold: true})
	if !pdata.HasExcess {
		pRows = append(pRows, Row{Cells: []string{"Remanent final", formatEuro(pdata.FinalRemainder)}, Bold: true})
	}
	blocks = append(blocks, Table{Title: label + " — Socis", Headers: []string{"Subtipus de despesa", "Brut"}, Widths: []uint{8, 4}, Rows: pRows})

	if pdata.HasExcess {
		adjRows := make([]Row, 0, len(pdata.Allocations)+1)
		for _, a := range pdata.Allocations {
			adjRows = append(adjRows, Row{
				Cells: []string{a.PartnerName, formatEuro(a.Requested), formatEuro(a.Allocated)},
				Red:   a.Allocated.Cmp(a.Requested) < 0,
			})
		}
		blocks = append(blocks, Table{Title: "Ajust de despeses per soci (" + label + ")", Headers: []string{"Soci", "Sol·licitat", "Assignat"}, Widths: []uint{5, 4, 3}, Rows: adjRows})
	}

	// 6. Detail per scope and per partner
	blocks = append(blocks, SectionTitle{Text: "Detall per secció i soci — " + label})
	// common detail
	if len(cat.Common.Items) > 0 {
		blocks = append(blocks, detailTable("Comú", cat.Common.Items))
	}
	for _, sd := range cat.Sections.SectionDetails {
		blocks = append(blocks, detailTable(sd.Label, sd.Items))
	}
	for _, pd := range pdata.PartnerDetails {
		rows := make([]Row, 0, len(pd.Items)+2)
		for _, it := range pd.Items {
			rows = append(rows, Row{Cells: []string{it.CpCode, it.Concept, formatEuro(it.ApprovedAmount)}})
		}
		rows = append(rows, Row{Cells: []string{"", "Total", formatEuro(pd.Total)}, Bold: true})
		if pd.IsCapped {
			rows = append(rows, Row{Cells: []string{"", "Import màxim autoritzat", formatEuro(pd.MaxAuthorized)}, Bold: true, Red: true})
		}
		blocks = append(blocks, Table{Title: pd.Name, Headers: []string{"CP", "Concepte", "Brut"}, Widths: []uint{2, 7, 3}, Rows: rows})
	}

	return blocks
}

func detailTable(title string, items []report.DetailItem) Table {
	rows := make([]Row, 0, len(items)+1)
	total := model.ZeroMoney()
	for _, it := range items {
		rows = append(rows, Row{Cells: []string{it.CpCode, it.Concept, formatEuro(it.RequestedAmount)}})
		total = total.Plus(it.RequestedAmount)
	}
	rows = append(rows, Row{Cells: []string{"", "Total", formatEuro(total)}, Bold: true})
	return Table{Title: title, Headers: []string{"CP", "Concepte", "Brut"}, Widths: []uint{2, 7, 3}, Rows: rows}
}

func resumTable(cat report.CategoryReportData) Table {
	label := categoryLabel(cat.Category)
	limit := cat.Common.Available
	socisApproved := model.ZeroMoney()
	for _, a := range cat.Sections.Partners.Allocations {
		socisApproved = socisApproved.Plus(a.Allocated)
	}
	socisRequested := cat.Sections.Partners.GrandTotal
	totalReq := cat.Common.Total.Plus(cat.Sections.Total).Plus(socisRequested)
	totalApp := cat.Common.Total.Plus(cat.Sections.Total).Plus(socisApproved)
	remReq := limit.Minus(totalReq)
	remApp := limit.Minus(totalApp)
	rows := []Row{
		{Cells: []string{"Import disponible", formatEuro(limit), formatEuro(limit)}, Bold: true},
		{Cells: []string{"Comú", formatEuro(cat.Common.Total), formatEuro(cat.Common.Total)}},
		{Cells: []string{"Seccions", formatEuro(cat.Sections.Total), formatEuro(cat.Sections.Total)}},
		{Cells: []string{"Socis", formatEuro(socisRequested), formatEuro(socisApproved)}},
		{Cells: []string{"Total", formatEuro(totalReq), formatEuro(totalApp)}, Bold: true},
		{Cells: []string{"Import remanent", formatEuro(remReq), formatEuro(remApp)}, Bold: true},
	}
	return Table{Title: label, Headers: []string{"Concepte", "Previst", "Aprovat"}, Widths: []uint{6, 3, 3}, Rows: rows}
}
