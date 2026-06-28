package persistence

import (
	"context"

	"github.com/pjover/espigol/internal/adapters/persistence/mapper"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

type AuditLog struct {
	q *sqlc.Queries
}

func NewAuditLog(q *sqlc.Queries) *AuditLog {
	return &AuditLog{q: q}
}

func (a *AuditLog) Append(ctx context.Context, e model.AuditEvent) error {
	return a.q.InsertAuditEvent(ctx, mapper.AuditToInsert(e))
}

func (a *AuditLog) List(ctx context.Context) ([]model.AuditEvent, error) {
	rows, err := a.q.ListAuditEvents(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.AuditEvent, 0, len(rows))
	for _, row := range rows {
		e, err := mapper.AuditFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}
