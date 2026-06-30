package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/pjover/espigol/internal/domain/model"
)

// reportDoneMsg carries the outcome of a generateReportCmd: the output paths
// written (PDF + Markdown), or an error.
type reportDoneMsg struct {
	year  int
	paths []string
	err   error
}

// generateReportCmd produces the report for year/state and writes it via
// deps.Exporter into deps.Cfg's output directory:
//   - CLOSED  -> deps.Reports.Latest(year) -> deps.Exporter.Export(rep, outputDir)
//   - DRAFT/OPEN -> deps.Reports.Preview(year) -> deps.Exporter.ExportData(rd, now, outputDir)
//
// This is the shared report-generation helper Task 12's Informes panel
// also uses; the Anys panel's "r" action calls it directly so report
// generation works without waiting on Task 12.
func generateReportCmd(deps Deps, year int, state model.WindowState) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		outputDir := ""
		if deps.Cfg != nil {
			outputDir = deps.Cfg.OutputDir
		}

		if state == model.WindowClosed {
			rep, ok, err := deps.Reports.Latest(ctx, year)
			if err != nil {
				return reportDoneMsg{year: year, err: err}
			}
			if !ok {
				return reportDoneMsg{year: year, err: fmt.Errorf("cap informe desat per a l'any %d", year)}
			}
			if err := deps.Exporter.Export(rep, outputDir); err != nil {
				return reportDoneMsg{year: year, err: err}
			}
			base := fmt.Sprintf("Previsions de despeses %d", year)
			return reportDoneMsg{year: year, paths: []string{base + ".pdf", base + ".md"}}
		}

		rd, err := deps.Reports.Preview(ctx, year)
		if err != nil {
			return reportDoneMsg{year: year, err: err}
		}
		if err := deps.Exporter.ExportData(rd, time.Now(), outputDir); err != nil {
			return reportDoneMsg{year: year, err: err}
		}
		base := fmt.Sprintf("Previsions de despeses %d", year)
		return reportDoneMsg{year: year, paths: []string{base + ".pdf", base + ".md"}}
	}
}
