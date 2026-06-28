package persistence

import (
	"context"
	"database/sql"
	"errors"

	"github.com/pjover/espigol/internal/adapters/persistence/mapper"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/forecastid"
	"github.com/pjover/espigol/internal/domain/model"
)

type ForecastRepository struct {
	conn *sql.DB
	q    *sqlc.Queries
}

func NewForecastRepository(conn *sql.DB, q *sqlc.Queries) *ForecastRepository {
	return &ForecastRepository{conn: conn, q: q}
}

// Create allocates the next CPYYnnn id for the forecast's year and inserts it,
// within a single transaction so concurrent creates cannot collide.
func (r *ForecastRepository) Create(ctx context.Context, f model.ExpenseForecast) (model.ExpenseForecast, error) {
	tx, err := r.conn.BeginTx(ctx, nil)
	if err != nil {
		return model.ExpenseForecast{}, err
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := r.q.WithTx(tx)

	ids, err := qtx.ListForecastIDsByYear(ctx, int64(f.Year()))
	if err != nil {
		return model.ExpenseForecast{}, err
	}

	// Sequence starts at 1 (CP26001 is the first forecast for a year).
	maxSeq := 0
	for _, id := range ids {
		_, seq, err := forecastid.ParseSeq(id)
		if err != nil {
			return model.ExpenseForecast{}, err
		}
		if seq > maxSeq {
			maxSeq = seq
		}
	}

	newID, err := forecastid.Format(f.Year(), maxSeq+1)
	if err != nil {
		return model.ExpenseForecast{}, err
	}

	withID, err := rebuildWithID(f, newID)
	if err != nil {
		return model.ExpenseForecast{}, err
	}
	if err := qtx.InsertForecast(ctx, mapper.ForecastToInsert(withID)); err != nil {
		return model.ExpenseForecast{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.ExpenseForecast{}, err
	}
	return withID, nil
}

// InsertWithID inserts a forecast using its existing id verbatim, bypassing the
// CPYYnnn allocation in Create. Used by the adopt tool to carry legacy ids.
func (r *ForecastRepository) InsertWithID(ctx context.Context, f model.ExpenseForecast) error {
	return r.q.InsertForecast(ctx, mapper.ForecastToInsert(f))
}

func (r *ForecastRepository) Save(ctx context.Context, f model.ExpenseForecast) error {
	return r.q.UpdateForecast(ctx, mapper.ForecastToUpdate(f))
}

func (r *ForecastRepository) FindByID(ctx context.Context, id string) (model.ExpenseForecast, bool, error) {
	row, err := r.q.GetForecast(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return model.ExpenseForecast{}, false, nil
	}
	if err != nil {
		return model.ExpenseForecast{}, false, err
	}
	f, err := mapper.ForecastFromRow(row)
	return f, err == nil, err
}

func (r *ForecastRepository) ListByYear(ctx context.Context, year int) ([]model.ExpenseForecast, error) {
	rows, err := r.q.ListForecastsByYear(ctx, int64(year))
	if err != nil {
		return nil, err
	}
	out := make([]model.ExpenseForecast, 0, len(rows))
	for _, row := range rows {
		f, err := mapper.ForecastFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// rebuildWithID returns a copy of f with the given id, re-running domain validation.
func rebuildWithID(f model.ExpenseForecast, id string) (model.ExpenseForecast, error) {
	return model.NewExpenseForecast(id, f.PartnerID(), f.Concept(), f.Description(),
		f.GrossAmount(), f.ApprovedAmount(), f.ApprovedOn(), f.PlannedDate(), f.Year(),
		f.SubtypeCode(), f.Scope(), f.AddedOn(), f.Enabled())
}
