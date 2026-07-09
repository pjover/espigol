package mapper

import (
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func ConcessionToUpsert(c model.Concession) sqlc.UpsertConcessionParams {
	return sqlc.UpsertConcessionParams{
		Year:           int64(c.Year()),
		GroupCode:      c.GroupCode(),
		SubtypeCode:    c.SubtypeCode(),
		Concept:        c.Concept(),
		RequestedTotal: c.RequestedTotal().String(),
		GrantedAmount:  c.GrantedAmount().String(),
	}
}

func ConcessionFromRow(r sqlc.Concession) (model.Concession, error) {
	req, err := model.MoneyFromString(r.RequestedTotal)
	if err != nil {
		return model.Concession{}, err
	}
	granted, err := model.MoneyFromString(r.GrantedAmount)
	if err != nil {
		return model.Concession{}, err
	}
	return model.NewConcession(int(r.Year), r.GroupCode, r.SubtypeCode, r.Concept, req, granted)
}

func ConcessionForecastFromRow(r sqlc.ConcessionForecast) (model.ConcessionForecast, error) {
	return model.NewConcessionForecast(int(r.Year), r.GroupCode, r.ForecastID)
}
