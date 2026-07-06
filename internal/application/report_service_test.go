package application_test

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

// TestReportService_Preview_MatchesGoldenTotals seeds an OPEN 2026 window with
// the anonymized golden dataset (mirrors internal/domain/services/golden_test.go's
// goldenInput, minus explicit forecast ids since ForecastRepository.Create
// allocates its own CPYYnnn ids) and checks Preview reproduces the golden totals.
func TestReportService_Preview_MatchesGoldenTotals(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "report_svc.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	q := sqlc.New(conn)
	ctx := context.Background()

	wr := persistence.NewWindowRepository(q)
	tax := persistence.NewTaxonomyRepository(q)
	pr := persistence.NewPartnerRepository(q)
	sr := persistence.NewSectionRepository(q)
	fr := persistence.NewForecastRepository(conn, q)

	w, _ := model.NewSubmissionWindow(2026, model.WindowOpen, ptrTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), nil,
		time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	if err := wr.Save(ctx, w); err != nil {
		t.Fatal(err)
	}

	ta, _ := model.NewExpenseType(2026, "A", "[a]", model.CategoryCurrent)
	tb, _ := model.NewExpenseType(2026, "B", "[b]", model.CategoryInvestment)
	_ = tax.SaveType(ctx, ta)
	_ = tax.SaveType(ctx, tb)
	sa, _ := model.NewExpenseSubtype(2026, "a1", "[a1]", "A")
	sb, _ := model.NewExpenseSubtype(2026, "b1", "[b1]", "B")
	_ = tax.SaveSubtype(ctx, sa)
	_ = tax.SaveSubtype(ctx, sb)

	for _, id := range []int{1, 2, 4, 5, 6, 7, 8, 11} {
		email := "soci" + strconv.Itoa(id) + "@e.test"
		p, _ := model.NewPartner(id, "Soci", "", "", email, "", model.Productor, 0,
			time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), false)
		if err := pr.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
	}

	oliva, _ := model.NewSection("oliva", "Secció d'oliva", true, 1)
	ram, _ := model.NewSection("ramaderia", "Secció de ramaderia", true, 2)
	_ = sr.Save(ctx, oliva)
	_ = sr.Save(ctx, ram)

	planned := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	com := model.NewCommonScope()
	par := model.NewPartnerScope()
	olivaScope, _ := model.NewSectionScope("oliva")
	ramScope, _ := model.NewSectionScope("ramaderia")

	type fc struct {
		partnerID int
		gross     string
		scope     model.ExpenseScope
		subtype   string
	}
	forecasts := []fc{
		// CURRENT
		{7, "2880.00", com, "a1"},
		{1, "1200.00", olivaScope, "a1"},
		{1, "380.00", olivaScope, "a1"},
		{1, "4304.00", olivaScope, "a1"},
		{1, "13187.00", olivaScope, "a1"},
		{1, "650.00", olivaScope, "a1"},
		{1, "5640.00", ramScope, "a1"},
		{1, "1750.00", ramScope, "a1"},
		// INVESTMENT
		{7, "31900.00", com, "b1"},
		{1, "3398.00", olivaScope, "b1"},
		{11, "1800.00", par, "b1"},
		{11, "1585.00", par, "b1"},
		{2, "400.00", par, "b1"},
		{2, "3085.00", par, "b1"},
		{2, "1962.00", par, "b1"},
		{2, "3270.00", par, "b1"},
		{2, "450.00", par, "b1"},
		{5, "70.00", par, "b1"},
		{5, "124.00", par, "b1"},
		{5, "1455.00", par, "b1"},
		{5, "191.00", par, "b1"},
		{5, "760.00", par, "b1"},
		{5, "148.00", par, "b1"},
		{6, "3719.00", par, "b1"},
		{4, "1322.22", par, "b1"},
		{7, "700.00", par, "b1"},
		{7, "638.74", par, "b1"},
		{8, "1819.00", par, "b1"},
	}
	for _, f := range forecasts {
		amt, err := model.MoneyFromString(f.gross)
		if err != nil {
			t.Fatal(err)
		}
		p, ok, err := pr.FindByID(ctx, f.partnerID)
		if err != nil || !ok {
			t.Fatalf("partner %d: %v", f.partnerID, err)
		}
		uf, err := model.NewUnsavedExpenseForecast(p, "Concepte", "", amt, model.ZeroMoney(), nil,
			planned, 2026, f.subtype, f.scope, planned, true)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fr.Create(ctx, uf); err != nil {
			t.Fatal(err)
		}
	}

	svc := application.NewReportService(persistence.NewTxManager(conn))
	rd, err := svc.Preview(ctx, 2026)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}

	if rd.HasNegativeRemainder {
		t.Errorf("HasNegativeRemainder should be false for 2026 golden data")
	}
	cur := rd.Categories[0]
	if cur.Category != model.CategoryCurrent {
		t.Fatal("category 0 must be CURRENT")
	}
	if cur.Common.Total.String() != "2880.00" {
		t.Errorf("current common total = %s, want 2880.00", cur.Common.Total.String())
	}
	if cur.Sections.Total.String() != "27111.00" {
		t.Errorf("current sections total = %s, want 27111.00", cur.Sections.Total.String())
	}

	inv := rd.Categories[1]
	if inv.Category != model.CategoryInvestment {
		t.Fatal("category 1 must be INVESTMENT")
	}
	if inv.Common.Total.String() != "31900.00" {
		t.Errorf("investment common total = %s, want 31900.00", inv.Common.Total.String())
	}
	if inv.Sections.Partners.GrandTotal.String() != "23498.96" {
		t.Errorf("investment socis grandTotal = %s, want 23498.96", inv.Sections.Partners.GrandTotal.String())
	}

	// Preview must not persist anything: no Report stored, forecasts not approved.
	if _, ok, _ := persistence.NewReportRepository(q).FindLatestByYear(ctx, 2026); ok {
		t.Errorf("Preview must not store a Report")
	}
}

func TestReportService_Preview_UnknownYear(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "report_svc_unknown.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	svc := application.NewReportService(persistence.NewTxManager(conn))
	if _, err := svc.Preview(context.Background(), 2099); !errors.Is(err, application.ErrWindowNotFound) {
		t.Errorf("want ErrWindowNotFound, got %v", err)
	}
}

func TestReportService_Latest(t *testing.T) {
	wsvc, conn := newSvc(t)
	seedOpenYearWithForecasts(t, conn)
	ctx := context.Background()

	rep, err := wsvc.Close(ctx, 2027)
	if err != nil {
		t.Fatal(err)
	}

	svc := application.NewReportService(persistence.NewTxManager(conn))
	got, ok, err := svc.Latest(ctx, 2027)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("want ok=true for closed year with stored report")
	}
	if got.ID() != rep.ID() {
		t.Errorf("latest report id = %d, want %d", got.ID(), rep.ID())
	}

	_, ok, err = svc.Latest(ctx, 2099)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("want ok=false for a year with no stored report")
	}
}
