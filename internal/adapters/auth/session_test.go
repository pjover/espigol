package auth_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/auth"
	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

// mutClock is a mutable clock for tests. Its time can be updated in place.
type mutClock struct{ t time.Time }

func (c *mutClock) Now() time.Time { return c.t }

func newStore(t *testing.T, now time.Time) (*auth.SessionStore, *sqlc.Queries, *mutClock) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	q := sqlc.New(conn)
	// a partner for the FK
	p, _ := model.NewPartner(1, "Soci", "", "", "s1@e.test", "", model.Productor, 0,
		time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), false)
	if err := persistence.NewPartnerRepository(q).Save(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	mc := &mutClock{t: now}
	return auth.NewSessionStore(q, mc), q, mc
}

func TestSessionStore_CreateGetDelete(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store, _, _ := newStore(t, now)
	ctx := context.Background()

	token, err := store.Create(ctx, 1, "s1@e.test")
	if err != nil || token == "" {
		t.Fatalf("create: token=%q err=%v", token, err)
	}
	s, ok, err := store.Get(ctx, token)
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if s.PartnerID != 1 || s.Email != "s1@e.test" {
		t.Errorf("session mismatch: %+v", s)
	}
	if err := store.Delete(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := store.Get(ctx, token); ok {
		t.Error("session should be gone after delete")
	}
}

func TestSessionStore_ExpiredIsAbsent(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	store, _, mc := newStore(t, now)
	ctx := context.Background()
	token, _ := store.Create(ctx, 1, "s1@e.test")

	// advance the mutable clock past the TTL
	mc.t = time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	if _, ok, _ := store.Get(ctx, token); ok {
		t.Error("expired session should be treated as absent")
	}
}
