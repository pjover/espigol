package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/domain/model"
)

func TestWindowRepository_RoundTrip(t *testing.T) {
	repo := persistence.NewWindowRepository(openTestDB(t))
	ctx := context.Background()

	deadline := time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC)
	w, _ := model.NewSubmissionWindow(2026, model.WindowClosed, nil, nil, deadline,
		model.MoneyOf(30000), model.MoneyOf(70000))
	if err := repo.Save(ctx, w); err != nil {
		t.Fatal(err)
	}

	got, found, err := repo.FindByYear(ctx, 2026)
	if err != nil || !found {
		t.Fatalf("FindByYear: (%v, %v)", found, err)
	}
	if got.State() != model.WindowClosed {
		t.Errorf("state = %q", got.State())
	}
	if got.CurrentExpenseLimit().String() != "30000.00" {
		t.Errorf("current limit = %q, want 30000.00", got.CurrentExpenseLimit().String())
	}
	if !got.Deadline().Equal(deadline) {
		t.Errorf("deadline = %v, want %v", got.Deadline(), deadline)
	}
	if got.OpenedAt() != nil {
		t.Errorf("OpenedAt should be nil, got %v", got.OpenedAt())
	}
}
