package report

import (
	"testing"

	"github.com/pjover/espigol/internal/domain/model"
)

func TestFormatEuro(t *testing.T) {
	cases := map[string]string{
		"1234.56": "1.234,56 €",
		"0":       "0,00 €",
		"-9":      "-9,00 €",
		"31900":   "31.900,00 €",
		"1322.22": "1.322,22 €",
		"1000000": "1.000.000,00 €",
		"-1234.5": "-1.234,50 €",
	}
	for in, want := range cases {
		m, err := model.MoneyFromString(in)
		if err != nil {
			t.Fatalf("money %q: %v", in, err)
		}
		if got := formatEuro(m); got != want {
			t.Errorf("formatEuro(%s) = %q, want %q", in, got, want)
		}
	}
}

func TestCategoryLabel(t *testing.T) {
	if categoryLabel(model.CategoryCurrent) != "Despesa corrent" {
		t.Errorf("current label wrong")
	}
	if categoryLabel(model.CategoryInvestment) != "Despesa d'inversió" {
		t.Errorf("investment label wrong")
	}
}
