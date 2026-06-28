package persistence

import (
	"context"

	"github.com/pjover/espigol/internal/adapters/persistence/mapper"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

type SectionRepository struct {
	q *sqlc.Queries
}

func NewSectionRepository(q *sqlc.Queries) *SectionRepository {
	return &SectionRepository{q: q}
}

func (r *SectionRepository) Save(ctx context.Context, s model.Section) error {
	return r.q.UpsertSection(ctx, mapper.SectionToRow(s))
}

func (r *SectionRepository) List(ctx context.Context) ([]model.Section, error) {
	rows, err := r.q.ListSections(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.Section, 0, len(rows))
	for _, row := range rows {
		s, err := mapper.SectionFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func (r *SectionRepository) AddMembership(ctx context.Context, m model.PartnerSection) error {
	return r.q.AddPartnerSection(ctx, sqlc.AddPartnerSectionParams{
		PartnerID:   int64(m.PartnerID()),
		SectionCode: m.SectionCode(),
	})
}

func (r *SectionRepository) ListMembershipsByPartner(ctx context.Context, partnerID int) ([]model.PartnerSection, error) {
	rows, err := r.q.ListPartnerSectionsByPartner(ctx, int64(partnerID))
	if err != nil {
		return nil, err
	}
	out := make([]model.PartnerSection, 0, len(rows))
	for _, row := range rows {
		m, err := mapper.PartnerSectionFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}
