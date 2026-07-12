package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/ports"
	"github.com/pjover/espigol/internal/domain/services"
)

// ReconciliationExporter writes a stored ReconciliationSnapshot's PDF BLOB and
// a freshly rendered Markdown document to the output directory.
type ReconciliationExporter struct {
	pdf ports.ReconciliationRenderer
	md  ReconciliationMarkdownRenderer
}

func NewReconciliationExporter(pdf ports.ReconciliationRenderer, md ReconciliationMarkdownRenderer) ReconciliationExporter {
	return ReconciliationExporter{pdf: pdf, md: md}
}

// Export writes "<outputDir>/Conciliació ajuts <year>.pdf" (the stored BLOB)
// and "<outputDir>/Conciliació ajuts <year>.md" (freshly rendered from the
// snapshot). Returns the written file paths.
func (e ReconciliationExporter) Export(rec model.ReconciliationSnapshot, outputDir string) ([]string, error) {
	dir := expandTilde(outputDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating output dir %q: %w", dir, err)
	}

	base := fmt.Sprintf("Conciliació ajuts %d", rec.Year())
	pdfPath := filepath.Join(dir, base+".pdf")
	if err := os.WriteFile(pdfPath, rec.Pdf(), 0o644); err != nil {
		return nil, fmt.Errorf("writing %q: %w", pdfPath, err)
	}

	var rd services.ReconciliationData
	if err := json.Unmarshal([]byte(rec.SnapshotJSON()), &rd); err != nil {
		return nil, fmt.Errorf("decoding snapshot JSON: %w", err)
	}
	mdPath := filepath.Join(dir, base+".md")
	if err := os.WriteFile(mdPath, e.md.Render(rd), 0o644); err != nil {
		return nil, fmt.Errorf("writing %q: %w", mdPath, err)
	}
	return []string{pdfPath, mdPath}, nil
}

// ExportData renders a live ReconciliationData snapshot to PDF + MD files.
func (e ReconciliationExporter) ExportData(rd services.ReconciliationData, at time.Time, outputDir string) ([]string, error) {
	dir := expandTilde(outputDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating output dir %q: %w", dir, err)
	}
	base := fmt.Sprintf("Conciliació ajuts %d", rd.Year)
	pdfBytes, err := e.pdf.Render(rd, at)
	if err != nil {
		return nil, fmt.Errorf("rendering PDF: %w", err)
	}
	pdfPath := filepath.Join(dir, base+".pdf")
	if err := os.WriteFile(pdfPath, pdfBytes, 0o644); err != nil {
		return nil, fmt.Errorf("writing PDF: %w", err)
	}
	mdPath := filepath.Join(dir, base+".md")
	if err := os.WriteFile(mdPath, e.md.Render(rd), 0o644); err != nil {
		return nil, fmt.Errorf("writing MD: %w", err)
	}
	return []string{pdfPath, mdPath}, nil
}

var _ ports.ReconciliationExporter = ReconciliationExporter{}
