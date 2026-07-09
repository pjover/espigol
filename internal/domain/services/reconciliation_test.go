package services

import (
	"testing"

	"github.com/pjover/espigol/internal/domain/model"
)

func TestReconciliation_EmptyInput_ReturnsEmptyData(t *testing.T) {
	in := ReconciliationInput{Year: 2025}
	got, err := ComputeReconciliation(in)
	if err != nil {
		t.Fatalf("ComputeReconciliation: %v", err)
	}
	if got.Year != 2025 {
		t.Errorf("Year = %d, want 2025", got.Year)
	}
	if len(got.Categories) != 0 {
		t.Errorf("Categories = %d, want 0", len(got.Categories))
	}
	// Enum values must be declared
	_ = StatusFullyJustified
	_ = StatusPartiallyJustified
	_ = StatusOverExecuted
	_ = StatusPaymentPending
	_ = StatusNoInvoice
	_ = model.ZeroMoney() // used later
}
