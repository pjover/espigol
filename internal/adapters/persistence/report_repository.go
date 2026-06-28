package persistence

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence/mapper"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

type ReportRepository struct {
	q *sqlc.Queries
}

func NewReportRepository(q *sqlc.Queries) *ReportRepository {
	return &ReportRepository{q: q}
}

func (r *ReportRepository) Insert(ctx context.Context, rep model.Report) (int, error) {
	id, err := r.q.InsertReport(ctx, mapper.ReportToInsert(rep))
	if err != nil {
		return 0, err
	}
	return int(id), nil
}

func (r *ReportRepository) FindLatestByYear(ctx context.Context, year int) (model.Report, bool, error) {
	row, err := r.q.GetLatestReportByYear(ctx, int64(year))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Report{}, false, nil
	}
	if err != nil {
		return model.Report{}, false, err
	}
	rep, err := mapper.ReportFromRow(row)
	return rep, err == nil, err
}

func (r *ReportRepository) MarkSuperseded(ctx context.Context, id int, at time.Time) error {
	return r.q.MarkReportSuperseded(ctx, sqlc.MarkReportSupersededParams{
		SupersededAt: mapper.FormatNullableTimestamp(&at),
		ID:           int64(id),
	})
}
