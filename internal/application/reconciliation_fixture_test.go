package application_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pjover/espigol/internal/adapters/importer"
	"github.com/pjover/espigol/internal/application"
)

func TestReconciliation2025Fixture_ImportsAndBundles(t *testing.T) {
	world := newReconWorldWithForecasts(t, "CP25008", "CP25009")
	svc := application.NewReconciliationService(world.tx)
	ctx := context.Background()

	in, err := importer.LoadReconciliation(filepath.Join("testdata", "reconciliation-2025-sample.json"), 2025)
	if err != nil {
		t.Fatalf("LoadReconciliation: %v", err)
	}
	res, err := svc.AdminImport(ctx, in)
	if err != nil {
		t.Fatalf("AdminImport: %v", err)
	}
	if res.Concessions != 1 || res.Invoices != 1 {
		t.Fatalf("counts: %+v", res)
	}

	links, _ := svc.ListConcessionLinks(ctx, 2025)
	if len(links) != 2 {
		t.Fatalf("A6-02 bundle should have 2 forecasts, got %d", len(links))
	}
	invs, _ := svc.ListInvoices(ctx, 2025)
	if len(invs) != 1 || len(invs[0].Links()) != 2 {
		t.Fatalf("F878 should link 2 forecasts, got %+v", invs)
	}
}
