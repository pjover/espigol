package persistence

import (
	"context"

	"github.com/pjover/espigol/internal/adapters/persistence/mapper"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

type TaxonomyRepository struct {
	q *sqlc.Queries
}

func NewTaxonomyRepository(q *sqlc.Queries) *TaxonomyRepository {
	return &TaxonomyRepository{q: q}
}

func (r *TaxonomyRepository) SaveType(ctx context.Context, t model.ExpenseType) error {
	return r.q.UpsertExpenseType(ctx, mapper.ExpenseTypeToRow(t))
}

func (r *TaxonomyRepository) SaveSubtype(ctx context.Context, s model.ExpenseSubtype) error {
	return r.q.UpsertExpenseSubtype(ctx, mapper.ExpenseSubtypeToRow(s))
}

func (r *TaxonomyRepository) ListTypes(ctx context.Context, year int) ([]model.ExpenseType, error) {
	rows, err := r.q.ListExpenseTypes(ctx, int64(year))
	if err != nil {
		return nil, err
	}
	out := make([]model.ExpenseType, 0, len(rows))
	for _, row := range rows {
		t, err := mapper.ExpenseTypeFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (r *TaxonomyRepository) DeleteType(ctx context.Context, year int, code string) error {
	return r.q.DeleteExpenseType(ctx, sqlc.DeleteExpenseTypeParams{
		Year: int64(year),
		Code: code,
	})
}

func (r *TaxonomyRepository) DeleteSubtype(ctx context.Context, year int, code string) error {
	return r.q.DeleteExpenseSubtype(ctx, sqlc.DeleteExpenseSubtypeParams{
		Year: int64(year),
		Code: code,
	})
}

func (r *TaxonomyRepository) ListSubtypes(ctx context.Context, year int) ([]model.ExpenseSubtype, error) {
	rows, err := r.q.ListExpenseSubtypes(ctx, int64(year))
	if err != nil {
		return nil, err
	}
	out := make([]model.ExpenseSubtype, 0, len(rows))
	for _, row := range rows {
		s, err := mapper.ExpenseSubtypeFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}
