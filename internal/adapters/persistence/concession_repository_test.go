package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/domain/model"
)

func TestConcessionRepository_RoundTrip(t *testing.T) {
	fr, q := newForecastRepo(t)
	seedForYear(t, q, 2025)
	ctx := context.Background()

	planned := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	p7, _ := model.NewPartner(7, "X", "Y", "V", "x@e.cat", "6", model.Productor, 1, planned, false)
	uf, _ := model.NewUnsavedExpenseForecast(p7, "Adob", "d", model.MoneyOf(6580), model.ZeroMoney(),
		nil, planned, 2025, "a1", model.NewCommonScope(), planned, true)
	f, err := fr.Create(ctx, uf) // allocates CP25001
	if err != nil {
		t.Fatalf("create forecast: %v", err)
	}

	repo := persistence.NewConcessionRepository(q)
	c, _ := model.NewConcession(2025, "A6-02", "a1", "Adob orgànic", model.MoneyOf(13880), model.MoneyOf(13880))
	if err := repo.Save(ctx, c); err != nil {
		t.Fatalf("Save concession: %v", err)
	}
	if err := repo.ReplaceMembership(ctx, 2025, "A6-02", []string{f.ID()}); err != nil {
		t.Fatalf("ReplaceMembership: %v", err)
	}

	got, err := repo.ListByYear(ctx, 2025)
	if err != nil || len(got) != 1 || got[0].GrantedAmount().Cmp(model.MoneyOf(13880)) != 0 {
		t.Fatalf("ListByYear = (%+v, %v)", got, err)
	}
	links, err := repo.ListForecastLinksByYear(ctx, 2025)
	if err != nil || len(links) != 1 || links[0].ForecastID() != f.ID() {
		t.Fatalf("links = (%+v, %v)", links, err)
	}

	// Delete cascades membership.
	if err := repo.Delete(ctx, 2025, "A6-02"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, _ = repo.ListByYear(ctx, 2025)
	links, _ = repo.ListForecastLinksByYear(ctx, 2025)
	if len(got) != 0 || len(links) != 0 {
		t.Fatalf("after delete: concessions=%d links=%d", len(got), len(links))
	}
}
