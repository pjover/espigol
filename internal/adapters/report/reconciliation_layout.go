package report

import (
	"fmt"
	"strings"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/services"
)

func buildReconciliationLayout(rd services.ReconciliationData) []Block {
	var blocks []Block
	blocks = append(blocks, summaryReconciliationBlocks(rd)...)
	for i, cat := range rd.Categories {
		blocks = append(blocks, categoryReconciliationBlocks(cat)...)
		if i < len(rd.Categories)-1 {
			blocks = append(blocks, PageBreak{})
		}
	}
	return blocks
}

// summaryReconciliationBlocks builds the leading "Resum" section: one row per
// category with its rolled-up figures, plus a grand-total row.
func summaryReconciliationBlocks(rd services.ReconciliationData) []Block {
	if len(rd.Categories) == 0 {
		return nil
	}
	rows := make([]Row, 0, len(rd.Categories)+1)
	var req, gr, ex, as, dev model.Money = model.ZeroMoney(), model.ZeroMoney(), model.ZeroMoney(), model.ZeroMoney(), model.ZeroMoney()
	for _, cat := range rd.Categories {
		rows = append(rows, Row{Cells: []string{
			categorySummaryLabel(cat),
			formatEuro(cat.Requested),
			formatEuro(cat.Granted),
			formatEuro(cat.Executed),
			formatEuro(cat.Assigned),
			formatEuro(cat.NetDeviation),
		}})
		req = req.Plus(cat.Requested)
		gr = gr.Plus(cat.Granted)
		ex = ex.Plus(cat.Executed)
		as = as.Plus(cat.Assigned)
		dev = dev.Plus(cat.NetDeviation)
	}
	rows = append(rows, Row{
		Cells: []string{"Total", formatEuro(req), formatEuro(gr), formatEuro(ex), formatEuro(as), formatEuro(dev)},
		Bold:  true,
	})
	return []Block{
		SectionTitle{Text: "Resum"},
		Table{
			Headers: []string{"Subtipus", "Demanat", "Concedit", "Executat", "Assignat", "Desviació"},
			Widths:  []uint{3, 2, 2, 2, 2, 2},
			Rows:    rows,
		},
	}
}

func categoryHeader(cat services.CategoryReconciliation) string {
	codes := make([]string, len(cat.Subtypes))
	for i, st := range cat.Subtypes {
		codes[i] = st.Code
	}
	return fmt.Sprintf("%s (%s)", categoryLabel(cat.Category), strings.Join(codes, ", "))
}

// categorySummaryLabel labels a category in the Resum table with its distinct
// type letters (e.g. "Despesa corrent (a)"), derived from the leading letter of
// its subtype codes — unlike categoryHeader, which lists every subtype code.
func categorySummaryLabel(cat services.CategoryReconciliation) string {
	seen := map[string]bool{}
	var letters []string
	for _, st := range cat.Subtypes {
		if st.Code == "" {
			continue
		}
		l := strings.ToLower(st.Code[:1])
		if !seen[l] {
			seen[l] = true
			letters = append(letters, l)
		}
	}
	return fmt.Sprintf("%s (%s)", categoryLabel(cat.Category), strings.Join(letters, ", "))
}

func categoryReconciliationBlocks(cat services.CategoryReconciliation) []Block {
	var blocks []Block

	// 1. Category heading
	blocks = append(blocks, SectionTitle{Text: categoryHeader(cat)})

	// 2. Category summary: one row per subtype + bold totals row
	summaryRows := make([]Row, 0, len(cat.Subtypes)+1)
	for _, st := range cat.Subtypes {
		summaryRows = append(summaryRows, Row{Cells: []string{
			st.Code,
			formatEuro(st.Requested),
			formatEuro(st.Granted),
			formatEuro(st.Executed),
			formatEuro(st.Assigned),
			formatEuro(st.Deviation),
		}})
	}
	summaryRows = append(summaryRows, Row{
		Cells: []string{
			"Total",
			formatEuro(cat.Requested),
			formatEuro(cat.Granted),
			formatEuro(cat.Executed),
			formatEuro(cat.Assigned),
			formatEuro(cat.NetDeviation),
		},
		Bold: true,
	})
	blocks = append(blocks, Table{
		Headers: []string{"Subtipus", "Demanat", "Concedit", "Executat", "Assignat", "Desviació"},
		Widths:  []uint{2, 2, 2, 2, 2, 2},
		Rows:    summaryRows,
	})

	// 3. Per-subtype detail sections
	for _, st := range cat.Subtypes {
		blocks = append(blocks, subtypeReconciliationBlocks(st)...)
	}

	return blocks
}

func subtypeReconciliationBlocks(st services.SubtypeReconciliation) []Block {
	var blocks []Block

	// Subtype heading
	blocks = append(blocks, SectionTitle{Text: st.Code + " — " + st.Label})

	// Concessions summary table
	cnRows := make([]Row, 0, len(st.Concessions)+1)
	for _, cn := range st.Concessions {
		cnRows = append(cnRows, Row{Cells: []string{
			cn.GroupCode, cn.Concept,
			formatEuro(cn.Requested), formatEuro(cn.Granted),
			formatEuro(cn.Executed), formatEuro(cn.Assigned),
			formatEuro(cn.Difference),
		}})
	}
	cnRows = append(cnRows, Row{
		Cells: []string{
			"Total", "",
			formatEuro(st.Requested), formatEuro(st.Granted),
			formatEuro(st.Executed), formatEuro(st.Assigned),
			formatEuro(st.Deviation),
		},
		Bold: true,
	})
	blocks = append(blocks, Table{
		Headers: []string{"Grup", "Concepte", "Demanat", "Concedit", "Executat", "Assignat", "Diferència"},
		Widths:  []uint{1, 2, 2, 2, 2, 2, 1},
		Rows:    cnRows,
	})

	// Per-concession forecast tables
	for _, cn := range st.Concessions {
		blocks = append(blocks, concessionBlocks(cn))
	}

	return blocks
}

func concessionBlocks(cn services.ConcessionReconciliation) Block {
	rows := make([]Row, 0, len(cn.Forecasts)*2)
	for _, fr := range cn.Forecasts {
		rows = append(rows, Row{Cells: []string{
			fr.ForecastID,
			fr.PartnerNickName,
			fr.Concept,
			formatEuro(fr.GrossAmount),
			formatEuro(fr.Executed),
			formatEuro(fr.Assigned),
			statusLabel(fr.Status),
		}})
		for _, inv := range fr.Invoices {
			paid := "✗"
			if inv.FullyPaid {
				paid = "✓"
			}
			rows = append(rows, Row{Cells: []string{
				"↳ " + inv.Issuer + " " + inv.Number + " (" + inv.IssueDate.Format("02/01/2006") + ")",
				"", "",
				formatEuro(inv.LinkedAmount),
				"", "",
				paid,
			}})
		}
	}
	return Table{
		Title:   cn.Concept + " (Grup " + cn.GroupCode + ")",
		Headers: []string{"Previsió", "Soci", "Concepte", "Previst", "Executat", "Assignat", "Estat"},
		Widths:  []uint{2, 1, 2, 2, 2, 2, 1},
		Rows:    rows,
	}
}

func statusLabel(s services.ForecastReconStatus) string {
	switch s {
	case services.StatusFullyJustified:
		return "Justificat"
	case services.StatusPartiallyJustified:
		return "Parcial"
	case services.StatusOverExecuted:
		return "Sobre-executat"
	case services.StatusPaymentPending:
		return "Pendent pagament"
	case services.StatusNoInvoice:
		return "Sense factura"
	}
	return ""
}
