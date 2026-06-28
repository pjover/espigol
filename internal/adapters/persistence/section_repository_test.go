package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/domain/model"
)

func TestSectionRepository_RoundTrip(t *testing.T) {
	q := openTestDB(t)
	repo := persistence.NewSectionRepository(q)
	ctx := context.Background()

	oliva, _ := model.NewSection("oliva", "Secció d'oliva", true, 1)
	ram, _ := model.NewSection("ramaderia", "Secció de ramaderia", true, 2)
	if err := repo.Save(ctx, oliva); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, ram); err != nil {
		t.Fatal(err)
	}

	secs, err := repo.List(ctx)
	if err != nil || len(secs) != 2 || secs[0].Code() != "oliva" {
		t.Fatalf("List = (%+v, %v)", secs, err)
	}
	if secs[0].Label() != "Secció d'oliva" {
		t.Errorf("label round trip wrong: %q", secs[0].Label())
	}
}

func TestSectionRepository_Membership(t *testing.T) {
	q := openTestDB(t)
	secRepo := persistence.NewSectionRepository(q)
	partnerRepo := persistence.NewPartnerRepository(q)
	ctx := context.Background()

	// FK requires partner + section to exist first.
	oliva, _ := model.NewSection("oliva", "Secció d'oliva", true, 1)
	_ = secRepo.Save(ctx, oliva)
	p, _ := model.NewPartner(1, "Pau", "B", "X", "p@e.cat", "6", model.Productor, 1, time.Date(2023, 4, 21, 0, 0, 0, 0, time.UTC), false)
	_ = partnerRepo.Save(ctx, p)

	m, _ := model.NewPartnerSection(1, "oliva")
	if err := secRepo.AddMembership(ctx, m); err != nil {
		t.Fatal(err)
	}
	got, err := secRepo.ListMembershipsByPartner(ctx, 1)
	if err != nil || len(got) != 1 || got[0].SectionCode() != "oliva" {
		t.Fatalf("memberships = (%+v, %v)", got, err)
	}
}

func TestSectionRepository_ListMemberships(t *testing.T) {
	q := openTestDB(t)
	pr := persistence.NewPartnerRepository(q)
	sr := persistence.NewSectionRepository(q)
	ctx := context.Background()

	oliva, _ := model.NewSection("oliva", "Secció d'oliva", true, 1)
	ram, _ := model.NewSection("ramaderia", "Secció de ramaderia", true, 2)
	_ = sr.Save(ctx, oliva)
	_ = sr.Save(ctx, ram)
	p1, _ := model.NewPartner(1, "A", "", "", "a@e.test", "", model.Productor, 1, time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), false)
	p2, _ := model.NewPartner(2, "B", "", "", "b@e.test", "", model.Productor, 1, time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), false)
	_ = pr.Save(ctx, p1)
	_ = pr.Save(ctx, p2)
	m1, _ := model.NewPartnerSection(1, "oliva")
	m2, _ := model.NewPartnerSection(1, "ramaderia")
	m3, _ := model.NewPartnerSection(2, "oliva")
	for _, m := range []model.PartnerSection{m1, m2, m3} {
		if err := sr.AddMembership(ctx, m); err != nil {
			t.Fatal(err)
		}
	}

	all, err := sr.ListMemberships(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("ListMemberships = %d rows, want 3", len(all))
	}
}
