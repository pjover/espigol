package report

import (
	"fmt"
	"time"

	"github.com/pjover/espigol/internal/domain/model/report"
	"github.com/pjover/espigol/internal/domain/ports"
)

// PDFRenderer renders ReportData to a PDF using the shared block layout and the
// maroto scaffolding. It implements ports.ReportRenderer (used by WindowService.Close
// to store the Report.pdf BLOB).
type PDFRenderer struct {
	BusinessName string
	LogoPath     string
}

// Render returns the PDF bytes for rd.
func (r PDFRenderer) Render(rd report.ReportData, generatedAt time.Time) ([]byte, error) {
	title := fmt.Sprintf("Previsions de despeses %d", rd.Year)
	footer := generatedAt.Format("02/01/2006")
	return renderDocument(title, footer, r.BusinessName, r.LogoPath, buildLayout(rd, generatedAt))
}

var _ ports.ReportRenderer = PDFRenderer{}
