package persistence_test

import (
	"context"
	"testing"

	"github.com/pjover/espigol/internal/adapters/persistence"
)

func TestAppStateRepository_ActiveYearRoundTrip(t *testing.T) {
	repo := persistence.NewAppStateRepository(openTestDB(t))
	ctx := context.Background()

	// Nothing stored yet → found=false.
	if _, found, err := repo.ActiveYear(ctx); err != nil || found {
		t.Fatalf("ActiveYear on empty = (found=%v, err=%v), want (false, nil)", found, err)
	}

	if err := repo.SetActiveYear(ctx, 2026); err != nil {
		t.Fatalf("SetActiveYear: %v", err)
	}
	year, found, err := repo.ActiveYear(ctx)
	if err != nil || !found || year != 2026 {
		t.Fatalf("ActiveYear after set = (%d, %v, %v), want (2026, true, nil)", year, found, err)
	}

	// Upsert: a second Set overwrites the single row.
	if err := repo.SetActiveYear(ctx, 2027); err != nil {
		t.Fatalf("SetActiveYear (update): %v", err)
	}
	if year, _, _ := repo.ActiveYear(ctx); year != 2027 {
		t.Errorf("ActiveYear after update = %d, want 2027", year)
	}
}
