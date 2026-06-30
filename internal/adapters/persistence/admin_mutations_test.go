package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/domain/model"
)

// TestAdminMutations exercises DeleteType/DeleteSubtype (taxonomy), RemoveMembershipsByPartner
// (section), and Remove (board authorization) against a real temp SQLite database.
func TestAdminMutations(t *testing.T) {
	ctx := context.Background()

	// --- taxonomy: delete type and subtype ---
	t.Run("DeleteTypeAndSubtype", func(t *testing.T) {
		q := openTestDB(t)
		winRepo := persistence.NewWindowRepository(q)
		taxRepo := persistence.NewTaxonomyRepository(q)

		// A SubmissionWindow is required as FK for expense_type/expense_subtype.
		w, _ := model.NewSubmissionWindow(2026, model.WindowDraft, nil, nil,
			time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
		if err := winRepo.Save(ctx, w); err != nil {
			t.Fatal(err)
		}

		typ, _ := model.NewExpenseType(2026, "A", "[a] Despeses corrents", model.CategoryCurrent)
		if err := taxRepo.SaveType(ctx, typ); err != nil {
			t.Fatal(err)
		}
		st, _ := model.NewExpenseSubtype(2026, "a1", "[a1] Sub-activitat", "A")
		if err := taxRepo.SaveSubtype(ctx, st); err != nil {
			t.Fatal(err)
		}

		// Precondition: 1 type, 1 subtype.
		types, err := taxRepo.ListTypes(ctx, 2026)
		if err != nil || len(types) != 1 {
			t.Fatalf("precondition: ListTypes = (%v, %v)", types, err)
		}
		subs, err := taxRepo.ListSubtypes(ctx, 2026)
		if err != nil || len(subs) != 1 {
			t.Fatalf("precondition: ListSubtypes = (%v, %v)", subs, err)
		}

		// Delete the subtype first (FK constraint).
		if err := taxRepo.DeleteSubtype(ctx, 2026, "a1"); err != nil {
			t.Fatalf("DeleteSubtype: %v", err)
		}
		subs, err = taxRepo.ListSubtypes(ctx, 2026)
		if err != nil || len(subs) != 0 {
			t.Fatalf("after DeleteSubtype: len=%d err=%v", len(subs), err)
		}

		// Now delete the type.
		if err := taxRepo.DeleteType(ctx, 2026, "A"); err != nil {
			t.Fatalf("DeleteType: %v", err)
		}
		types, err = taxRepo.ListTypes(ctx, 2026)
		if err != nil || len(types) != 0 {
			t.Fatalf("after DeleteType: len=%d err=%v", len(types), err)
		}
	})

	// --- section: remove memberships by partner ---
	t.Run("RemoveMembershipsByPartner", func(t *testing.T) {
		q := openTestDB(t)
		secRepo := persistence.NewSectionRepository(q)
		partnerRepo := persistence.NewPartnerRepository(q)

		oliva, _ := model.NewSection("oliva", "Secció d'oliva", true, 1)
		ram, _ := model.NewSection("ram", "Secció de ramaderia", true, 2)
		if err := secRepo.Save(ctx, oliva); err != nil {
			t.Fatal(err)
		}
		if err := secRepo.Save(ctx, ram); err != nil {
			t.Fatal(err)
		}

		p, _ := model.NewPartner(5, "Marta", "Vila", "X5", "m@e.cat", "6", model.Productor, 1,
			time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), false)
		if err := partnerRepo.Save(ctx, p); err != nil {
			t.Fatal(err)
		}

		m1, _ := model.NewPartnerSection(5, "oliva")
		m2, _ := model.NewPartnerSection(5, "ram")
		if err := secRepo.AddMembership(ctx, m1); err != nil {
			t.Fatal(err)
		}
		if err := secRepo.AddMembership(ctx, m2); err != nil {
			t.Fatal(err)
		}

		// Precondition: 2 memberships for partner 5.
		got, err := secRepo.ListMembershipsByPartner(ctx, 5)
		if err != nil || len(got) != 2 {
			t.Fatalf("precondition: memberships = (%v, %v)", got, err)
		}

		if err := secRepo.RemoveMembershipsByPartner(ctx, 5); err != nil {
			t.Fatalf("RemoveMembershipsByPartner: %v", err)
		}

		got, err = secRepo.ListMembershipsByPartner(ctx, 5)
		if err != nil || len(got) != 0 {
			t.Fatalf("after RemoveMembershipsByPartner: len=%d err=%v", len(got), err)
		}
	})

	// --- board authorization: remove ---
	t.Run("RemoveBoardAuthorization", func(t *testing.T) {
		q := openTestDB(t)
		pr := persistence.NewPartnerRepository(q)
		sr := persistence.NewSectionRepository(q)
		repo := persistence.NewBoardAuthorizationRepository(q)

		p, _ := model.NewPartner(9, "Jordi", "M", "X9", "j@e.cat", "6", model.Productor, 1,
			time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), true)
		if err := pr.Save(ctx, p); err != nil {
			t.Fatal(err)
		}
		oliva, _ := model.NewSection("oliva", "Secció d'oliva", true, 1)
		if err := sr.Save(ctx, oliva); err != nil {
			t.Fatal(err)
		}

		common, _ := model.NewBoardAuthorization(9, model.ScopeCommon, "")
		section, _ := model.NewBoardAuthorization(9, model.ScopeSection, "oliva")
		if err := repo.Save(ctx, common); err != nil {
			t.Fatal(err)
		}
		if err := repo.Save(ctx, section); err != nil {
			t.Fatal(err)
		}

		// Precondition: 2 authorizations.
		auths, err := repo.ListByPartner(ctx, 9)
		if err != nil || len(auths) != 2 {
			t.Fatalf("precondition: auths = (%v, %v)", auths, err)
		}

		// Remove the section-scoped one.
		if n, err := repo.Remove(ctx, 9, model.ScopeSection, "oliva"); err != nil || n != 1 {
			t.Fatalf("Remove(section): n=%d err=%v", n, err)
		}
		auths, err = repo.ListByPartner(ctx, 9)
		if err != nil || len(auths) != 1 {
			t.Fatalf("after Remove(section): len=%d err=%v", len(auths), err)
		}

		// Remove the common one (sectionCode = ""), proving the NULL-safe match.
		if n, err := repo.Remove(ctx, 9, model.ScopeCommon, ""); err != nil || n != 1 {
			t.Fatalf("Remove(common): n=%d err=%v", n, err)
		}
		auths, err = repo.ListByPartner(ctx, 9)
		if err != nil || len(auths) != 0 {
			t.Fatalf("after Remove(common): len=%d err=%v", len(auths), err)
		}

		// Removing again matches nothing.
		if n, err := repo.Remove(ctx, 9, model.ScopeCommon, ""); err != nil || n != 0 {
			t.Fatalf("Remove(common) no-op: n=%d err=%v", n, err)
		}
	})
}
