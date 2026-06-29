package report

import (
	"strings"
	"testing"

	"github.com/pjover/espigol/internal/domain/model/report"
)

func TestMarkdownRenderer_StructureAndNumbers(t *testing.T) {
	rd := buildGolden(t)
	md := string(MarkdownRenderer{}.Render(rd))

	if !strings.HasPrefix(md, "# Previsions de despeses 2026") {
		t.Errorf("missing H1 title; got start: %.40q", md)
	}
	for _, want := range []string{
		"## Resum",
		"## Despesa corrent",
		"## Despesa d'inversió",
		"### Despesa corrent",
		"### Despesa d'inversió",
		"| CP | Concepte | Brut |", // a detail/common table header
		"2.880,00 €", "27.111,00 €", "23.498,96 €", "11.203,04 €",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q", want)
		}
	}
	// well-formed table: a header separator row exists
	if !strings.Contains(md, "| --- |") && !strings.Contains(md, "|---|") {
		t.Errorf("no markdown table separator found")
	}
}

var _ = report.ReportData{} // ensure import used
