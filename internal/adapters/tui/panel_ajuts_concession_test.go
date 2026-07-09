package tui

import "testing"

func TestParseForecastIDs(t *testing.T) {
	got := parseForecastIDs(" CP25008 , CP25009 ,, ")
	if len(got) != 2 || got[0] != "CP25008" || got[1] != "CP25009" {
		t.Fatalf("parseForecastIDs = %v", got)
	}
	if len(parseForecastIDs("")) != 0 {
		t.Error("empty string should yield no ids")
	}
}

func TestParseMoney(t *testing.T) {
	m, err := parseMoney(" 13880.00 ")
	if err != nil || m.String() != "13880.00" {
		t.Fatalf("parseMoney = (%s, %v)", m, err)
	}
	if _, err := parseMoney("abc"); err == nil {
		t.Error("expected error for non-numeric")
	}
}
