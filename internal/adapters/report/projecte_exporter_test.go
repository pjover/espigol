package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjecteExporter_WritesBothFiles(t *testing.T) {
	dir := t.TempDir()
	paths, err := NewProjecteExporter().Export(projData2025(t), dir)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("paths = %v, want 2", paths)
	}

	proj := filepath.Join(dir, "Projecte d'actuació 2025.md")
	press := filepath.Join(dir, "Pressupost del projecte d'actuació 2025.md")
	if paths[0] != proj || paths[1] != press {
		t.Errorf("paths = %v, want [%q %q]", paths, proj, press)
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %q to exist: %v", p, err)
		}
	}

	projBody, _ := os.ReadFile(proj)
	if !strings.Contains(string(projBody), "# Projecte d'actuació 2025") {
		t.Errorf("projecte file missing its title")
	}
	pressBody, _ := os.ReadFile(press)
	if !strings.Contains(string(pressBody), "## Resum per tipus de despesa") {
		t.Errorf("pressupost file missing its summary")
	}
}
