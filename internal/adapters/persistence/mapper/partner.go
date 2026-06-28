package mapper

import (
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func PartnerToRow(p model.Partner) sqlc.UpsertPartnerParams {
	board := int64(0)
	if p.BoardMember() {
		board = 1
	}
	return sqlc.UpsertPartnerParams{
		ID:          int64(p.ID()),
		Name:        p.Name(),
		Surname:     p.Surname(),
		VatCode:     p.VatCode(),
		Email:       p.Email(),
		Mobile:      p.Mobile(),
		PartnerType: string(p.PartnerType()),
		RiaNumber:   int64(p.RiaNumber()),
		AddedOn:     FormatDate(p.AddedOn()),
		BoardMember: board,
	}
}

func PartnerFromRow(r sqlc.Partner) (model.Partner, error) {
	pt, err := model.ParsePartnerType(r.PartnerType)
	if err != nil {
		return model.Partner{}, err
	}
	addedOn, err := ParseDate(r.AddedOn)
	if err != nil {
		return model.Partner{}, err
	}
	return model.NewPartner(int(r.ID), r.Name, r.Surname, r.VatCode, r.Email, r.Mobile,
		pt, int(r.RiaNumber), addedOn, r.BoardMember == 1)
}
