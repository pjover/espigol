package model

import (
	"testing"
	"time"
)

func TestNewExpenseForecast_Valid(t *testing.T) {
	planned := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	added := time.Date(2026, 2, 21, 19, 0, 0, 0, time.UTC)
	f, err := NewExpenseForecast("CP26023", 7, "Projecte", "desc", MoneyOf(2880), MoneyOf(2880),
		nil, planned, 2026, "a2", NewCommonScope(), added, true)
	if err != nil {
		t.Fatal(err)
	}
	if f.ID() != "CP26023" || f.Year() != 2026 || f.Scope().Kind() != ScopeCommon {
		t.Errorf("accessors wrong: %+v", f)
	}
	f2 := f.WithApprovedAmount(MoneyOf(2000))
	if f2.ApprovedAmount().String() != "2000.00" || f.ApprovedAmount().String() != "2880.00" {
		t.Error("WithApprovedAmount should not mutate original")
	}
}

func TestNewExpenseForecast_YearMustMatchPlannedDate(t *testing.T) {
	planned := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	_, err := NewExpenseForecast("CP25001", 1, "c", "d", ZeroMoney(), ZeroMoney(),
		nil, planned, 2025, "a1", NewPartnerScope(), time.Now(), true)
	if err == nil {
		t.Fatal("expected error: year 2025 != plannedDate.Year() 2026")
	}
}
