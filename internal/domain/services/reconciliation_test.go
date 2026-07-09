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

func TestReconciliation_Group_UnderRun_AssignedEqualsExecuted(t *testing.T) {
	// Granted 100, Executed 60 → Assigned = 60, prorated to forecasts
	f1 := mkForecastForReconciliation(t, "CP25001", 7, "a2", "70.00")
	f2 := mkForecastForReconciliation(t, "CP25002", 7, "a2", "30.00")
	invGranted, _ := model.MoneyFromString("100.00")
	c, _ := model.NewConcession(2025, "A2-01", "a2", "concept", model.MoneyOf(100), invGranted)
	l1, _ := model.NewConcessionForecast(2025, "A2-01", "CP25001")
	l2, _ := model.NewConcessionForecast(2025, "A2-01", "CP25002")
	invA := mkInvoice(t, 10, 2025, "40.00", "40.00", "CP25001", "40.00")
	invB := mkInvoice(t, 11, 2025, "20.00", "20.00", "CP25002", "20.00")

	in := ReconciliationInput{
		Year: 2025, Forecasts: []model.ExpenseForecast{f1, f2},
		Concessions: []model.Concession{c},
		Links:       []model.ConcessionForecast{l1, l2},
		Invoices:    []model.Invoice{invA, invB},
	}
	exec := executedAndPending(in)
	groups, assigned := assignForGroups(in, exec)

	if got := groups["A2-01"].Base.String(); got != "60.00" {
		t.Errorf("Base = %s, want 60.00", got)
	}
	if got := assigned["CP25001"].String(); got != "40.00" {
		t.Errorf("CP25001 assigned = %s, want 40.00", got)
	}
	if got := assigned["CP25002"].String(); got != "20.00" {
		t.Errorf("CP25002 assigned = %s, want 20.00", got)
	}
}

func TestReconciliation_Group_OverRun_CappedAtGranted(t *testing.T) {
	// Granted 100, Executed 150 → Assigned = 100, prorated by share of Executed
	f1 := mkForecastForReconciliation(t, "CP25001", 7, "a2", "70.00")
	f2 := mkForecastForReconciliation(t, "CP25002", 7, "a2", "30.00")
	c, _ := model.NewConcession(2025, "A2-01", "a2", "concept", model.MoneyOf(100), model.MoneyOf(100))
	l1, _ := model.NewConcessionForecast(2025, "A2-01", "CP25001")
	l2, _ := model.NewConcessionForecast(2025, "A2-01", "CP25002")
	invA := mkInvoice(t, 10, 2025, "100.00", "100.00", "CP25001", "100.00")
	invB := mkInvoice(t, 11, 2025, "50.00", "50.00", "CP25002", "50.00")

	in := ReconciliationInput{
		Year: 2025, Forecasts: []model.ExpenseForecast{f1, f2},
		Concessions: []model.Concession{c},
		Links:       []model.ConcessionForecast{l1, l2},
		Invoices:    []model.Invoice{invA, invB},
	}
	exec := executedAndPending(in)
	groups, assigned := assignForGroups(in, exec)

	if got := groups["A2-01"].Base.String(); got != "100.00" {
		t.Errorf("Base = %s, want 100.00", got)
	}
	// share 100/150 * 100 = 66.67 (largest remainder); 50/150 * 100 = 33.33
	sum := assigned["CP25001"].Plus(assigned["CP25002"])
	if sum.String() != "100.00" {
		t.Errorf("Σ Assigned = %s, want 100.00 exactly", sum.String())
	}
}

