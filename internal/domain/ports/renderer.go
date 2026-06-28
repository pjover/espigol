package ports

import (
	"time"

	"github.com/pjover/espigol/internal/domain/model/report"
)

// ReportRenderer renders a computed ReportData into a document (e.g. PDF).
// Phase 4 uses a no-op; Phase 5 provides the real maroto/Markdown renderer.
type ReportRenderer interface {
	Render(rd report.ReportData, generatedAt time.Time) ([]byte, error)
}
