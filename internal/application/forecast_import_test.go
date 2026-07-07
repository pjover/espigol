package application_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

func impNow() time.Time { return time.Date(2025, 6, 1, 9, 0, 0, 0, time.UTC) }

// seedImportYear seeds a window (given state) for 2025 with type A / subtype a1,
// section "oliva", and partners 1 and 7. Returns the open conn and queries.
func seedImportYear(t *testing.T, state model.WindowState) (*application.ForecastService, *persistence.ForecastRepository, func(context.Context) []model.ExpenseForecast) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "imp.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	q := sqlc.New(conn)
	ctx := context.Background()

	var openedAt, closedAt *time.Time
	if state == model.WindowOpen || state == model.WindowClosed {
		o := impNow()
		openedAt = &o
	}
	if state == model.WindowClosed {
		c := impNow()
		closedAt = &c
	}
	w, _ := model.NewSubmissionWindow(2025, state, openedAt, closedAt,
		time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	if err := persistence.NewWindowRepository(q).Save(ctx, w); err != nil {
		t.Fatal(err)
	}
	tax := persistence.NewTaxonomyRepository(q)
	ta, _ := model.NewExpenseType(2025, "A", "[a]", model.CategoryCurrent)
	_ = tax.SaveType(ctx, ta)
	sa, _ := model.NewExpenseSubtype(2025, "a1", "[a1]", "A")
	_ = tax.SaveSubtype(ctx, sa)
	sec, _ := model.NewSection("oliva", "Secció d'oliva", true, 1)
	_ = persistence.NewSectionRepository(q).Save(ctx, sec)
	pr := persistence.NewPartnerRepository(q)
	for _, id := range []int{1, 7} {
		p, _ := model.NewPartner(id, "Soci", "", "", fmt.Sprintf("s%d@e.test", id), "", model.Productor, 0, impNow(), false)
		_ = pr.Save(ctx, p)
	}

	fr := persistence.NewForecastRepository(conn, q)
	svc := application.NewForecastService(persistence.NewTxManager(conn), fixedClock{t: impNow()})
	list := func(ctx context.Context) []model.ExpenseForecast {
		out, err := fr.ListByYear(ctx, 2025)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	return svc, fr, list
}

func commonEntry(partnerID int, gross string) application.ForecastImportEntry {
	amt, _ := model.MoneyFromString(gross)
	return application.ForecastImportEntry{
		PartnerID:   partnerID,
		Scope:       model.ScopeCommon,
		SubtypeCode: "a1",
		Concept:     "Concepte",
		GrossAmount: amt,
		PlannedDate: time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
	}
}

func TestAdminImport_ReplacesAllForYear(t *testing.T) {
	svc, fr, list := seedImportYear(t, model.WindowOpen)
	ctx := context.Background()

	// Pre-existing forecast that import must remove.
	amt, _ := model.MoneyFromString("999.00")
	pre, _ := model.NewUnsavedExpenseForecast(mustPartner(t, 1), "Vell", "", amt,
		model.ZeroMoney(), nil, time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC), 2025, "a1",
		model.NewCommonScope(), impNow(), true)
	if _, err := fr.Create(ctx, pre); err != nil {
		t.Fatal(err)
	}

	entries := []application.ForecastImportEntry{commonEntry(7, "2880.00"), commonEntry(1, "1200.00")}
	res, err := svc.AdminImport(ctx, "admin@espigol", 2025, entries)
	if err != nil {
		t.Fatalf("AdminImport: %v", err)
	}
	if res.Deleted != 1 || res.Created != 2 {
		t.Errorf("result = %+v, want {Deleted:1 Created:2}", res)
	}
	if got := len(list(ctx)); got != 2 {
		t.Errorf("forecasts after import = %d, want 2", got)
	}

	// Idempotent: re-running yields the same set (2), not 4.
	res2, err := svc.AdminImport(ctx, "admin@espigol", 2025, entries)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Deleted != 2 || res2.Created != 2 || len(list(ctx)) != 2 {
		t.Errorf("re-run not idempotent: res=%+v count=%d", res2, len(list(ctx)))
	}
}

func TestAdminImport_RequiresOpenWindow(t *testing.T) {
	svc, _, _ := seedImportYear(t, model.WindowDraft)
	_, err := svc.AdminImport(context.Background(), "admin@espigol", 2025,
		[]application.ForecastImportEntry{commonEntry(1, "100.00")})
	if !errors.Is(err, application.ErrWindowNotOpen) {
		t.Errorf("want ErrWindowNotOpen, got %v", err)
	}
}

func TestAdminImport_MissingPartnerRollsBack(t *testing.T) {
	svc, fr, list := seedImportYear(t, model.WindowOpen)
	ctx := context.Background()
	amt, _ := model.MoneyFromString("50.00")
	pre, _ := model.NewUnsavedExpenseForecast(mustPartner(t, 1), "Vell", "", amt,
		model.ZeroMoney(), nil, time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC), 2025, "a1",
		model.NewCommonScope(), impNow(), true)
	if _, err := fr.Create(ctx, pre); err != nil {
		t.Fatal(err)
	}

	entries := []application.ForecastImportEntry{commonEntry(99, "100.00")}
	if _, err := svc.AdminImport(ctx, "admin@espigol", 2025, entries); err == nil {
		t.Fatal("expected error for missing partner 99")
	}
	// Roll back: the pre-existing forecast must survive.
	if got := len(list(ctx)); got != 1 {
		t.Errorf("forecasts after failed import = %d, want 1 (unchanged)", got)
	}
}

// mustPartner builds a valid Partner value for constructing test forecasts.
func mustPartner(t *testing.T, id int) model.Partner {
	t.Helper()
	p, err := model.NewPartner(id, "Soci", "", "", "s@e.test", "", model.Productor, 0, impNow(), false)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
