package services

import (
	"testing"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
)

func TestReconciliation_EmptyInput_ReturnsEmptyData(t *testing.T) {
	in := ReconciliationInput{Year: 2025}
	got, err := ComputeReconciliation(in)
	if err != nil {
		t.Fatalf("ComputeReconciliation: %v", err)
	}
	if got.Year != 2025 {
		t.Errorf("Year = %d, want 2025", got.Year)
	}
	if len(got.Categories) != 0 {
		t.Errorf("Categories = %d, want 0", len(got.Categories))
	}
	// Enum values must be declared
	_ = StatusFullyJustified
	_ = StatusPartiallyJustified
	_ = StatusOverExecuted
	_ = StatusPaymentPending
	_ = StatusNoInvoice
	_ = model.ZeroMoney() // used later
}

// mkForecastForReconciliation is a compact ExpenseForecast constructor for these tests.
func mkForecastForReconciliation(t *testing.T, id string, partnerID int, subtypeCode string, gross string) model.ExpenseForecast {
	t.Helper()
	planned := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	p, err := model.NewPartner(partnerID, "X", "Y", "V", "x"+id+"@e.cat", "6", model.Productor, 1, planned, false)
	if err != nil {
		t.Fatal(err)
	}
	grossMoney, err := model.MoneyFromString(gross)
	if err != nil {
		t.Fatal(err)
	}
	f, err := model.NewExpenseForecast(id, p, "concept "+id, "", grossMoney, model.ZeroMoney(),
		nil, planned, 2025, subtypeCode, model.NewCommonScope(), planned, true)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// mkInvoice builds an Invoice with one payment and one link. paidTotal is
// how much has been paid (may be less than net → unpaid).
func mkInvoice(t *testing.T, id int, year int, net string, paidTotal string, forecastID string, linkAmount string) model.Invoice {
	t.Helper()
	netM, _ := model.MoneyFromString(net)
	paidM, _ := model.MoneyFromString(paidTotal)
	linkM, _ := model.MoneyFromString(linkAmount)
	issued := time.Date(year, 3, 1, 0, 0, 0, 0, time.UTC)
	paidOn := time.Date(year, 4, 1, 0, 0, 0, 0, time.UTC)

	var payments []model.InvoicePayment
	if !paidM.IsZero() {
		p := model.NewInvoicePayment(id, id, paidOn, paidM)
		payments = []model.InvoicePayment{p}
	}
	link, err := model.NewForecastInvoice(forecastID, id, linkM)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := model.NewInvoice(id, year, "Sup", "B1", "F"+forecastID, issued, netM, nil, nil, payments, []model.ForecastInvoice{link})
	if err != nil {
		t.Fatal(err)
	}
	return inv
}

func TestReconciliation_PaymentGate_UnpaidExcludedFromExecuted(t *testing.T) {
	f := mkForecastForReconciliation(t, "CP25001", 7, "a2", "100.00")
	unpaid := mkInvoice(t, 1, 2025, "60.00", "0.00", "CP25001", "60.00")
	partiallyPaid := mkInvoice(t, 2, 2025, "40.00", "20.00", "CP25001", "40.00")
	fullyPaid := mkInvoice(t, 3, 2025, "50.00", "50.00", "CP25001", "50.00")

	in := ReconciliationInput{
		Year:      2025,
		Forecasts: []model.ExpenseForecast{f},
		Invoices:  []model.Invoice{unpaid, partiallyPaid, fullyPaid},
	}
	m := executedAndPending(in)

	got := m["CP25001"]
	wantExec, _ := model.MoneyFromString("50.00")
	wantPend, _ := model.MoneyFromString("100.00")
	if got.Executed.Cmp(wantExec) != 0 {
		t.Errorf("Executed = %s, want 50.00", got.Executed.String())
	}
	if got.Pending.Cmp(wantPend) != 0 {
		t.Errorf("Pending = %s, want 100.00", got.Pending.String())
	}
	if len(got.Invoices) != 3 {
		t.Errorf("Invoices = %d, want 3", len(got.Invoices))
	}
	// Ordered by IssueDate then Number — all same date here, ensure Number order F<id>
	if got.Invoices[0].Number != "FCP25001" || !got.Invoices[2].FullyPaid {
		t.Errorf("invoices not in expected order/state: %+v", got.Invoices)
	}
}

func TestReconciliation_PaymentGate_ForecastWithNoLinks_ExecutedZero(t *testing.T) {
	f := mkForecastForReconciliation(t, "CP25002", 7, "a2", "50.00")
	in := ReconciliationInput{
		Year:      2025,
		Forecasts: []model.ExpenseForecast{f},
	}
	m := executedAndPending(in)
	got := m["CP25002"]
	if !got.Executed.IsZero() || !got.Pending.IsZero() {
		t.Errorf("empty forecast: executed=%s pending=%s", got.Executed, got.Pending)
	}
	if len(got.Invoices) != 0 {
		t.Errorf("Invoices should be empty, got %d", len(got.Invoices))
	}
}
