package application_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

func fcNow() time.Time { return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) }

func newFcSvc(t *testing.T) (*application.ForecastService, *sql.DB) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "fc.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return application.NewForecastService(persistence.NewTxManager(conn), fixedClock{t: fcNow()}), conn
}

// seedOpen2026 builds an OPEN 2026 window with taxonomy a1(CURRENT)/b1(INVESTMENT),
// sections oliva/ramaderia, and partners 1 (soci) and 7 (board, authorized COMMON + SECTION oliva).
func seedOpen2026(t *testing.T, conn *sql.DB) {
	t.Helper()
	q := sqlc.New(conn)
	ctx := context.Background()
	w, _ := model.NewSubmissionWindow(2026, model.WindowOpen, ptrTime(fcNow()), nil,
		time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	_ = persistence.NewWindowRepository(q).Save(ctx, w)
	tax := persistence.NewTaxonomyRepository(q)
	ta, _ := model.NewExpenseType(2026, "A", "[a]", model.CategoryCurrent)
	tb, _ := model.NewExpenseType(2026, "B", "[b]", model.CategoryInvestment)
	_ = tax.SaveType(ctx, ta)
	_ = tax.SaveType(ctx, tb)
	sa, _ := model.NewExpenseSubtype(2026, "a1", "[a1]", "A")
	sb, _ := model.NewExpenseSubtype(2026, "b1", "[b1]", "B")
	_ = tax.SaveSubtype(ctx, sa)
	_ = tax.SaveSubtype(ctx, sb)
	sr := persistence.NewSectionRepository(q)
	oliva, _ := model.NewSection("oliva", "Secció d'oliva", true, 1)
	_ = sr.Save(ctx, oliva)
	pr := persistence.NewPartnerRepository(q)
	soci, _ := model.NewPartner(1, "Soci U", "Soci U", "", "", "u1@e.test", "", model.Productor, 0, fcNow(), false)
	board, _ := model.NewPartner(7, "Board", "Board", "", "", "b7@e.test", "", model.Productor, 0, fcNow(), true)
	_ = pr.Save(ctx, soci)
	_ = pr.Save(ctx, board)
	bar := persistence.NewBoardAuthorizationRepository(q)
	ac, _ := model.NewBoardAuthorization(7, model.ScopeCommon, "")
	as, _ := model.NewBoardAuthorization(7, model.ScopeSection, "oliva")
	_ = bar.Save(ctx, ac)
	_ = bar.Save(ctx, as)
}

func partner(t *testing.T, conn *sql.DB, id int) model.Partner {
	t.Helper()
	p, ok, err := persistence.NewPartnerRepository(sqlc.New(conn)).FindByID(context.Background(), id)
	if err != nil || !ok {
		t.Fatalf("partner %d: %v", id, err)
	}
	return p
}

func mustMoney(t *testing.T, s string) model.Money {
	t.Helper()
	m, err := model.MoneyFromString(s)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func partnerInput(amount string) application.ForecastInput {
	m, _ := model.MoneyFromString(amount)
	return application.ForecastInput{
		Concept: "Eines", Description: "", GrossAmount: m,
		PlannedDate: time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
		SubtypeCode: "b1", ScopeKind: model.ScopePartner,
	}
}

func TestForecastService_SociCreateAndOwnEdit(t *testing.T) {
	svc, conn := newFcSvc(t)
	seedOpen2026(t, conn)
	ctx := context.Background()
	soci := partner(t, conn, 1)

	f, err := svc.Create(ctx, soci, partnerInput("500.00"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if f.Partner().ID() != 1 || f.Scope().Kind() != model.ScopePartner || f.ID() == "" {
		t.Errorf("created wrong: %+v", f)
	}
	in := partnerInput("600.00")
	if err := svc.Update(ctx, soci, f.ID(), in); err != nil {
		t.Fatalf("update own: %v", err)
	}
	got, _ := svc.Get(ctx, soci, f.ID())
	if got.GrossAmount().String() != "600.00" {
		t.Errorf("update not applied: %s", got.GrossAmount())
	}
	if err := svc.Delete(ctx, soci, f.ID()); err != nil {
		t.Fatalf("delete own: %v", err)
	}
}

func TestForecastService_SociCannotTouchOthers(t *testing.T) {
	svc, conn := newFcSvc(t)
	seedOpen2026(t, conn)
	ctx := context.Background()
	board := partner(t, conn, 7)
	soci := partner(t, conn, 1)

	// board creates a COMMON forecast it is authorized for
	common := application.ForecastInput{Concept: "Comú", GrossAmount: mustMoney(t, "100.00"),
		PlannedDate: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), SubtypeCode: "a1", ScopeKind: model.ScopeCommon}
	cf, err := svc.Create(ctx, board, common)
	if err != nil {
		t.Fatalf("board create common: %v", err)
	}
	// a regular soci must not edit a COMMON forecast
	if err := svc.Update(ctx, soci, cf.ID(), common); !errors.Is(err, application.ErrForbidden) {
		t.Errorf("soci editing COMMON: want ErrForbidden, got %v", err)
	}
}

func TestForecastService_BoardScopeAuthorization(t *testing.T) {
	svc, conn := newFcSvc(t)
	seedOpen2026(t, conn)
	ctx := context.Background()
	board := partner(t, conn, 7)

	// authorized: SECTION oliva
	okIn := application.ForecastInput{Concept: "Oliva", GrossAmount: mustMoney(t, "200.00"),
		PlannedDate: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), SubtypeCode: "a1",
		ScopeKind: model.ScopeSection, SectionCode: "oliva"}
	if _, err := svc.Create(ctx, board, okIn); err != nil {
		t.Fatalf("board create authorized section: %v", err)
	}
	// unauthorized: SECTION ramaderia (board 7 only authorized for oliva + common)
	badIn := okIn
	badIn.SectionCode = "ramaderia"
	if _, err := svc.Create(ctx, board, badIn); !errors.Is(err, application.ErrForbidden) {
		t.Errorf("board create unauthorized section: want ErrForbidden, got %v", err)
	}
}

func TestForecastService_ListByYear(t *testing.T) {
	svc, conn := newFcSvc(t)
	seedOpen2026(t, conn)
	ctx := context.Background()
	soci := partner(t, conn, 1)
	board := partner(t, conn, 7)

	if _, err := svc.Create(ctx, soci, partnerInput("500.00")); err != nil {
		t.Fatalf("create soci forecast: %v", err)
	}
	common := application.ForecastInput{Concept: "Comú", GrossAmount: mustMoney(t, "100.00"),
		PlannedDate: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), SubtypeCode: "a1", ScopeKind: model.ScopeCommon}
	if _, err := svc.Create(ctx, board, common); err != nil {
		t.Fatalf("create board forecast: %v", err)
	}

	got, err := svc.ListByYear(ctx, 2026)
	if err != nil {
		t.Fatalf("ListByYear: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListByYear returned %d forecasts, want 2 (all partners)", len(got))
	}

	empty, err := svc.ListByYear(ctx, 2099)
	if err != nil {
		t.Fatalf("ListByYear empty year: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("ListByYear(2099) = %d items, want 0", len(empty))
	}
}

func TestForecastService_RejectsWhenNoOpenWindow(t *testing.T) {
	svc, conn := newFcSvc(t)
	// no window seeded
	_ = conn
	soci, _ := model.NewPartner(1, "X", "X", "", "", "x@e.test", "", model.Productor, 0, fcNow(), false)
	if _, err := svc.Create(context.Background(), soci, partnerInput("100.00")); !errors.Is(err, application.ErrNoOpenWindow) {
		t.Errorf("want ErrNoOpenWindow, got %v", err)
	}
}
