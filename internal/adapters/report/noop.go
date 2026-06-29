// Package report holds ReportRenderer adapters. Phase 4 ships only a no-op;
// Phase 5 adds the maroto PDF and Markdown renderers here.
package report

import (
	"time"

	reportmodel "github.com/pjover/espigol/internal/domain/model/report"
)

// NoopRenderer is a placeholder ReportRenderer that produces no document.
// It exists so the window-close flow can run before PDF rendering lands (Phase 5).
type NoopRenderer struct{}

// Render returns an empty (non-nil) byte slice, which satisfies the
// report.pdf BLOB NOT NULL column without producing an actual document.
func (NoopRenderer) Render(rd reportmodel.ReportData, generatedAt time.Time) ([]byte, error) {
	return []byte{}, nil
}
