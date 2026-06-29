package report

import (
	"strings"

	"github.com/pjover/espigol/internal/domain/model"
)

// formatEuro renders Money as EU currency: "1.234,56 €" (period thousands,
// comma decimal, symbol after a space, always two decimals).
func formatEuro(m model.Money) string {
	s := m.String() // canonical "-1234.56" / "31900.00"
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	intPart, decPart := s, "00"
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, decPart = s[:i], s[i+1:]
	}

	// group the integer part with '.' every three digits
	var b strings.Builder
	n := len(intPart)
	for i := 0; i < n; i++ {
		if i > 0 && (n-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteByte(intPart[i])
	}

	out := b.String() + "," + decPart + " €"
	if neg {
		out = "-" + out
	}
	return out
}

// categoryLabel returns the Catalan label for an expense category.
func categoryLabel(c model.ExpenseCategory) string {
	switch c {
	case model.CategoryInvestment:
		return "Despesa d'inversió"
	default:
		return "Despesa corrent"
	}
}
