package persistence

import (
	"context"
	"database/sql"
	"errors"

	"github.com/pjover/espigol/internal/adapters/persistence/mapper"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

type ReconciliationSnapshotRepository struct {
	q *sqlc.Queries
}

func NewReconciliationSnapshotRepository(q *sqlc.Queries) *ReconciliationSnapshotRepository {
	return &ReconciliationSnapshotRepository{q: q}
}

func (r *ReconciliationSnapshotRepository) Save(ctx context.Context, s model.ReconciliationSnapshot) error {
	return r.q.UpsertReconciliationSnapshot(ctx, mapper.ReconciliationSnapshotToUpsert(s))
}

func (r *ReconciliationSnapshotRepository) FindByYear(ctx context.Context, year int) (model.ReconciliationSnapshot, bool, error) {
	row, err := r.q.GetReconciliationSnapshotByYear(ctx, int64(year))
	if errors.Is(err, sql.ErrNoRows) {
		return model.ReconciliationSnapshot{}, false, nil
	}
	if err != nil {
		return model.ReconciliationSnapshot{}, false, err
	}
	snap, err := mapper.ReconciliationSnapshotFromRow(row)
	return snap, err == nil, err
}
