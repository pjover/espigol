package application

import (
	"context"

	"github.com/pjover/espigol/internal/domain/ports"
	"github.com/pjover/espigol/internal/domain/services"
)

// ProjecteService assembles the year's forecasts + taxonomy and computes the
// grouped ProjecteData used by the two Consorci documents. Read-only.
type ProjecteService struct {
	tx ports.TxManager
}

func NewProjecteService(tx ports.TxManager) *ProjecteService {
	return &ProjecteService{tx: tx}
}

func (s *ProjecteService) Compute(ctx context.Context, year int) (services.ProjecteData, error) {
	var out services.ProjecteData
	err := s.tx.WithinTx(ctx, func(r ports.RepoSet) error {
		forecasts, err := r.Forecasts.ListByYear(ctx, year)
		if err != nil {
			return err
		}
		types, err := r.Taxonomy.ListTypes(ctx, year)
		if err != nil {
			return err
		}
		subtypes, err := r.Taxonomy.ListSubtypes(ctx, year)
		if err != nil {
			return err
		}
		out = services.ComputeProjecte(services.ProjecteInput{
			Year: year, Forecasts: forecasts, Types: types, Subtypes: subtypes,
		})
		return nil
	})
	return out, err
}
