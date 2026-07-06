package application_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

// seedOpenYearWithForecasts builds an OPEN 2027 window with a tiny scenario:
// common (current) 100, a soci (partner) investment 500; limits 200/1000.
func seedOpenYearWithForecasts(t *testing.T, conn *sql.DB) {
	t.Helper()
	q := sqlc.New(conn)
	ctx := context.Background()
	wr := persistence.NewWindowRepository(q)
	tax := persistence.NewTaxonomyRepository(q)
	pr := persistence.NewPartnerRepository(q)
	sr := persistence.NewSectionRepository(q)
	fr := persistence.NewForecastRepository(conn, q)

	w, _ := model.NewSubmissionWindow(2027, model.WindowOpen, ptrTime(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)), nil,
		time.Date(2027, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(200), model.MoneyOf(1000))
	_ = wr.Save(ctx, w)
	ta, _ := model.NewExpenseType(2027, "A", "[a]", model.CategoryCurrent)
	tb, _ := model.NewExpenseType(2027, "B", "[b]", model.CategoryInvestment)
	_ = tax.SaveType(ctx, ta)
	_ = tax.SaveType(ctx, tb)
	sa, _ := model.NewExpenseSubtype(2027, "a1", "[a1]", "A")
	sb, _ := model.NewExpenseSubtype(2027, "b1", "[b1]", "B")
	_ = tax.SaveSubtype(ctx, sa)
	_ = tax.SaveSubtype(ctx, sb)
	p, _ := model.NewPartner(1, "Soci 1", "", "", "s1@e.test", "", model.Productor, 1, time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), false)
	_ = pr.Save(ctx, p)

	oliva, _ := model.NewSection("oliva", "Secció d'oliva", true, 1)
	_ = sr.Save(ctx, oliva)

	planned := time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	common, _ := model.NewUnsavedExpenseForecast(p, "Comú", "", model.MoneyOf(100), model.ZeroMoney(), nil, planned, 2027, "a1", model.NewCommonScope(), planned, true)
	soci, _ := model.NewUnsavedExpenseForecast(p, "Soci", "", model.MoneyOf(500), model.ZeroMoney(), nil, planned, 2027, "b1", model.NewPartnerScope(), planned, true)
	secScope, _ := model.NewSectionScope("oliva")
	sec, _ := model.NewUnsavedExpenseForecast(p, "Secció oliva", "", model.MoneyOf(50), model.ZeroMoney(), nil, planned, 2027, "b1", secScope, planned, true)
	if _, err := fr.Create(ctx, common); err != nil {
		t.Fatal(err)
	}
	if _, err := fr.Create(ctx, soci); err != nil {
		t.Fatal(err)
	}
	if _, err := fr.Create(ctx, sec); err != nil {
		t.Fatal(err)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestClose_PersistsApprovedAndReport(t *testing.T) {
	svc, conn := newSvc(t)
	seedOpenYearWithForecasts(t, conn)
	ctx := context.Background()

	rep, err := svc.Close(ctx, 2027)
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if rep.Year() != 2027 || rep.SnapshotJSON() == "" {
		t.Errorf("report wrong: %+v", rep)
	}

	// window CLOSED
	w, _, _ := persistence.NewWindowRepository(sqlc.New(conn)).FindByYear(ctx, 2027)
	if w.State() != model.WindowClosed || w.ClosedAt() == nil {
		t.Errorf("window not closed: %+v", w)
	}
	// forecasts got approvedAmount + approvedOn
	fs, _ := persistence.NewForecastRepository(conn, sqlc.New(conn)).ListByYear(ctx, 2027)
	for _, f := range fs {
		if f.ApprovedOn() == nil {
			t.Errorf("forecast %s missing approvedOn", f.ID())
		}
	}
	// snapshot deserializes; common total 100
	rd, err := application.SnapshotFromJSON(rep.SnapshotJSON())
	if err != nil {
		t.Fatal(err)
	}
	if rd.Categories[0].Common.Total.String() != "100.00" {
		t.Errorf("snapshot common total = %s, want 100.00", rd.Categories[0].Common.Total.String())
	}
	// audit WINDOW_CLOSED
	audits, _ := persistence.NewAuditLog(sqlc.New(conn)).List(ctx)
	var sawClose bool
	for _, a := range audits {
		if a.Kind() == model.AuditWindowClosed {
			sawClose = true
		}
	}
	if !sawClose {
		t.Errorf("no WINDOW_CLOSED audit")
	}
}

func TestClose_RejectsNonOpen(t *testing.T) {
	svc, conn := newSvc(t)
	seedClosedYear(t, conn, 2026)
	ctx := context.Background()
	if _, err := svc.Close(ctx, 2026); !errors.Is(err, application.ErrWrongState) {
		t.Errorf("want ErrWrongState closing a CLOSED window, got %v", err)
	}
}
