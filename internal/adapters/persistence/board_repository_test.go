package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/domain/model"
)

func TestBoardAuthorizationRepository_RoundTrip(t *testing.T) {
	q := openTestDB(t)
	pr := persistence.NewPartnerRepository(q)
	sr := persistence.NewSectionRepository(q)
	repo := persistence.NewBoardAuthorizationRepository(q)
	ctx := context.Background()

	p, _ := model.NewPartner(7, "X", "Y", "V", "x@e.cat", "6", model.Productor, 1,
		time.Date(2023, 4, 21, 0, 0, 0, 0, time.UTC), true)
	_ = pr.Save(ctx, p)
	oliva, _ := model.NewSection("oliva", "Secció d'oliva", true, 1)
	_ = sr.Save(ctx, oliva)

	common, _ := model.NewBoardAuthorization(7, model.ScopeCommon, "")
	section, _ := model.NewBoardAuthorization(7, model.ScopeSection, "oliva")
	if err := repo.Save(ctx, common); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, section); err != nil {
		t.Fatal(err)
	}
	// idempotent: saving common again must not error or duplicate.
	if err := repo.Save(ctx, common); err != nil {
		t.Fatal(err)
	}

	got, err := repo.ListByPartner(ctx, 7)
	if err != nil || len(got) != 2 {
		t.Fatalf("ListByPartner: len=%d err=%v", len(got), err)
	}
}