func TestReconciliation_LargestRemainder_ClosesCent(t *testing.T) {
	// Granted 100.00, two equal Executed of 33.33 each → Σ Executed = 66.66,
	// Base = min(100, 66.66) = 66.66. Since Executed == Base, Assigned == Executed.
	// The remainder rule matters when Base rounds unevenly across shares:
	// try Granted 100 with Executed 33.33 / 33.33 → shares are 50/50 of Base=66.66
	// → 33.33 each. Sum = 66.66 exactly. No leak.
	f1 := mkForecastForReconciliation(t, "CP25001", 7, "a2", "50.00")
	f2 := mkForecastForReconciliation(t, "CP25002", 7, "a2", "50.00")
	c, _ := model.NewConcession(2025, "A2-01", "a2", "concept", model.MoneyOf(100), model.MoneyOf(100))
	l1, _ := model.NewConcessionForecast(2025, "A2-01", "CP25001")
	l2, _ := model.NewConcessionForecast(2025, "A2-01", "CP25002")
	invA := mkInvoice(t, 10, 2025, "33.33", "33.33", "CP25001", "33.33")
	invB := mkInvoice(t, 11, 2025, "33.33", "33.33", "CP25002", "33.33")

	in := ReconciliationInput{
		Year: 2025, Forecasts: []model.ExpenseForecast{f1, f2},
		Concessions: []model.Concession{c}, Links: []model.ConcessionForecast{l1, l2},
		Invoices: []model.Invoice{invA, invB},
	}
	exec := executedAndPending(in)
	groups, assigned := assignForGroups(in, exec)

	base := groups["A2-01"].Base
	sum := assigned["CP25001"].Plus(assigned["CP25002"])
	if sum.Cmp(base) != 0 {
		t.Errorf("Σ Assigned = %s, want %s exactly (no rounding leak)", sum, base)
	}
}

func TestReconciliation_Status_NoInvoice(t *testing.T) {
	f := mkForecastForReconciliation(t, "CP25001", 7, "a2", "100.00")
	fx := forecastExec{Executed: model.ZeroMoney(), Pending: model.ZeroMoney()}
	got := statusFor(f, fx, groupResult{}, true)
	if got != StatusNoInvoice {
		t.Errorf("status = %v, want StatusNoInvoice", got)
	}
}

func TestReconciliation_Status_PaymentPending_WhenAnyLinkUnpaid(t *testing.T) {
	f := mkForecastForReconciliation(t, "CP25001", 7, "a2", "100.00")
	// Both paid and unpaid links → still PaymentPending
	pending, _ := model.MoneyFromString("30.00")
	execAmt, _ := model.MoneyFromString("70.00")
	fx := forecastExec{Executed: execAmt, Pending: pending, Invoices: []InvoiceContribution{{}, {}}}
	g := groupResult{Base: model.MoneyOf(100), Assigned: model.MoneyOf(100), Executed: execAmt}
	got := statusFor(f, fx, g, true)
	if got != StatusPaymentPending {
		t.Errorf("status = %v, want StatusPaymentPending", got)
	}
}

func TestReconciliation_Status_OverExecuted(t *testing.T) {
	f := mkForecastForReconciliation(t, "CP25001", 7, "a2", "50.00")
	// Paid Executed 80 > GrossAmount 50, no pending, group fully justified
	exec, _ := model.MoneyFromString("80.00")
	fx := forecastExec{Executed: exec, Pending: model.ZeroMoney(), Invoices: []InvoiceContribution{{}}}
	g := groupResult{Base: model.MoneyOf(100), Assigned: model.MoneyOf(100), Executed: exec}
	got := statusFor(f, fx, g, true)
	if got != StatusOverExecuted {
		t.Errorf("status = %v, want StatusOverExecuted", got)
	}
}

func TestReconciliation_Status_PartiallyJustified(t *testing.T) {
	f := mkForecastForReconciliation(t, "CP25001", 7, "a2", "100.00")
	// Executed 60 < GrossAmount 100, no pending, group Executed 60 < Granted 100
	exec, _ := model.MoneyFromString("60.00")
	granted, _ := model.MoneyFromString("100.00")
	fx := forecastExec{Executed: exec, Pending: model.ZeroMoney(), Invoices: []InvoiceContribution{{}}}
	g := groupResult{Base: exec, Assigned: exec, Executed: exec, Granted: granted}
	got := statusFor(f, fx, g, true /*hasGroup*/)
	if got != StatusPartiallyJustified {
		t.Errorf("status = %v, want StatusPartiallyJustified", got)
	}
}

