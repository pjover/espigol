package mapper

import (
	"database/sql"

	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func nullableSection(s model.ExpenseScope) sql.NullString {
	if s.Kind() == model.ScopeSection {
		return sql.NullString{String: s.SectionCode(), Valid: true}
	}
	return sql.NullString{}
}

func ForecastToInsert(f model.ExpenseForecast) sqlc.InsertForecastParams {
	return sqlc.InsertForecastParams{
		ID:             f.ID(),
		PartnerID:      int64(f.PartnerID()),
		Concept:        f.Concept(),
		Description:    f.Description(),
		GrossAmount:    f.GrossAmount().String(),
		ApprovedAmount: f.ApprovedAmount().String(),
		ApprovedOn:     FormatNullableTimestamp(f.ApprovedOn()),
		PlannedDate:    FormatDate(f.PlannedDate()),
		Year:           int64(f.Year()),
		SubtypeCode:    f.SubtypeCode(),
		ScopeKind:      string(f.Scope().Kind()),
		SectionCode:    nullableSection(f.Scope()),
		AddedOn:        FormatTimestamp(f.AddedOn()),
		Enabled:        boolToInt(f.Enabled()),
	}
}

func ForecastToUpdate(f model.ExpenseForecast) sqlc.UpdateForecastParams {
	return sqlc.UpdateForecastParams{
		ID:             f.ID(),
		PartnerID:      int64(f.PartnerID()),
		Concept:        f.Concept(),
		Description:    f.Description(),
		GrossAmount:    f.GrossAmount().String(),
		ApprovedAmount: f.ApprovedAmount().String(),
		ApprovedOn:     FormatNullableTimestamp(f.ApprovedOn()),
		PlannedDate:    FormatDate(f.PlannedDate()),
		Year:           int64(f.Year()),
		SubtypeCode:    f.SubtypeCode(),
		ScopeKind:      string(f.Scope().Kind()),
		SectionCode:    nullableSection(f.Scope()),
		AddedOn:        FormatTimestamp(f.AddedOn()),
		Enabled:        boolToInt(f.Enabled()),
	}
}

func ForecastFromRow(r sqlc.ExpenseForecast) (model.ExpenseForecast, error) {
	gross, err := model.MoneyFromString(r.GrossAmount)
	if err != nil {
		return model.ExpenseForecast{}, err
	}
	approved, err := model.MoneyFromString(r.ApprovedAmount)
	if err != nil {
		return model.ExpenseForecast{}, err
	}
	approvedOn, err := ParseNullableTimestamp(r.ApprovedOn)
	if err != nil {
		return model.ExpenseForecast{}, err
	}
	planned, err := ParseDate(r.PlannedDate)
	if err != nil {
		return model.ExpenseForecast{}, err
	}
	added, err := ParseTimestamp(r.AddedOn)
	if err != nil {
		return model.ExpenseForecast{}, err
	}
	kind, err := model.ParseScopeKind(r.ScopeKind)
	if err != nil {
		return model.ExpenseForecast{}, err
	}
	sectionCode := ""
	if r.SectionCode.Valid {
		sectionCode = r.SectionCode.String
	}
	scope, err := model.NewScope(kind, sectionCode)
	if err != nil {
		return model.ExpenseForecast{}, err
	}
	return model.NewExpenseForecast(r.ID, int(r.PartnerID), r.Concept, r.Description,
		gross, approved, approvedOn, planned, int(r.Year), r.SubtypeCode, scope, added, r.Enabled == 1)
}
