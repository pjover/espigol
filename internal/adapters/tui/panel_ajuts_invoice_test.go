package tui

import (
	"testing"

	"github.com/pjover/espigol/internal/domain/model"
)

func TestParsePayments(t *testing.T) {
	pays, err := parsePayments("2025-04-01:1234.56; 2025-05-01:200.00 ")
	if err != nil || len(pays) != 2 {
		t.Fatalf("parsePayments = (%v, %v)", pays, err)
	}
	if pays[0].Amount.Cmp(model.MoneyOf(0).Plus(mustMoneyT(t, "1234.56"))) != 0 {
		t.Errorf("amount[0] = %s", pays[0].Amount)
	}
	if pays[0].PaidOn.Year() != 2025 || pays[0].PaidOn.Month() != 4 {
		t.Errorf("date[0] = %v", pays[0].PaidOn)
	}
	if _, err := parsePayments("bad"); err == nil {
		t.Error("expected error for malformed payment")
	}
}

func TestParseLinks(t *testing.T) {
	links, err := parseLinks("CP25030:500.00;CP25032:734.56")
	if err != nil || len(links) != 2 || links[1].ForecastID != "CP25032" {
		t.Fatalf("parseLinks = (%v, %v)", links, err)
	}
}

func mustMoneyT(t *testing.T, s string) model.Money {
	t.Helper()
	m, err := model.MoneyFromString(s)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
