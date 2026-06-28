package model

import "testing"

func TestMoneyFromString_NormalizesScale(t *testing.T) {
	m, err := MoneyFromString("31900")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.String() != "31900.00" {
		t.Errorf("String() = %q, want %q", m.String(), "31900.00")
	}
}

func TestMoneyFromString_Rejects(t *testing.T) {
	if _, err := MoneyFromString("not-a-number"); err == nil {
		t.Fatal("expected error for invalid input")
	}
}

func TestMoneyArithmetic(t *testing.T) {
	a, _ := MoneyFromString("10.00")
	b, _ := MoneyFromString("3.00")
	if got := a.Plus(b).String(); got != "13.00" {
		t.Errorf("Plus = %q, want 13.00", got)
	}
	if got := a.Minus(b).String(); got != "7.00" {
		t.Errorf("Minus = %q, want 7.00", got)
	}
	if got := b.Times(3).String(); got != "9.00" {
		t.Errorf("Times = %q, want 9.00", got)
	}
	if got := a.DividedBy(3).String(); got != "3.33" {
		t.Errorf("DividedBy = %q, want 3.33 (HALF_UP scale 2)", got)
	}
	if got := a.Negate().String(); got != "-10.00" {
		t.Errorf("Negate = %q, want -10.00", got)
	}
}

func TestMoneyCmpAndZero(t *testing.T) {
	a, _ := MoneyFromString("10.00")
	b, _ := MoneyFromString("3.00")
	if a.Cmp(b) <= 0 {
		t.Errorf("expected a > b")
	}
	if !ZeroMoney().IsZero() {
		t.Errorf("ZeroMoney should be zero")
	}
	if MoneyOf(5).String() != "5.00" {
		t.Errorf("MoneyOf(5) = %q, want 5.00", MoneyOf(5).String())
	}
}

func TestMoney_RealValueRoundTrips(t *testing.T) {
	// The former-REAL legacy value must survive exactly.
	m, err := MoneyFromString("1322.22")
	if err != nil {
		t.Fatal(err)
	}
	if m.String() != "1322.22" {
		t.Errorf("String() = %q, want 1322.22", m.String())
	}
}
