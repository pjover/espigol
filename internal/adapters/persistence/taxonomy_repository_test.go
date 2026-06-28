package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/domain/model"
)

func TestTaxonomyRepository_RoundTrip(t *testing.T) {
	q := openTestDB(t)
	winRepo := persistence.NewWindowRepository(q)
	taxRepo := persistence.NewTaxonomyRepository(q)
	ctx := context.Background()

	// window FK first
	w, _ := model.NewSubmissionWindow(2026, model.WindowDraft, nil, nil,
		time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	if err := winRepo.Save(ctx, w); err != nil {
		t.Fatal(err)
	}

	typ, _ := model.NewExpenseType(2026, "A", "[a] Despeses corrents", model.CategoryCurrent)
	if err := taxRepo.SaveType(ctx, typ); err != nil {
		t.Fatal(err)
	}
	// a2/a3 share a label but are distinct codes (opaque-code quirk).
	st2, _ := model.NewExpenseSubtype(2026, "a2", "[a2] Activitats d'informació", "A")
	st3, _ := model.NewExpenseSubtype(2026, "a3", "[a2] Activitats d'informació", "A")
	if err := taxRepo.SaveSubtype(ctx, st2); err != nil {
		t.Fatal(err)
	}
	if err := taxRepo.SaveSubtype(ctx, st3); err != nil {
		t.Fatal(err)
	}

	types, err := taxRepo.ListTypes(ctx, 2026)
	if err != nil || len(types) != 1 || types[0].Category() != model.CategoryCurrent {
		t.Fatalf("ListTypes = (%+v, %v)", types, err)
	}
	subs, err := taxRepo.ListSubtypes(ctx, 2026)
	if err != nil || len(subs) != 2 {
		t.Fatalf("ListSubtypes len=%d err=%v", len(subs), err)
	}
	if subs[0].Code() == subs[1].Code() {
		t.Error("a2 and a3 must remain distinct codes")
	}
}
