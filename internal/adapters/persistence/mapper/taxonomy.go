package mapper

import (
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func ExpenseTypeToRow(t model.ExpenseType) sqlc.UpsertExpenseTypeParams {
	return sqlc.UpsertExpenseTypeParams{
		Year:     int64(t.Year()),
		Code:     t.Code(),
		Label:    t.Label(),
		Category: string(t.Category()),
	}
}

func ExpenseTypeFromRow(r sqlc.ExpenseType) (model.ExpenseType, error) {
	cat, err := model.ParseExpenseCategory(r.Category)
	if err != nil {
		return model.ExpenseType{}, err
	}
	return model.NewExpenseType(int(r.Year), r.Code, r.Label, cat)
}

func ExpenseSubtypeToRow(s model.ExpenseSubtype) sqlc.UpsertExpenseSubtypeParams {
	return sqlc.UpsertExpenseSubtypeParams{
		Year:     int64(s.Year()),
		Code:     s.Code(),
		Label:    s.Label(),
		TypeCode: s.TypeCode(),
	}
}

func ExpenseSubtypeFromRow(r sqlc.ExpenseSubtype) (model.ExpenseSubtype, error) {
	return model.NewExpenseSubtype(int(r.Year), r.Code, r.Label, r.TypeCode)
}
