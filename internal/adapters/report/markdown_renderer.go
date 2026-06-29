package report

import (
	"fmt"
	"strings"
	"time"

	"github.com/pjover/espigol/internal/domain/model/report"
)

// MarkdownRenderer renders ReportData to Markdown using the shared block layout,
// so its sections and tables match the PDF exactly.
type MarkdownRenderer struct{}

// Render returns the Markdown document for rd.
func (MarkdownRenderer) Render(rd report.ReportData) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# Previsions de despeses %d\n\n", rd.Year)
	for _, blk := range buildLayout(rd, time.Time{}) {
		switch v := blk.(type) {
		case SectionTitle:
			fmt.Fprintf(&b, "## %s\n\n", v.Text)
		case PageBreak:
			b.WriteString("---\n\n")
		case Table:
			writeMarkdownTable(&b, v)
		}
	}
	return []byte(b.String())
}

func writeMarkdownTable(b *strings.Builder, t Table) {
	if t.Title != "" {
		fmt.Fprintf(b, "### %s\n\n", t.Title)
	}
	headers := t.Headers
	// drop fully-empty header sets (e.g. the remainder summary) into a 2-col blank header
	fmt.Fprintf(b, "| %s |\n", strings.Join(headers, " | "))
	sep := make([]string, len(headers))
	for i := range sep {
		sep[i] = "---"
	}
	fmt.Fprintf(b, "| %s |\n", strings.Join(sep, " | "))
	for _, row := range t.Rows {
		cells := make([]string, len(row.Cells))
		for i, c := range row.Cells {
			cell := escapePipes(c)
			if (row.Bold || row.Red) && cell != "" {
				cell = "**" + cell + "**"
			}
			cells[i] = cell
		}
		fmt.Fprintf(b, "| %s |\n", strings.Join(cells, " | "))
	}
	b.WriteString("\n")
}

func escapePipes(s string) string { return strings.ReplaceAll(s, "|", "\\|") }
