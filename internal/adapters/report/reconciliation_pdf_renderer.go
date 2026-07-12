package report

import (
	"fmt"
	"time"

	"github.com/pjover/espigol/internal/domain/ports"
	"github.com/pjover/espigol/internal/domain/services"
)

// ReconciliationPDFRenderer renders ReconciliationData to PDF via the shared
// block layout + maroto scaffolding. Implements ports.ReconciliationRenderer.
type ReconciliationPDFRenderer struct {
	BusinessName string
	LogoPath     string
}

func (r ReconciliationPDFRenderer) Render(rd services.ReconciliationData, generatedAt time.Time) ([]byte, error) {
	title := fmt.Sprintf("Conciliació d'ajuts %d", rd.Year)
	footer := generatedAt.Format("02/01/2006")
	return renderDocument(title, footer, r.BusinessName, r.LogoPath, buildReconciliationLayout(rd))
}

var _ ports.ReconciliationRenderer = ReconciliationPDFRenderer{}
