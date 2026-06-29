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
	if err := NewReportExporter().Export(rep, dir); err != nil {
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
