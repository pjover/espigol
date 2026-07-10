package persistence_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func seedWindow2025ForReconciliation(t *testing.T, q *sqlc.Queries) {
	t.Helper()
	w, err := model.NewSubmissionWindow(2025, model.WindowDraft, nil, nil,
		time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC), model.MoneyOf(30000), model.MoneyOf(70000))
	if err != nil {
		t.Fatal(err)
	}
	winRepo := persistence.NewWindowRepository(q)
	if err := winRepo.Save(context.Background(), w); err != nil {
		t.Fatal(err)
	}
}

func TestReconciliationSnapshotRepository_RoundTrip(t *testing.T) {
	q := openTestDB(t)
	seedWindow2025ForReconciliation(t, q)

	repo := persistence.NewReconciliationSnapshotRepository(q)
	ctx := context.Background()

	at := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
	snap, err := model.NewReconciliationSnapshot(2025, at, `{"year":2025}`, []byte("%PDF-1"))
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, snap); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, ok, err := repo.FindByYear(ctx, 2025)
	if err != nil {
		t.Fatalf("FindByYear: %v", err)
	}
	if !ok {
		t.Fatal("FindByYear: not found")
	}
	if got.Year() != 2025 {
		t.Errorf("Year = %d, want 2025", got.Year())
	}
	if got.SnapshotJSON() != `{"year":2025}` {
		t.Errorf("SnapshotJSON = %q", got.SnapshotJSON())
	}
	if string(got.Pdf()) != "%PDF-1" {
		t.Errorf("Pdf = %q", got.Pdf())
	}
}

func TestReconciliationSnapshotRepository_UpsertOverwrites(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	q := sqlc.New(conn)
	seedWindow2025ForReconciliation(t, q)

	repo := persistence.NewReconciliationSnapshotRepository(q)
	ctx := context.Background()

	at1 := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	at2 := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)

	s1, _ := model.NewReconciliationSnapshot(2025, at1, `{"year":2025,"v":1}`, []byte("v1"))
	s2, _ := model.NewReconciliationSnapshot(2025, at2, `{"year":2025,"v":2}`, []byte("v2"))
	if err := repo.Save(ctx, s1); err != nil {
		t.Fatalf("Save s1: %v", err)
	}
	if err := repo.Save(ctx, s2); err != nil {
		t.Fatalf("Save s2: %v", err)
	}

	// count must be 1 (upsert, not insert)
	var count int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM reconciliation_snapshot WHERE year=2025").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 row, got %d", count)
	}

	got, ok, _ := repo.FindByYear(ctx, 2025)
	if !ok || got.SnapshotJSON() != `{"year":2025,"v":2}` {
		t.Errorf("upsert did not overwrite: %q", got.SnapshotJSON())
	}
}

func TestReconciliationSnapshotRepository_UnknownYearReturnsFalse(t *testing.T) {
	repo := persistence.NewReconciliationSnapshotRepository(openTestDB(t))
	_, ok, err := repo.FindByYear(context.Background(), 9999)
	if err != nil {
		t.Fatalf("FindByYear: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for unknown year")
	}
}
