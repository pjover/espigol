package mapper

import (
	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/model"
)

func WindowToRow(w model.SubmissionWindow) sqlc.UpsertSubmissionWindowParams {
	return sqlc.UpsertSubmissionWindowParams{
		Year:                   int64(w.Year()),
		State:                  string(w.State()),
		OpenedAt:               FormatNullableTimestamp(w.OpenedAt()),
		ClosedAt:               FormatNullableTimestamp(w.ClosedAt()),
		Deadline:               FormatTimestamp(w.Deadline()),
		CurrentExpenseLimit:    w.CurrentExpenseLimit().String(),
		InvestmentExpenseLimit: w.InvestmentExpenseLimit().String(),
	}
}

func WindowFromRow(r sqlc.SubmissionWindow) (model.SubmissionWindow, error) {
	state, err := model.ParseWindowState(r.State)
	if err != nil {
		return model.SubmissionWindow{}, err
	}
	openedAt, err := ParseNullableTimestamp(r.OpenedAt)
	if err != nil {
		return model.SubmissionWindow{}, err
	}
	closedAt, err := ParseNullableTimestamp(r.ClosedAt)
	if err != nil {
		return model.SubmissionWindow{}, err
	}
	deadline, err := ParseTimestamp(r.Deadline)
	if err != nil {
		return model.SubmissionWindow{}, err
	}
	current, err := model.MoneyFromString(r.CurrentExpenseLimit)
	if err != nil {
		return model.SubmissionWindow{}, err
	}
	investment, err := model.MoneyFromString(r.InvestmentExpenseLimit)
	if err != nil {
		return model.SubmissionWindow{}, err
	}
	return model.NewSubmissionWindow(int(r.Year), state, openedAt, closedAt, deadline, current, investment)
}
