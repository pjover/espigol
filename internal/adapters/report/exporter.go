package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
	domreport "github.com/pjover/espigol/internal/domain/model/report"
)

// ReportExporter writes a stored Report's PDF BLOB and a freshly rendered
// Markdown document to the output directory. It does NOT re-render the PDF, so
// the .pdf file is byte-identical to the stored BLOB.
type ReportExporter struct {
	pdf PDFRenderer
	md  MarkdownRenderer
}

func NewReportExporter(pdf PDFRenderer) ReportExporter { return ReportExporter{pdf: pdf} }

// Export writes "Previsions de despeses <year>.pdf" (the BLOB) and ".md"
// (rendered from the snapshot) into outputDir.
func (e ReportExporter) Export(rep model.Report, outputDir string) error {
	dir := expandTilde(outputDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating output dir %q: %w", dir, err)
	}

	base := fmt.Sprintf("Previsions de despeses %d", rep.Year())
	pdfPath := filepath.Join(dir, base+".pdf")
	if err := os.WriteFile(pdfPath, rep.Pdf(), 0o644); err != nil {
		return fmt.Errorf("writing %q: %w", pdfPath, err)
	}

	var rd domreport.ReportData
	if err := json.Unmarshal([]byte(rep.SnapshotJSON()), &rd); err != nil {
		return fmt.Errorf("decoding report snapshot: %w", err)
	}
	mdPath := filepath.Join(dir, base+".md")
	if err := os.WriteFile(mdPath, e.md.Render(rd), 0o644); err != nil {
		return fmt.Errorf("writing %q: %w", mdPath, err)
	}
	return nil
}

// ExportData renders a live ReportData to PDF + MD files (a preview; not stored).
func (e ReportExporter) ExportData(rd domreport.ReportData, generatedAt time.Time, outputDir string) error {
	dir := expandTilde(outputDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating output dir %q: %w", dir, err)
	}
	base := fmt.Sprintf("Previsions de despeses %d", rd.Year)
	pdfBytes, err := e.pdf.Render(rd, generatedAt)
	if err != nil {
		return fmt.Errorf("rendering pdf: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, base+".pdf"), pdfBytes, 0o644); err != nil {
		return fmt.Errorf("writing pdf: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, base+".md"), e.md.Render(rd), 0o644); err != nil {
		return fmt.Errorf("writing md: %w", err)
	}
	return nil
}

func expandTilde(p string) string {
	if len(p) < 2 || p[:2] != "~/" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, p[2:])
}
