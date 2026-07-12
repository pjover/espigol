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

// Create allocates the next CPYYnnn id for the forecast's year and inserts it.
// When called within a TxManager transaction, r.q is already tx-scoped so the
// ID allocation and insert are atomic with the surrounding transaction. When
// called directly (e.g. in standalone tests), r.q runs against the bare connection.
func (r *ForecastRepository) Create(ctx context.Context, f model.ExpenseForecast) (model.ExpenseForecast, error) {
	ids, err := r.q.ListForecastIDsByYear(ctx, int64(f.Year()))
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
	if err := r.q.InsertForecast(ctx, mapper.ForecastToInsert(withID)); err != nil {
		return model.ExpenseForecast{}, err
	}
	return withID, nil
}

func (r *ForecastRepository) Save(ctx context.Context, f model.ExpenseForecast) error {
	return r.q.UpdateForecast(ctx, mapper.ForecastToUpdate(f))
}

func (r *ForecastRepository) fetchPartner(ctx context.Context, partnerID int64) (model.Partner, error) {
	row, err := r.q.GetPartner(ctx, partnerID)
	if err != nil {
		return model.Partner{}, err
	}
	return mapper.PartnerFromRow(row)
}

func (r *ForecastRepository) FindByID(ctx context.Context, id string) (model.ExpenseForecast, bool, error) {
	row, err := r.q.GetForecast(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return model.ExpenseForecast{}, false, nil
	}
	if err != nil {
		return model.ExpenseForecast{}, false, err
	}
	partner, err := r.fetchPartner(ctx, row.PartnerID)
	if err != nil {
		return model.ExpenseForecast{}, false, err
	}
	f, err := mapper.ForecastFromRow(row, partner)
	return f, err == nil, err
}

func (r *ForecastRepository) ListByYear(ctx context.Context, year int) ([]model.ExpenseForecast, error) {
	rows, err := r.q.ListForecastsByYear(ctx, int64(year))
	if err != nil {
		return nil, err
	}
	out := make([]model.ExpenseForecast, 0, len(rows))
	for _, row := range rows {
		partner, err := r.fetchPartner(ctx, row.PartnerID)
		if err != nil {
			return nil, err
		}
		f, err := mapper.ForecastFromRow(row, partner)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

func (r *ForecastRepository) Delete(ctx context.Context, id string) error {
	return r.q.DeleteForecast(ctx, id)
}

// rebuildWithID returns a copy of f with the given id, re-running domain validation.
func rebuildWithID(f model.ExpenseForecast, id string) (model.ExpenseForecast, error) {
	return model.NewExpenseForecast(id, f.Partner(), f.Concept(), f.Description(),
		f.GrossAmount(), f.ApprovedAmount(), f.ApprovedOn(), f.PlannedDate(), f.Year(),
		f.SubtypeCode(), f.Scope(), f.AddedOn(), f.Enabled())
}