func TestReconciliation_Status_FullyJustified(t *testing.T) {
	f := mkForecastForReconciliation(t, "CP25001", 7, "a2", "100.00")
	// Executed 100 == GrossAmount, group Executed >= Granted
	exec, _ := model.MoneyFromString("100.00")
	granted, _ := model.MoneyFromString("100.00")
	fx := forecastExec{Executed: exec, Pending: model.ZeroMoney(), Invoices: []InvoiceContribution{{}}}
	g := groupResult{Base: granted, Assigned: granted, Executed: exec, Granted: granted}
	got := statusFor(f, fx, g, true)
	if got != StatusFullyJustified {
		t.Errorf("status = %v, want StatusFullyJustified", got)
	}
}

// helpers to build taxonomy + partners for hierarchy tests
func taxTypesAndSubtypes(t *testing.T) ([]model.ExpenseType, []model.ExpenseSubtype) {
	t.Helper()
	tCurrent, _ := model.NewExpenseType(2025, "A", "Corrents", model.CategoryCurrent)
	tInvest, _ := model.NewExpenseType(2025, "B", "Inversió", model.CategoryInvestment)
	a2, _ := model.NewExpenseSubtype(2025, "a2", "Prom.", "A")
	a4, _ := model.NewExpenseSubtype(2025, "a4", "Preven.", "A")
	a6, _ := model.NewExpenseSubtype(2025, "a6", "Fert.", "A")
	b1, _ := model.NewExpenseSubtype(2025, "b1", "Maquin.", "B")
	b2, _ := model.NewExpenseSubtype(2025, "b2", "Etno.", "B")
	return []model.ExpenseType{tCurrent, tInvest}, []model.ExpenseSubtype{a2, a4, a6, b1, b2}
}

func TestReconciliation_Hierarchy_EmptyCategoryOmitted(t *testing.T) {
	// Only INVESTMENT concessions → output has 1 category
	f := mkForecastForReconciliation(t, "CP25001", 7, "b1", "100.00")
	c, _ := model.NewConcession(2025, "B1-01", "b1", "concept", model.MoneyOf(100), model.MoneyOf(100))
	l, _ := model.NewConcessionForecast(2025, "B1-01", "CP25001")
	inv := mkInvoice(t, 1, 2025, "100.00", "100.00", "CP25001", "100.00")
	types, subs := taxTypesAndSubtypes(t)

	in := ReconciliationInput{
		Year: 2025, Forecasts: []model.ExpenseForecast{f},
		Concessions: []model.Concession{c}, Links: []model.ConcessionForecast{l},
		Invoices: []model.Invoice{inv}, Types: types, Subtypes: subs,
	}
	got, err := ComputeReconciliation(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Categories) != 1 {
		t.Fatalf("Categories = %d, want 1", len(got.Categories))
	}
	if got.Categories[0].Category != model.CategoryInvestment {
		t.Errorf("Category = %v, want INVESTMENT", got.Categories[0].Category)
	}
}

