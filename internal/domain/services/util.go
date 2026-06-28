package services

import (
	"github.com/shopspring/decimal"

	"github.com/pjover/espigol/internal/domain/model"
)

func mustMoney(s string) model.Money {
	m, err := model.MoneyFromString(s)
	if err != nil {
		panic("services: invalid money literal " + s)
	}
	return m
}

func absDecimal(d decimal.Decimal) decimal.Decimal { return d.Abs() }
