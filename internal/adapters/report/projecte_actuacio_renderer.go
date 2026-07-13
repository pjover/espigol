package report

import (
	"fmt"
	"strings"

	"github.com/pjover/espigol/internal/domain/services"
)

// ProjecteActuacioRenderer renders the "Projecte d'actuació" narrative skeleton
// (Markdown): editable placeholders for the intro and each Objectius apartat,
// plus a populated Activitats section (one bullet per concept with its CP code).
type ProjecteActuacioRenderer struct{}

func (ProjecteActuacioRenderer) Render(d services.ProjecteData) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# Projecte d'actuació %d\n\n", d.Year)
	b.WriteString("_[Introducció: convocatòria, BOIB, descripció de la cooperativa i activitats sol·licitades.]_\n\n")

	b.WriteString("## Objectius\n\n")
	for _, tp := range d.Tipus {
		for _, ap := range tp.Apartats {
			fmt.Fprintf(&b, "### %s\n\n", apartatHeading(ap.Code, ap.Label))
			b.WriteString("_[Objectius d'aquest apartat]_\n\n")
		}
	}

	b.WriteString("## Activitats\n\n")
	for _, tp := range d.Tipus {
		for _, ap := range tp.Apartats {
			fmt.Fprintf(&b, "### %s\n\n", apartatHeading(ap.Code, ap.Label))
			for _, c := range ap.Concepts {
				fmt.Fprintf(&b, "- %s (%s)\n", c.Name, strings.Join(c.CPs, ", "))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("_Estellencs, en data de la signatura._\n\n")
	b.WriteString("_Pere Jover Casasnovas, President_\n")
	return []byte(b.String())
}
