package report

import (
	"fmt"
	"html"
	"strings"

	"github.com/pjover/espigol/internal/domain/model/report"
)

// HTMLRenderer renders ReportData to an HTML fragment using the shared block
// layout, so the on-screen report matches the PDF and Markdown by construction.
type HTMLRenderer struct{}

// Render returns the report as an HTML fragment (a sequence of <h1>/<h2>/<h3>/<table>).
func (HTMLRenderer) Render(rd report.ReportData) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "<h1>%s</h1>\n", html.EscapeString(fmt.Sprintf("Previsions de despeses %d", rd.Year)))
	for _, blk := range buildLayout(rd) {
		switch v := blk.(type) {
		case SectionTitle:
			fmt.Fprintf(&b, "<h2>%s</h2>\n", html.EscapeString(v.Text))
		case PageBreak:
			// no page concept in HTML
		case Table:
			writeHTMLTable(&b, v)
		}
	}
	return []byte(b.String())
}

func writeHTMLTable(b *strings.Builder, t Table) {
	if t.Title != "" {
		fmt.Fprintf(b, "<h3>%s</h3>\n", html.EscapeString(t.Title))
	}
	b.WriteString("<table>\n")
	if hasNonEmpty(t.Headers) {
		b.WriteString("<thead><tr>")
		for _, h := range t.Headers {
			fmt.Fprintf(b, "<th>%s</th>", html.EscapeString(h))
		}
		b.WriteString("</tr></thead>\n")
	}
	b.WriteString("<tbody>\n")
	for _, row := range t.Rows {
		b.WriteString("<tr")
		if row.Red {
			b.WriteString(` class="red"`)
		}
		b.WriteString(">")
		for _, c := range row.Cells {
			cell := html.EscapeString(c)
			if row.Bold && c != "" {
				cell = "<strong>" + cell + "</strong>"
			}
			fmt.Fprintf(b, "<td>%s</td>", cell)
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</tbody>\n</table>\n")
}
