package model

import (
	"encoding/json"
	"github.com/shopspring/decimal"
)

// Money is an immutable monetary amount, always scale 2, HALF_UP rounding.
// It deliberately imports no database/sql types; serialization lives in the
// persistence mapper via String() and MoneyFromString.
type Money struct {
	amount decimal.Decimal
}

func normalize(d decimal.Decimal) decimal.Decimal {
	return d.Round(2)
}

// MoneyFromString parses a decimal string (e.g. "31900.00") into Money.
func MoneyFromString(s string) (Money, error) {
	d, err := decimal.NewFromString(s)
	if err != nil {
		return Money{}, err
	}
	return Money{amount: normalize(d)}, nil
}

// MoneyOf builds Money from a whole-unit integer amount.
func MoneyOf(value int64) Money {
	return Money{amount: normalize(decimal.NewFromInt(value))}
}

// ZeroMoney returns a zero amount.
func ZeroMoney() Money {
	return Money{amount: normalize(decimal.Zero)}
}

func (m Money) Plus(other Money) Money  { return Money{amount: normalize(m.amount.Add(other.amount))} }
func (m Money) Minus(other Money) Money { return Money{amount: normalize(m.amount.Sub(other.amount))} }
func (m Money) Times(factor int) Money {
	return Money{amount: normalize(m.amount.Mul(decimal.NewFromInt(int64(factor))))}
}
func (m Money) TimesRatio(ratio decimal.Decimal) Money {
	return Money{amount: normalize(m.amount.Mul(ratio))}
}

// DividedBy divides by a positive integer (HALF_UP, scale 2). Panics on divisor < 1,
// mirroring the reference implementation's IllegalArgumentException.
func (m Money) DividedBy(divisor int) Money {
	if divisor < 1 {
		panic("Money.DividedBy: divisor must be >= 1")
	}
	return Money{amount: m.amount.DivRound(decimal.NewFromInt(int64(divisor)), 2)}
}

func (m Money) Negate() Money              { return Money{amount: normalize(m.amount.Neg())} }
func (m Money) Cmp(other Money) int        { return m.amount.Cmp(other.amount) }
func (m Money) IsZero() bool               { return m.amount.IsZero() }
func (m Money) Decimal() decimal.Decimal   { return m.amount }

// String returns the canonical fixed-scale form, e.g. "31900.00".
func (m Money) String() string { return m.amount.StringFixed(2) }

// MarshalJSON renders Money as its canonical decimal string, e.g. "31900.00".
func (m Money) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.String())
}

// UnmarshalJSON parses a decimal string (as produced by MarshalJSON) into Money.
func (m *Money) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsed, err := MoneyFromString(s)
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}
