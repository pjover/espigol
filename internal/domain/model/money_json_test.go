package model

import (
	"encoding/json"
	"testing"
)

func TestMoney_JSONRoundTrip(t *testing.T) {
	for _, s := range []string{"31900.00", "1322.22", "0.00", "-5.00"} {
		m, err := MoneyFromString(s)
		if err != nil {
			t.Fatal(err)
		}
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if string(b) != `"`+s+`"` {
			t.Errorf("Marshal(%s) = %s, want %q", s, b, `"`+s+`"`)
		}
		var back Money
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if back.Cmp(m) != 0 || back.String() != s {
			t.Errorf("round trip %s -> %s", s, back.String())
		}
	}
}

func TestMoney_JSONInStruct(t *testing.T) {
	type wrap struct {
		Amount Money `json:"amount"`
	}
	m, _ := MoneyFromString("12.50")
	b, err := json.Marshal(wrap{Amount: m})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"amount":"12.50"}` {
		t.Errorf("got %s", b)
	}
	var w wrap
	if err := json.Unmarshal(b, &w); err != nil {
		t.Fatal(err)
	}
	if w.Amount.String() != "12.50" {
		t.Errorf("struct round trip = %s", w.Amount.String())
	}
}
