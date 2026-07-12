// internal/domain/model/reconciliation_snapshot_test.go
package model_test

import (
	"testing"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
)

func TestNewReconciliationSnapshot_HappyPath(t *testing.T) {
	at := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
	snap, err := model.NewReconciliationSnapshot(2025, at, `{"year":2025}`, []byte("%PDF-"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.Year() != 2025 {
		t.Errorf("Year = %d, want 2025", snap.Year())
	}
	if snap.GeneratedAt() != at {
		t.Errorf("GeneratedAt = %v, want %v", snap.GeneratedAt(), at)
	}
	if snap.SnapshotJSON() != `{"year":2025}` {
		t.Errorf("SnapshotJSON = %q", snap.SnapshotJSON())
	}
	if string(snap.Pdf()) != "%PDF-" {
		t.Errorf("Pdf = %q", snap.Pdf())
	}
}

func TestNewReconciliationSnapshot_RejectsNegativeYear(t *testing.T) {
	_, err := model.NewReconciliationSnapshot(-1, time.Now(), `{"year":-1}`, nil)
	if err == nil {
		t.Fatal("expected error for negative year")
	}
}

func TestNewReconciliationSnapshot_RejectsEmptySnapshotJSON(t *testing.T) {
	_, err := model.NewReconciliationSnapshot(2025, time.Now(), "", []byte("%PDF-"))
	if err == nil {
		t.Fatal("expected error for empty snapshotJSON")
	}
}
