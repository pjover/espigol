package report

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pjover/espigol/internal/domain/services"
)

// ProjecteExporter renders and writes the two Consorci Markdown documents for a
// year into outputDir, returning their paths (Projecte first, Pressupost second).
type ProjecteExporter struct {
	projecte   ProjecteActuacioRenderer
	pressupost PressupostRenderer
}

func NewProjecteExporter() ProjecteExporter { return ProjecteExporter{} }

func (e ProjecteExporter) Export(d services.ProjecteData, outputDir string) ([]string, error) {
	dir := expandTilde(outputDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating output dir %q: %w", dir, err)
	}
	projPath := filepath.Join(dir, fmt.Sprintf("Projecte d'actuació %d.md", d.Year))
	if err := os.WriteFile(projPath, e.projecte.Render(d), 0o644); err != nil {
		return nil, fmt.Errorf("writing %q: %w", projPath, err)
	}
	pressPath := filepath.Join(dir, fmt.Sprintf("Pressupost del projecte d'actuació %d.md", d.Year))
	if err := os.WriteFile(pressPath, e.pressupost.Render(d), 0o644); err != nil {
		return nil, fmt.Errorf("writing %q: %w", pressPath, err)
	}
	return []string{projPath, pressPath}, nil
}
