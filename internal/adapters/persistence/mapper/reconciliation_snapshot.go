package mapper

import (
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func ReconciliationSnapshotToUpsert(s model.ReconciliationSnapshot) sqlc.UpsertReconciliationSnapshotParams {
	return sqlc.UpsertReconciliationSnapshotParams{
		Year:         int64(s.Year()),
		GeneratedAt:  FormatTimestamp(s.GeneratedAt()),
		SnapshotJson: s.SnapshotJSON(),
		Pdf:          s.Pdf(),
	}
}

func ReconciliationSnapshotFromRow(r sqlc.ReconciliationSnapshot) (model.ReconciliationSnapshot, error) {
	at, err := ParseTimestamp(r.GeneratedAt)
	if err != nil {
		return model.ReconciliationSnapshot{}, err
	}
	return model.NewReconciliationSnapshot(int(r.Year), at, r.SnapshotJson, r.Pdf)
}
