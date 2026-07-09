package model_test

import (
	"testing"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
)

func TestNewInvoice_AggregatesAndPaidTotal(t *testing.T) {
	d := time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC)
	pays := []model.InvoicePayment{
		model.NewInvoicePayment(0, 0, d, model.MoneyOf(1000)),
		model.NewInvoicePayment(0, 0, d.AddDate(0, 1, 0), model.MoneyOf(234)),
	}
	link, err := model.NewForecastInvoice("CP25030", 0, model.MoneyOf(500))
	if err != nil {
		t.Fatalf("NewForecastInvoice: %v", err)
	}
	inv, err := model.NewInvoice(0, 2025, "Jardines Campaner", "B12345678", "F878",
		d, model.MoneyOf(1234), nil, nil, pays, []model.ForecastInvoice{link})
	if err != nil {
		t.Fatalf("NewInvoice: %v", err)
	}
	if inv.Number() != "F878" || inv.Year() != 2025 {
		t.Errorf("unexpected header: %+v", inv)
	}
	if len(inv.Payments()) != 2 || len(inv.Links()) != 1 {
		t.Errorf("children: payments=%d links=%d", len(inv.Payments()), len(inv.Links()))
	}
	if inv.PaidTotal().Cmp(model.MoneyOf(1234)) != 0 {
		t.Errorf("PaidTotal = %s, want 1234.00", inv.PaidTotal())
	}
}

func TestNewInvoice_Rejects(t *testing.T) {
	d := time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC)
	if _, err := model.NewInvoice(0, 2025, "", "n", "num", d, model.ZeroMoney(), nil, nil, nil, nil); err == nil {
		t.Error("empty issuer: expected error")
	}
	if _, err := model.NewInvoice(0, 2025, "iss", "n", "", d, model.ZeroMoney(), nil, nil, nil, nil); err == nil {
		t.Error("empty number: expected error")
	}
}

func TestForecastInvoice_Rejects(t *testing.T) {
	if _, err := model.NewForecastInvoice("", 1, model.ZeroMoney()); err == nil {
		t.Error("empty forecastID: expected error")
	}
}

func TestInvoice_WithID(t *testing.T) {
	d := time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC)
	inv, _ := model.NewInvoice(0, 2025, "iss", "n", "num", d, model.MoneyOf(10), nil, nil, nil, nil)
	if inv.WithID(7).ID() != 7 {
		t.Error("WithID did not set id")
	}
}
