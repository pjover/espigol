package model_test

import (
	"testing"

	"github.com/pjover/espigol/internal/domain/model"
)

func TestNewConcession_Valid(t *testing.T) {
	c, err := model.NewConcession(2025, "A6-02", "a6", "Adob orgànic",
		model.MoneyOf(13880), model.MoneyOf(13880))
	if err != nil {
		t.Fatalf("NewConcession: %v", err)
	}
	if c.GroupCode() != "A6-02" || c.SubtypeCode() != "a6" || c.Concept() != "Adob orgànic" {
		t.Errorf("unexpected fields: %+v", c)
	}
	if c.GrantedAmount().Cmp(model.MoneyOf(13880)) != 0 {
		t.Errorf("granted = %s", c.GrantedAmount())
	}
}

func TestNewConcession_Rejects(t *testing.T) {
	cases := map[string]struct{ group, subtype string }{
		"empty group":   {"", "a6"},
		"empty subtype": {"A6-02", ""},
	}
	for name, tc := range cases {
		if _, err := model.NewConcession(2025, tc.group, tc.subtype, "c",
			model.ZeroMoney(), model.ZeroMoney()); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestNewConcessionForecast_Valid(t *testing.T) {
	cf, err := model.NewConcessionForecast(2025, "A6-02", "CP25008")
	if err != nil {
		t.Fatalf("NewConcessionForecast: %v", err)
	}
	if cf.ForecastID() != "CP25008" || cf.GroupCode() != "A6-02" || cf.Year() != 2025 {
		t.Errorf("unexpected: %+v", cf)
	}
}

func TestNewConcessionForecast_Rejects(t *testing.T) {
	if _, err := model.NewConcessionForecast(2025, "", "CP25008"); err == nil {
		t.Error("empty group: expected error")
	}
	if _, err := model.NewConcessionForecast(2025, "A6-02", ""); err == nil {
		t.Error("empty forecastID: expected error")
	}
}
