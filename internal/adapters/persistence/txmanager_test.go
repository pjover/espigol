package persistence_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence"
	"github.com/pjover/espigol/internal/adapters/persistence/db"
	sqlc "github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/ports"
)

func sqlcQueries(conn *sql.DB) *sqlc.Queries { return sqlc.New(conn) }

func newTxManager(t *testing.T) (*persistence.TxManager, ports.PartnerRepository) {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "tx.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	tm := persistence.NewTxManager(conn)
	// a non-tx repo for reading committed state
	return tm, persistence.NewPartnerRepository(sqlcQueries(conn))
}

func samplePartner(t *testing.T, id int) model.Partner {
	t.Helper()
	p, err := model.NewPartner(id, "P", "", "", "p"+string(rune('0'+id))+"@e.test", "", model.Productor, 1, time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC), false)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTxManager_CommitsOnSuccess(t *testing.T) {
	tm, reader := newTxManager(t)
	ctx := context.Background()
	if err := tm.WithinTx(ctx, func(r ports.RepoSet) error {
		return r.Partners.Save(ctx, samplePartner(t, 1))
	}); err != nil {
		t.Fatal(err)
	}
	_, found, err := reader.FindByID(ctx, 1)
	if err != nil || !found {
		t.Errorf("partner should be committed: found=%v err=%v", found, err)
	}
}

func TestTxManager_RollsBackOnError(t *testing.T) {
	tm, reader := newTxManager(t)
	ctx := context.Background()
	sentinel := errors.New("boom")
	err := tm.WithinTx(ctx, func(r ports.RepoSet) error {
		if e := r.Partners.Save(ctx, samplePartner(t, 2)); e != nil {
			return e
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel, got %v", err)
	}
	_, found, _ := reader.FindByID(ctx, 2)
	if found {
		t.Errorf("partner must have been rolled back")
	}
}
