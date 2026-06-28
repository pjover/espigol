package mapper

import (
	"database/sql"

	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func BoardAuthToRow(a model.BoardAuthorization) sqlc.UpsertBoardAuthorizationParams {
	section := sql.NullString{}
	if a.SectionCode() != "" {
		section = sql.NullString{String: a.SectionCode(), Valid: true}
	}
	return sqlc.UpsertBoardAuthorizationParams{
		PartnerID:   int64(a.PartnerID()),
		ScopeKind:   string(a.ScopeKind()),
		SectionCode: section,
	}
}

func BoardAuthFromRow(r sqlc.BoardAuthorization) (model.BoardAuthorization, error) {
	kind, err := model.ParseScopeKind(r.ScopeKind)
	if err != nil {
		return model.BoardAuthorization{}, err
	}
	section := ""
	if r.SectionCode.Valid {
		section = r.SectionCode.String
	}
	return model.NewBoardAuthorization(int(r.PartnerID), kind, section)
}
