package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/domain/model"
)

func TestInvoiceRepository_RoundTrip(t *testing.T) {
	fr, q := newForecastRepo(t)
	seedForYear(t, q, 2025)
	ctx := context.Background()

	planned := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	p7, _ := model.NewPartner(7, "X", "X", "Y", "V", "x@e.cat", "6", model.Productor, 1, planned, false)
	uf, _ := model.NewUnsavedExpenseForecast(p7, "Màquina", "d", model.MoneyOf(500), model.ZeroMoney(),
		nil, planned, 2025, "a1", model.NewCommonScope(), planned, true)
	f, _ := fr.Create(ctx, uf)

	repo := persistence.NewInvoiceRepository(q)
	d := time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC)
	pay := model.NewInvoicePayment(0, 0, d, model.MoneyOf(500))
	link, _ := model.NewForecastInvoice(f.ID(), 0, model.MoneyOf(500))
	inv, _ := model.NewInvoice(0, 2025, "Ribot", "B999", "FD-39521", d, model.MoneyOf(500),
		nil, nil, []model.InvoicePayment{pay}, []model.ForecastInvoice{link})

	saved, err := repo.Save(ctx, inv)
	if err != nil || saved.ID() == 0 {
		t.Fatalf("Save = (%+v, %v)", saved, err)
	}

	got, err := repo.ListByYear(ctx, 2025)
	if err != nil || len(got) != 1 {
		t.Fatalf("ListByYear = (%d, %v)", len(got), err)
	}
	g := got[0]
	if g.Number() != "FD-39521" || len(g.Payments()) != 1 || len(g.Links()) != 1 {
		t.Fatalf("unexpected aggregate: %+v", g)
	}
	if g.PaidTotal().Cmp(model.MoneyOf(500)) != 0 || g.Links()[0].ForecastID() != f.ID() {
		t.Fatalf("children wrong: paid=%s link=%s", g.PaidTotal(), g.Links()[0].ForecastID())
	}

	if err := repo.Delete(ctx, g.ID()); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, _ = repo.ListByYear(ctx, 2025)
	if len(got) != 0 {
		t.Fatalf("after delete: %d invoices", len(got))
	}
}
