package persistence

import (
	"context"
	"database/sql"

	"github.com/pjover/espigol/internal/adapters/persistence/mapper"
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

type BoardAuthorizationRepository struct {
	q *sqlc.Queries
}

func NewBoardAuthorizationRepository(q *sqlc.Queries) *BoardAuthorizationRepository {
	return &BoardAuthorizationRepository{q: q}
}

func (r *BoardAuthorizationRepository) Save(ctx context.Context, a model.BoardAuthorization) error {
	return r.q.UpsertBoardAuthorization(ctx, mapper.BoardAuthToRow(a))
}

// Remove deletes the matching authorization and returns the number of rows
// removed (0 when no such authorization existed).
func (r *BoardAuthorizationRepository) Remove(ctx context.Context, partnerID int, scopeKind model.ScopeKind, sectionCode string) (int64, error) {
	var section sql.NullString
	if sectionCode != "" {
		section = sql.NullString{String: sectionCode, Valid: true}
	}
	return r.q.DeleteBoardAuthorization(ctx, sqlc.DeleteBoardAuthorizationParams{
		PartnerID:   int64(partnerID),
		ScopeKind:   string(scopeKind),
		SectionCode: section,
	})
}

func (r *BoardAuthorizationRepository) ListByPartner(ctx context.Context, partnerID int) ([]model.BoardAuthorization, error) {
	rows, err := r.q.ListBoardAuthorizationsByPartner(ctx, int64(partnerID))
	if err != nil {
		return nil, err
	}
	out := make([]model.BoardAuthorization, 0, len(rows))
	for _, row := range rows {
		a, err := mapper.BoardAuthFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}
