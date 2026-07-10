package report

import (
	"strings"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/services"
)

func minimalReconData(t *testing.T) services.ReconciliationData {
	t.Helper()
	m := func(s string) model.Money {
		v, err := model.MoneyFromString(s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	return services.ReconciliationData{
		Year: 2025,
		Categories: []services.CategoryReconciliation{
			{
				Category:     model.CategoryCurrent,
				Requested:    m("1000.00"),
				Granted:      m("900.00"),
				Executed:     m("800.00"),
				Assigned:     m("800.00"),
				NetDeviation: m("100.00"),
				Subtypes: []services.SubtypeReconciliation{
					{
						Code:      "a6",
						Label:     "[a6]",
						Requested: m("1000.00"),
						Granted:   m("900.00"),
						Executed:  m("800.00"),
						Assigned:  m("800.00"),
						Deviation: m("100.00"),
						Concessions: []services.ConcessionReconciliation{
							{
								GroupCode:  "A6-01",
								Concept:    "Adob orgànic",
								Requested:  m("1000.00"),
								Granted:    m("900.00"),
								Executed:   m("800.00"),
								Assigned:   m("800.00"),
								Difference: m("100.00"),
								Forecasts: []services.ForecastReconciliation{
									{
										ForecastID:  "CP25001",
										PartnerID:   7,
										Concept:     "Fertilitzant",
										GrossAmount: m("500.00"),
										Executed:    m("400.00"),
										Assigned:    m("400.00"),
										Status:      services.StatusPartiallyJustified,
										Invoices: []services.InvoiceContribution{
											{
												InvoiceID:    1,
												Issuer:       "Campaner",
												Number:       "F1",
												IssueDate:    time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
												LinkedAmount: m("400.00"),
												FullyPaid:    true,
											},
										},
									},
									{
										ForecastID:  "CP25002",
										PartnerID:   1,
										Concept:     "Herbicida",
										GrossAmount: m("500.00"),
										Executed:    m("400.00"),
										Assigned:    m("400.00"),
										Status:      services.StatusPaymentPending,
										Invoices: []services.InvoiceContribution{
											{
												InvoiceID:    2,
												Issuer:       "Jardines",
												Number:       "F2",
												IssueDate:    time.Date(2025, 4, 1, 0, 0, 0, 0, time.UTC),
												LinkedAmount: m("400.00"),
												FullyPaid:    false,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestBuildReconciliationLayout_BlockStructure(t *testing.T) {
	rd := minimalReconData(t)
	blocks := buildReconciliationLayout(rd)

	// One category → no PageBreak; expected blocks:
	// [0] SectionTitle (category header)
	// [1] Table (category summary)
	// [2] SectionTitle (subtype)
	// [3] Table (concessions summary)
	// [4] Table (per-forecast for A6-01)
	if len(blocks) != 5 {
		t.Fatalf("len(blocks) = %d, want 5; blocks: %#v", len(blocks), blocks)
	}

	// [0] Category header contains "a6"
	st0, ok := blocks[0].(SectionTitle)
	if !ok {
		t.Fatalf("blocks[0] = %T, want SectionTitle", blocks[0])
	}
	if !strings.Contains(st0.Text, "a6") {
		t.Errorf("blocks[0].Text %q should contain subtype code", st0.Text)
	}

	// [1] Category summary table has correct headers
	tbl1, ok := blocks[1].(Table)
	if !ok {
		t.Fatalf("blocks[1] = %T, want Table", blocks[1])
	}
	if len(tbl1.Headers) < 5 {
		t.Errorf("category summary headers = %v", tbl1.Headers)
	}
	// last row is totals (Bold)
	last := tbl1.Rows[len(tbl1.Rows)-1]
	if !last.Bold {
		t.Errorf("last row of category summary should be Bold")
	}

	// [2] Subtype title "a6 — [a6]"
	st2, ok := blocks[2].(SectionTitle)
	if !ok {
		t.Fatalf("blocks[2] = %T, want SectionTitle", blocks[2])
	}
	if !strings.Contains(st2.Text, "a6") || !strings.Contains(st2.Text, "[a6]") {
		t.Errorf("blocks[2].Text = %q", st2.Text)
	}

	// [3] Concessions summary table — 1 concession row + 1 totals row
	tbl3, ok := blocks[3].(Table)
	if !ok {
		t.Fatalf("blocks[3] = %T, want Table", blocks[3])
	}
	if len(tbl3.Rows) != 2 {
		t.Errorf("concessions summary rows = %d, want 2", len(tbl3.Rows))
	}

	// [4] Per-forecast table — 2 forecast rows + 2 invoice follow-up rows
	tbl4, ok := blocks[4].(Table)
	if !ok {
		t.Fatalf("blocks[4] = %T, want Table", blocks[4])
	}
	if len(tbl4.Rows) != 4 {
		t.Errorf("per-forecast rows = %d, want 4 (2 forecasts + 2 invoice rows)", len(tbl4.Rows))
	}
	if !strings.Contains(tbl4.Title, "A6-01") {
		t.Errorf("per-forecast table title %q should contain A6-01", tbl4.Title)
	}
}

func TestBuildReconciliationLayout_TwoCategoriesHavePageBreak(t *testing.T) {
	rd := minimalReconData(t)
	// duplicate the category
	rd.Categories = append(rd.Categories, rd.Categories[0])
	blocks := buildReconciliationLayout(rd)

	// count PageBreaks and find the index
	var pageBreakIndex int
	pageBreakCount := 0
	for i, b := range blocks {
		if _, ok := b.(PageBreak); ok {
			pageBreakIndex = i
			pageBreakCount++
		}
	}

	// verify exactly one PageBreak
	if pageBreakCount != 1 {
		t.Errorf("expected exactly 1 PageBreak, found %d", pageBreakCount)
	}

	// verify PageBreak is not at the start (index 0)
	if pageBreakIndex == 0 {
		t.Error("PageBreak should not be at index 0 (before all content)")
	}

	// verify PageBreak is not at the end (len(blocks)-1)
	if pageBreakIndex == len(blocks)-1 {
		t.Error("PageBreak should not be at the last index (after all content)")
	}
}

func TestStatusLabel(t *testing.T) {
	cases := []struct {
		s    services.ForecastReconStatus
		want string
	}{
		{services.StatusFullyJustified, "Justificat"},
		{services.StatusPartiallyJustified, "Parcial"},
		{services.StatusOverExecuted, "Sobre-executat"},
		{services.StatusPaymentPending, "Pendent pagament"},
		{services.StatusNoInvoice, "Sense factura"},
	}
	for _, c := range cases {
		if got := statusLabel(c.s); got != c.want {
			t.Errorf("statusLabel(%d) = %q, want %q", c.s, got, c.want)
		}
	}
}
