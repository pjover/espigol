// internal/adapters/report/reconciliation_pdf_renderer_test.go
package report_test

import (
	"testing"
	"time"

	reportpkg "github.com/pjover/espigol/internal/adapters/report"
	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/services"
)

func TestReconciliationPDFRenderer_ProducesPDF(t *testing.T) {
	rd := services.ReconciliationData{
		Year: 2025,
		Categories: []services.CategoryReconciliation{
			{
				Category: model.CategoryCurrent,
				Subtypes: []services.SubtypeReconciliation{
					{Code: "a6", Label: "[a6]"},
				},
			},
		},
	}
	r := reportpkg.ReconciliationPDFRenderer{BusinessName: "Test Coop", LogoPath: ""}
	pdf, err := r.Render(rd, time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(pdf) == 0 {
		t.Fatal("expected non-empty PDF bytes")
	}
	if string(pdf[:5]) != "%PDF-" {
		t.Errorf("PDF does not start with %%PDF-: %q", pdf[:5])
	}
}
