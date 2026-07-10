package report

import (
	"fmt"
	"strings"

	"github.com/pjover/espigol/internal/domain/services"
)

// ReconciliationMarkdownRenderer renders ReconciliationData to Markdown using
// the shared block layout, so sections and tables match the PDF exactly.
type ReconciliationMarkdownRenderer struct{}

func (ReconciliationMarkdownRenderer) Render(rd services.ReconciliationData) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# Conciliació d'ajuts %d\n\n", rd.Year)
	for _, blk := range buildReconciliationLayout(rd) {
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
