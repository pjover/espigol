// Package persistence holds SQLite repositories implementing the domain ports.
package persistence

import (
	"context"
	"database/sql"
	"errors"

	"github.com/pjover/espigol/internal/adapters/persistence/mapper"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/ports"
)

type PartnerRepository struct {
	q *sqlc.Queries
}

var _ ports.PartnerRepository = (*PartnerRepository)(nil)

func NewPartnerRepository(q *sqlc.Queries) *PartnerRepository {
	return &PartnerRepository{q: q}
}

func (r *PartnerRepository) Save(ctx context.Context, p model.Partner) error {
	return r.q.UpsertPartner(ctx, mapper.PartnerToRow(p))
}

func (r *PartnerRepository) FindByID(ctx context.Context, id int) (model.Partner, bool, error) {
	row, err := r.q.GetPartner(ctx, int64(id))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Partner{}, false, nil
	}
	if err != nil {
		return model.Partner{}, false, err
	}
	p, err := mapper.PartnerFromRow(row)
	return p, err == nil, err
}

func (r *PartnerRepository) FindByEmail(ctx context.Context, email string) (model.Partner, bool, error) {
	row, err := r.q.GetPartnerByEmail(ctx, email)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Partner{}, false, nil
	}
	if err != nil {
		return model.Partner{}, false, err
	}
	p, err := mapper.PartnerFromRow(row)
	return p, err == nil, err
}

func (r *PartnerRepository) List(ctx context.Context) ([]model.Partner, error) {
	rows, err := r.q.ListPartners(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.Partner, 0, len(rows))
	for _, row := range rows {
		p, err := mapper.PartnerFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}
