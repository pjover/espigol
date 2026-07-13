package report

import (
	"fmt"
	"strings"

	"github.com/pjover/espigol/internal/domain/model"
	"github.com/pjover/espigol/internal/domain/services"
)

// PressupostRenderer renders the F1 budget document (Markdown) from ProjecteData:
// a summary table by tipus/apartat plus per-concept breakdowns for corrents and
// inversions, each concept line carrying its CP code(s).
type PressupostRenderer struct{}

func (PressupostRenderer) Render(d services.ProjecteData) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# Pressupost del projecte d'actuació %d\n\n", d.Year)
	b.WriteString("El pressupost de les actuacions per a les quals es demana subvenció és el següent:\n\n")

	// Resum per tipus de despesa.
	b.WriteString("## Resum per tipus de despesa\n\n")
	resum := Table{Headers: []string{"Tipus", "Apartat", "Brut"}}
	for _, tp := range d.Tipus {
		for i, ap := range tp.Apartats {
			tipusCell := ""
			if i == 0 {
				tipusCell = tipusHeading(tp.Code, tp.Label)
			}
			resum.Rows = append(resum.Rows, Row{Cells: []string{tipusCell, apartatHeading(ap.Code, ap.Label), formatEuro(ap.Total)}})
		}
		resum.Rows = append(resum.Rows, Row{Cells: []string{"Total " + tipusHeading(tp.Code, tp.Label), "", formatEuro(tp.Total)}, Bold: true})
	}
	resum.Rows = append(resum.Rows, Row{Cells: []string{"Total general", "", formatEuro(d.Total)}, Bold: true})
	writeMarkdownTable(&b, resum)

	// Desglossament per conceptes, one section per tipus.
	for _, tp := range d.Tipus {
		fmt.Fprintf(&b, "## %s\n\n", desglossamentTitle(tp))
		for _, ap := range tp.Apartats {
			tbl := Table{Title: apartatHeading(ap.Code, ap.Label), Headers: []string{"Concepte", "CP", "Brut"}}
			for _, c := range ap.Concepts {
				tbl.Rows = append(tbl.Rows, Row{Cells: []string{c.Name, strings.Join(c.CPs, ", "), formatEuro(c.Total)}})
			}
			tbl.Rows = append(tbl.Rows, Row{Cells: []string{"Total " + apartatPrefix(ap.Code), "", formatEuro(ap.Total)}, Bold: true})
			writeMarkdownTable(&b, tbl)
		}
		fmt.Fprintf(&b, "**Total general: %s**\n\n", formatEuro(tp.Total))
	}
	return []byte(b.String())
}

func desglossamentTitle(tp services.TipusProjecte) string {
	if tp.Category == model.CategoryInvestment {
		return "Desglossament per conceptes d'inversions"
	}
	return "Desglossament per conceptes de despeses corrents"
}

// apartatPrefix turns a subtype code into its dotted apartat prefix: "a2" -> "a.2.".
func apartatPrefix(code string) string {
	if len(code) < 2 {
		return code + "."
	}
	return code[:1] + "." + code[1:] + "."
}

// apartatHeading is the full apartat label, e.g. "a.2. Activitats d'informació…".
func apartatHeading(code, label string) string { return apartatPrefix(code) + " " + label }

// tipusHeading is the full tipus label, e.g. "A. Despeses corrents".
func tipusHeading(code, label string) string { return code + ". " + label }
