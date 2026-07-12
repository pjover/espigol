package persistence_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

// seedForYear inserts the window + a type + subtype + partner the forecast FKs require.
func seedForYear(t *testing.T, q *sqlc.Queries, year int) {
	t.Helper()
	ctx := context.Background()
	win := persistence.NewWindowRepository(q)
	tax := persistence.NewTaxonomyRepository(q)
	pr := persistence.NewPartnerRepository(q)
	w, _ := model.NewSubmissionWindow(year, model.WindowOpen, nil, nil,
		time.Date(year, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	if err := win.Save(ctx, w); err != nil {
		t.Fatal(err)
	}
	typ, _ := model.NewExpenseType(year, "A", "[a]", model.CategoryCurrent)
	_ = tax.SaveType(ctx, typ)
	st, _ := model.NewExpenseSubtype(year, "a1", "[a1]", "A")
	_ = tax.SaveSubtype(ctx, st)
	p, _ := model.NewPartner(7, "X", "X", "Y", "V", "x@e.cat", "6", model.Productor, 1,
		time.Date(2023, 4, 21, 0, 0, 0, 0, time.UTC), false)
	_ = pr.Save(ctx, p)
}

func newForecastRepo(t *testing.T) (*persistence.ForecastRepository, *sqlc.Queries) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	q := sqlc.New(conn)
	return persistence.NewForecastRepository(conn, q), q
}

func TestForecastRepository_CreateAllocatesIDAndRoundTrips(t *testing.T) {
	repo, q := newForecastRepo(t)
	seedForYear(t, q, 2026)
	ctx := context.Background()

	planned := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	p7, _ := model.NewPartner(7, "X", "X", "Y", "V", "x@e.cat", "6", model.Productor, 1, planned, false)
	f, _ := model.NewUnsavedExpenseForecast(p7, "Concepte", "desc", model.MoneyOf(2880), model.ZeroMoney(),
		nil, planned, 2026, "a1", model.NewCommonScope(), planned, true)

	created, err := repo.Create(ctx, f)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID() != "CP26001" {
		t.Errorf("first id = %q, want CP26001", created.ID())
	}

	second, err := repo.Create(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID() != "CP26002" {
		t.Errorf("second id = %q, want CP26002", second.ID())
	}

	got, found, err := repo.FindByID(ctx, "CP26001")
	if err != nil || !found {
		t.Fatalf("FindByID: (%v, %v)", found, err)
	}
	if got.GrossAmount().String() != "2880.00" || got.Scope().Kind() != model.ScopeCommon {
		t.Errorf("round trip mismatch: %+v", got)
	}
}

func TestForecastRepository_SectionScopeAndMoneyExactness(t *testing.T) {
	repo, q := newForecastRepo(t)
	seedForYear(t, q, 2026)
	ctx := context.Background()
	// section FK
	secRepo := persistence.NewSectionRepository(q)
	oliva, _ := model.NewSection("oliva", "Secció d'oliva", true, 1)
	_ = secRepo.Save(ctx, oliva)

	planned := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	sectionScope, _ := model.NewSectionScope("oliva")
	gross, _ := model.MoneyFromString("1322.22") // the former-REAL value
	p7, _ := model.NewPartner(7, "X", "X", "Y", "V", "x@e.cat", "6", model.Productor, 1, planned, false)
	f, _ := model.NewUnsavedExpenseForecast(p7, "C", "d", gross, model.ZeroMoney(),
		nil, planned, 2026, "a1", sectionScope, planned, true)

	created, err := repo.Create(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	got, _, _ := repo.FindByID(ctx, created.ID())
	if got.GrossAmount().String() != "1322.22" {
		t.Errorf("money exactness lost: %q", got.GrossAmount().String())
	}
	if got.Scope().Kind() != model.ScopeSection || got.Scope().SectionCode() != "oliva" {
		t.Errorf("section scope round trip wrong: %+v", got.Scope())
	}
}
