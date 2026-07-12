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

func openTestDB(t *testing.T) *sqlc.Queries {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return sqlc.New(conn)
}

func TestPartnerRepository_RoundTrip(t *testing.T) {
	repo := persistence.NewPartnerRepository(openTestDB(t))
	ctx := context.Background()

	p, _ := model.NewPartner(1, "Pau", "Pau", "Bosch Palmer", "X1", "pau@e.cat", "600",
		model.Productor, 13937, time.Date(2023, 4, 21, 0, 0, 0, 0, time.UTC), true)
	if err := repo.Save(ctx, p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, found, err := repo.FindByID(ctx, 1)
	if err != nil || !found {
		t.Fatalf("FindByID: (%v, found=%v)", err, found)
	}
	if got.Name() != "Pau" || got.PartnerType() != model.Productor || !got.BoardMember() {
		t.Errorf("round trip mismatch: %+v", got)
	}
	if !got.AddedOn().Equal(p.AddedOn()) {
		t.Errorf("AddedOn = %v, want %v", got.AddedOn(), p.AddedOn())
	}

	byEmail, found, err := repo.FindByEmail(ctx, "pau@e.cat")
	if err != nil || !found || byEmail.ID() != 1 {
		t.Errorf("FindByEmail: (%+v, %v, %v)", byEmail, found, err)
	}

	all, err := repo.List(ctx)
	if err != nil || len(all) != 1 {
		t.Errorf("List: len=%d err=%v", len(all), err)
	}
}

func TestPartnerRepository_NotFound(t *testing.T) {
	repo := persistence.NewPartnerRepository(openTestDB(t))
	_, found, err := repo.FindByID(context.Background(), 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("expected found=false for missing partner")
	}
}
