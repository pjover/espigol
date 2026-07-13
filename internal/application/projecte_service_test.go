package application_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/application"
	"github.com/pjover/espigol/internal/domain/model"
)

func TestProjecteService_ComputeReadsForecastsAndTaxonomy(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "proj.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	q := sqlc.New(conn)
	ctx := context.Background()

	win := persistence.NewWindowRepository(q)
	tax := persistence.NewTaxonomyRepository(q)
	pr := persistence.NewPartnerRepository(q)
	fr := persistence.NewForecastRepository(conn, q)

	w, _ := model.NewSubmissionWindow(2025, model.WindowOpen, nil, nil,
		time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	_ = win.Save(ctx, w)
	ta, _ := model.NewExpenseType(2025, "A", "Despeses corrents", model.CategoryCurrent)
	_ = tax.SaveType(ctx, ta)
	sa, _ := model.NewExpenseSubtype(2025, "a6", "Despeses de fertilitzants", "A")
	_ = tax.SaveSubtype(ctx, sa)
	planned := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	p, _ := model.NewPartner(7, "X", "X", "Y", "V", "x@e.cat", "6", model.Productor, 1, planned, false)
	_ = pr.Save(ctx, p)
	uf, _ := model.NewUnsavedExpenseForecast(p, "Adob orgànic", "d", model.MoneyOf(6580), model.ZeroMoney(),
		nil, planned, 2025, "a6", model.NewCommonScope(), planned, true)
	if _, err := fr.Create(ctx, uf); err != nil {
		t.Fatal(err)
	}

	svc := application.NewProjecteService(persistence.NewTxManager(conn))
	data, err := svc.Compute(ctx, 2025)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if data.Year != 2025 || data.Total.String() != "6580.00" {
		t.Errorf("Compute = year %d total %s, want 2025 / 6580.00", data.Year, data.Total.String())
	}
	if len(data.Tipus) != 1 || len(data.Tipus[0].Apartats) != 1 || data.Tipus[0].Apartats[0].Code != "a6" {
		t.Errorf("structure = %+v, want one tipus A with apartat a6", data.Tipus)
	}
}
