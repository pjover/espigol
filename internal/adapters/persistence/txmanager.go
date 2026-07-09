package persistence

import (
	"context"
	"database/sql"

	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/ports"
)

// TxManager runs units of work in a single SQLite transaction.
type TxManager struct {
	db *sql.DB
}

func NewTxManager(db *sql.DB) *TxManager {
	return &TxManager{db: db}
}

// WithinTx begins a transaction, builds a tx-scoped RepoSet, runs fn, and
// commits on success or rolls back on error/panic.
func (t *TxManager) WithinTx(ctx context.Context, fn func(ports.RepoSet) error) (err error) {
	tx, err := t.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	q := sqlc.New(t.db).WithTx(tx)
	repos := ports.RepoSet{
		Partners:    NewPartnerRepository(q),
		Forecasts:   NewForecastRepository(t.db, q),
		Windows:     NewWindowRepository(q),
		Taxonomy:    NewTaxonomyRepository(q),
		Sections:    NewSectionRepository(q),
		Reports:     NewReportRepository(q),
		Audit:       NewAuditLog(q),
		BoardAuth:   NewBoardAuthorizationRepository(q),
		Concessions: NewConcessionRepository(q),
		Invoices:    NewInvoiceRepository(q),
	}
	if err := fn(repos); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

var _ ports.TxManager = (*TxManager)(nil)