func TestReconciliation_CategoryNetDeviation_A4OverA6Under(t *testing.T) {
	// a4 over-spent by 461, a6 under-spent by 879 → NetDeviation for CURRENT = 418
	fA4 := mkForecastForReconciliation(t, "CP25004", 7, "a4", "500.00")
	fA6 := mkForecastForReconciliation(t, "CP25006", 7, "a6", "1000.00")

	// a4 concession: Granted 500, Executed 961 → deviation −461
	cA4, _ := model.NewConcession(2025, "A4-01", "a4", "cA4",
		mustMoney("500.00"), mustMoney("500.00"))
	lA4, _ := model.NewConcessionForecast(2025, "A4-01", "CP25004")
	invA4 := mkInvoice(t, 1, 2025, "961.00", "961.00", "CP25004", "961.00")

	// a6 concession: Granted 1000, Executed 121 → deviation +879
	cA6, _ := model.NewConcession(2025, "A6-01", "a6", "cA6",
		mustMoney("1000.00"), mustMoney("1000.00"))
	lA6, _ := model.NewConcessionForecast(2025, "A6-01", "CP25006")
	invA6 := mkInvoice(t, 2, 2025, "121.00", "121.00", "CP25006", "121.00")

	types, subs := taxTypesAndSubtypes(t)
	in := ReconciliationInput{
		Year: 2025, Forecasts: []model.ExpenseForecast{fA4, fA6},
		Concessions: []model.Concession{cA4, cA6},
		Links:       []model.ConcessionForecast{lA4, lA6},
		Invoices:    []model.Invoice{invA4, invA6},
		Types:       types, Subtypes: subs,
	}
	got, _ := ComputeReconciliation(in)
	if len(got.Categories) != 1 {
		t.Fatalf("want 1 category, got %d", len(got.Categories))
	}
	cat := got.Categories[0]
	if cat.NetDeviation.String() != "418.00" {
		t.Errorf("NetDeviation = %s, want 418.00", cat.NetDeviation.String())
	}
	// Per-subtype deviations preserved (raw, not netted)
	subMap := map[string]SubtypeReconciliation{}
	for _, s := range cat.Subtypes {
		subMap[s.Code] = s
	}
	if got := subMap["a4"].Deviation.String(); got != "-461.00" {
		t.Errorf("a4 Deviation = %s, want -461.00", got)
	}
	if got := subMap["a6"].Deviation.String(); got != "879.00" {
		t.Errorf("a6 Deviation = %s, want 879.00", got)
	}
}

func TestReconciliation_DisabledForecastsSkipped(t *testing.T) {
	f := mkForecastForReconciliation(t, "CP25001", 7, "a2", "100.00")
	// A disabled forecast — build it directly with enabled=false
	planned := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	p, _ := model.NewPartner(7, "X", "Y", "V", "y@e.cat", "6", model.Productor, 1, planned, false)
	disabled, _ := model.NewExpenseForecast("CP25002", p, "d", "", model.MoneyOf(50), model.ZeroMoney(),
		nil, planned, 2025, "a2", model.NewCommonScope(), planned, false /*enabled=false*/)
	c, _ := model.NewConcession(2025, "A2-01", "a2", "concept", model.MoneyOf(150), model.MoneyOf(150))
	l1, _ := model.NewConcessionForecast(2025, "A2-01", "CP25001")
	l2, _ := model.NewConcessionForecast(2025, "A2-01", "CP25002")
	invA := mkInvoice(t, 1, 2025, "100.00", "100.00", "CP25001", "100.00")
	invB := mkInvoice(t, 2, 2025, "50.00", "50.00", "CP25002", "50.00")

	types, subs := taxTypesAndSubtypes(t)
	in := ReconciliationInput{
		Year: 2025, Forecasts: []model.ExpenseForecast{f, disabled},
		Concessions: []model.Concession{c},
		Links:       []model.ConcessionForecast{l1, l2},
		Invoices:    []model.Invoice{invA, invB},
		Types:       types, Subtypes: subs,
	}
	got, _ := ComputeReconciliation(in)
	// CP25002 should not appear anywhere; its 50 shouldn't be in Executed either.
	for _, cat := range got.Categories {
		for _, s := range cat.Subtypes {
			for _, cn := range s.Concessions {
				for _, fr := range cn.Forecasts {
					if fr.ForecastID == "CP25002" {
						t.Fatal("disabled forecast leaked into output")
					}
				}
			}
		}
	}

	// Verify Executed exclusion: exactly one category, one subtype, one concession
	if len(got.Categories) != 1 {
		t.Fatalf("Categories = %d, want 1", len(got.Categories))
	}
	cat := got.Categories[0]
	if len(cat.Subtypes) != 1 || len(cat.Subtypes[0].Concessions) != 1 {
		t.Fatalf("expected exactly one concession, got %+v", cat)
	}
	cn := cat.Subtypes[0].Concessions[0]
	if cn.Executed.String() != "100.00" {
		t.Errorf("Concession Executed = %s, want 100.00 (disabled forecast's invoice must be excluded)", cn.Executed.String())
	}
	if cn.Assigned.String() != "100.00" {
		t.Errorf("Concession Assigned = %s, want 100.00", cn.Assigned.String())
	}
}

