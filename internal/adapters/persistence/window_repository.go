package persistence

import (
	"context"
	"database/sql"
	"errors"

	"github.com/pjover/espigol/internal/adapters/persistence/mapper"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

type WindowRepository struct {
	q *sqlc.Queries
}

func NewWindowRepository(q *sqlc.Queries) *WindowRepository {
	return &WindowRepository{q: q}
}

func (r *WindowRepository) Save(ctx context.Context, w model.SubmissionWindow) error {
	return r.q.UpsertSubmissionWindow(ctx, mapper.WindowToRow(w))
}

func (r *WindowRepository) FindByYear(ctx context.Context, year int) (model.SubmissionWindow, bool, error) {
	row, err := r.q.GetSubmissionWindow(ctx, int64(year))
	if errors.Is(err, sql.ErrNoRows) {
		return model.SubmissionWindow{}, false, nil
	}
	if err != nil {
		return model.SubmissionWindow{}, false, err
	}
	w, err := mapper.WindowFromRow(row)
	return w, err == nil, err
}

func (r *WindowRepository) List(ctx context.Context) ([]model.SubmissionWindow, error) {
	rows, err := r.q.ListSubmissionWindows(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.SubmissionWindow, 0, len(rows))
	for _, row := range rows {
		w, err := mapper.WindowFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, nil
}
