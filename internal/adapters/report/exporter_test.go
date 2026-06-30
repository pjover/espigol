package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pjover/espigol/internal/domain/model"
)

func TestReportExporter_WritesPdfAndMd(t *testing.T) {
	rd := buildGolden(t)
	snapshot, err := json.Marshal(rd)
	if err != nil {
		t.Fatal(err)
	}
	pdfBytes := []byte("%PDF-1.7 fake")
	rep, err := model.NewReport(1, 2026, time.Now().UTC(), string(snapshot), pdfBytes, nil)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	exporter := NewReportExporter(PDFRenderer{BusinessName: "Test", LogoPath: ""})
	if err := exporter.Export(rep, dir); err != nil {
		t.Fatalf("export: %v", err)
	}

	pdfPath := filepath.Join(dir, "Previsions de despeses 2026.pdf")
	mdPath := filepath.Join(dir, "Previsions de despeses 2026.md")
	gotPdf, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("pdf not written: %v", err)
	}
	if string(gotPdf) != string(pdfBytes) {
		t.Errorf("pdf file bytes != BLOB")
	}
	gotMd, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("md not written: %v", err)
	}
	if !strings.Contains(string(gotMd), "# Previsions de despeses 2026") || !strings.Contains(string(gotMd), "23.498,96 €") {
		t.Errorf("md content unexpected")
	}
}

func TestReportExporter_ExportData_RendersFreshPreview(t *testing.T) {
	rd := buildGolden(t)
	generatedAt := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)

	dir := t.TempDir()
	exporter := NewReportExporter(PDFRenderer{BusinessName: "Test", LogoPath: ""})
	if err := exporter.ExportData(rd, generatedAt, dir); err != nil {
		t.Fatalf("export data: %v", err)
	}

	pdfPath := filepath.Join(dir, "Previsions de despeses 2026.pdf")
	mdPath := filepath.Join(dir, "Previsions de despeses 2026.md")

	gotPdf, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("pdf not written: %v", err)
	}
	if !strings.HasPrefix(string(gotPdf), "%PDF") {
		t.Errorf("pdf does not start with %%PDF")
	}

	gotMd, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("md not written: %v", err)
	}
	if !strings.Contains(string(gotMd), "2.880,00 €") {
		t.Errorf("md content missing golden EU number")
	}
}
