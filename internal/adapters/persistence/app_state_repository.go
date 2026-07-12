package persistence

import (
	"context"
	"database/sql"
	"errors"

	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
)

// AppStateRepository persists the single-row app_state table (the TUI's
// last-selected year). It stores a raw scalar, so no domain mapper is needed.
type AppStateRepository struct {
	q *sqlc.Queries
}

func NewAppStateRepository(q *sqlc.Queries) *AppStateRepository {
	return &AppStateRepository{q: q}
}

func (r *AppStateRepository) ActiveYear(ctx context.Context) (int, bool, error) {
	year, err := r.q.GetActiveYear(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return int(year), true, nil
}

func (r *AppStateRepository) SetActiveYear(ctx context.Context, year int) error {
	return r.q.SetActiveYear(ctx, int64(year))
}
