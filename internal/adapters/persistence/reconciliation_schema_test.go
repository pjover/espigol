package persistence_test

import (
	"context"
	"testing"

	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
)

func TestReconciliationSchema_TablesExistAndQueryEmpty(t *testing.T) {
	q := openTestDB(t)
	ctx := context.Background()

	cs, err := q.ListConcessionsByYear(ctx, 2025)
	if err != nil {
		t.Fatalf("ListConcessionsByYear: %v", err)
	}
	if len(cs) != 0 {
		t.Fatalf("want 0 concessions, got %d", len(cs))
	}
	inv, err := q.ListInvoicesByYear(ctx, 2025)
	if err != nil {
		t.Fatalf("ListInvoicesByYear: %v", err)
	}
	if len(inv) != 0 {
		t.Fatalf("want 0 invoices, got %d", len(inv))
	}
	_ = sqlc.Concession{}
}
