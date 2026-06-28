package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/domain/model"
)

func TestReportRepository_InsertAndLatest(t *testing.T) {
	q := openTestDB(t)
	winRepo := persistence.NewWindowRepository(q)
	repo := persistence.NewReportRepository(q)
	ctx := context.Background()

	w, _ := model.NewSubmissionWindow(2026, model.WindowClosed, nil, nil,
		time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	_ = winRepo.Save(ctx, w)

	gen := time.Date(2026, 6, 23, 15, 15, 59, 0, time.UTC)
	r1, _ := model.NewReport(0, 2026, gen, `{"a":1}`, []byte{0x25, 0x50}, nil)
	id1, err := repo.Insert(ctx, r1)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, found, err := repo.FindLatestByYear(ctx, 2026)
	if err != nil || !found || got.ID() != id1 {
		t.Fatalf("FindLatest: (%+v, %v, %v)", got, found, err)
	}
	if got.SnapshotJSON() != `{"a":1}` || len(got.Pdf()) != 2 {
		t.Errorf("round trip mismatch: json=%q pdfLen=%d", got.SnapshotJSON(), len(got.Pdf()))
	}

	// supersede r1, insert r2 -> latest is r2
	if err := repo.MarkSuperseded(ctx, id1, gen.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	r2, _ := model.NewReport(0, 2026, gen.Add(2*time.Hour), `{"a":2}`, []byte{0x25}, nil)
	id2, _ := repo.Insert(ctx, r2)
	latest, _, _ := repo.FindLatestByYear(ctx, 2026)
	if latest.ID() != id2 {
		t.Errorf("latest id = %d, want %d", latest.ID(), id2)
	}
}
