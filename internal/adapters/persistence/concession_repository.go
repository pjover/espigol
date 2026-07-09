package persistence

import (
	"context"

	"github.com/pjover/espigol/internal/adapters/persistence/mapper"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

type ConcessionRepository struct {
	q *sqlc.Queries
}

func NewConcessionRepository(q *sqlc.Queries) *ConcessionRepository {
	return &ConcessionRepository{q: q}
}

func (r *ConcessionRepository) ListByYear(ctx context.Context, year int) ([]model.Concession, error) {
	rows, err := r.q.ListConcessionsByYear(ctx, int64(year))
	if err != nil {
		return nil, err
	}
	out := make([]model.Concession, 0, len(rows))
	for _, row := range rows {
		c, err := mapper.ConcessionFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (r *ConcessionRepository) ListForecastLinksByYear(ctx context.Context, year int) ([]model.ConcessionForecast, error) {
	rows, err := r.q.ListConcessionForecastsByYear(ctx, int64(year))
	if err != nil {
		return nil, err
	}
	out := make([]model.ConcessionForecast, 0, len(rows))
	for _, row := range rows {
		cf, err := mapper.ConcessionForecastFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, cf)
	}
	return out, nil
}

func (r *ConcessionRepository) Save(ctx context.Context, c model.Concession) error {
	return r.q.UpsertConcession(ctx, mapper.ConcessionToUpsert(c))
}

func (r *ConcessionRepository) Delete(ctx context.Context, year int, groupCode string) error {
	if err := r.q.DeleteConcessionForecastsByGroup(ctx, sqlc.DeleteConcessionForecastsByGroupParams{
		Year: int64(year), GroupCode: groupCode,
	}); err != nil {
		return err
	}
	return r.q.DeleteConcession(ctx, sqlc.DeleteConcessionParams{Year: int64(year), GroupCode: groupCode})
}

func (r *ConcessionRepository) ReplaceMembership(ctx context.Context, year int, groupCode string, forecastIDs []string) error {
	if err := r.q.DeleteConcessionForecastsByGroup(ctx, sqlc.DeleteConcessionForecastsByGroupParams{
		Year: int64(year), GroupCode: groupCode,
	}); err != nil {
		return err
	}
	for _, fid := range forecastIDs {
		if err := r.q.InsertConcessionForecast(ctx, sqlc.InsertConcessionForecastParams{
			Year: int64(year), ForecastID: fid, GroupCode: groupCode,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *ConcessionRepository) ReplaceForYear(ctx context.Context, year int, concessions []model.Concession, links []model.ConcessionForecast) error {
	if err := r.q.DeleteConcessionForecastsByYear(ctx, int64(year)); err != nil {
		return err
	}
	if err := r.q.DeleteConcessionsByYear(ctx, int64(year)); err != nil {
		return err
	}
	for _, c := range concessions {
		if err := r.q.UpsertConcession(ctx, mapper.ConcessionToUpsert(c)); err != nil {
			return err
		}
	}
	for _, l := range links {
		if err := r.q.InsertConcessionForecast(ctx, sqlc.InsertConcessionForecastParams{
			Year: int64(l.Year()), ForecastID: l.ForecastID(), GroupCode: l.GroupCode(),
		}); err != nil {
			return err
		}
	}
	return nil
}
