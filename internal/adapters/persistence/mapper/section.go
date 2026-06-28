package mapper

import (
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func SectionToRow(s model.Section) sqlc.UpsertSectionParams {
	active := int64(0)
	if s.Active() {
		active = 1
	}
	return sqlc.UpsertSectionParams{
		Code:         s.Code(),
		Label:        s.Label(),
		Active:       active,
		DisplayOrder: int64(s.DisplayOrder()),
	}
}

func SectionFromRow(r sqlc.Section) (model.Section, error) {
	return model.NewSection(r.Code, r.Label, r.Active == 1, int(r.DisplayOrder))
}

func PartnerSectionFromRow(r sqlc.PartnerSection) (model.PartnerSection, error) {
	return model.NewPartnerSection(int(r.PartnerID), r.SectionCode)
}
